package notify

// Cards are the bot's messages: a bold header, a rule, one labelled field per
// line, and inline buttons. Every dynamic value is HTML-escaped and capped.

import (
	"context"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"tagent/internal/tmux"
)

const (
	rule      = "━━━━━━━━━━━━━━"
	bodyUnits = 2500 // escaped body of one card; header and fields use the rest of Telegram's cap
	listMax   = 20   // rows in an agents or tickets view
)

type button = map[string]string

func btn(text, data string) button { return button{"text": text, "callback_data": data} }

func keyboard(rows ...[]button) map[string]any {
	return map[string]any{"inline_keyboard": rows}
}

// esc is s escaped, cut so that it takes at most units UTF-16 units.
func esc(s string, units int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		e := html.EscapeString(string(r))
		if n += utf16Units(e); n > units {
			return b.String() + "…"
		}
		b.WriteString(e)
	}
	return b.String()
}

func card(icon, title string, fields []string, body string) string {
	parts := []string{icon + " <b>" + esc(title, 200) + "</b>", rule}
	if len(fields) > 0 {
		parts = append(parts, fields...)
		if body != "" {
			parts = append(parts, rule)
		}
	}
	if body != "" {
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n")
}

func field(icon, label, value string) string {
	return icon + " " + label + ": <b>" + esc(value, 200) + "</b>"
}

// ── agent alerts ──────────────────────────────────────────────────────────

type paneInfo struct{ Project, Branch, Ticket string }

var paneContext = readPaneContext // seam: tests fake tmux, git and the board

// readPaneContext says where a pane works; anything unknown stays empty.
func readPaneContext(ctx context.Context, pane string) paneInfo {
	var info paneInfo
	dir := tmux.DisplayMessage(pane, "#{pane_current_path}")
	dir = strings.TrimSpace(dir)
	if dir != "" {
		root, _, _ := strings.Cut(dir, "/.worktrees/")
		info.Project = filepath.Base(root)
		if out, err := exec.CommandContext(ctx, "git", "-C", dir, "branch", "--show-current").Output(); err == nil {
			info.Branch = strings.TrimSpace(string(out))
		}
	}
	if _, id, ok := strings.Cut(tmux.GetPaneOptionValue(pane, tmux.PaneTicket), "#"); ok && id != "" {
		info.Ticket = "#" + id
		// "<id>: <title>" is the first line of the brief, resolved from the pane's repo.
		if out, err := tagentIn(ctx, dir, "task", "show", id, "--brief"); err == nil {
			first, _, _ := strings.Cut(out, "\n")
			if _, title, ok := strings.Cut(first, ": "); ok {
				info.Ticket += " " + title
			}
		}
	}
	return info
}

// tagentIn runs this binary in dir, for board commands that resolve by directory.
func tagentIn(ctx context.Context, dir string, args ...string) (string, error) {
	exe, err := os.Executable()
	if err != nil || testingBinary() {
		return "", fmt.Errorf("unavailable")
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir, cmd.Env = dir, append(os.Environ(), "TMUX_PANE=")
	out, err := cmd.Output()
	return string(out), err
}

func isPermission(body string) bool {
	return body == "Permission required" || strings.HasPrefix(body, "permission")
}

// alertCard is the message for something a pane's agent reports. question and
// screen describe an open dialog; both are empty without one.
func alertCard(a agentPane, info paneInfo, title, body, question, screen string) string {
	icon, head := "🔔", title
	switch {
	case isPermission(body):
		icon, head = "🔐", "Permission required"
	case question != "":
		icon, head = "❓", "Needs your answer"
	case strings.HasPrefix(body, "Task failed"):
		icon, head = "❌", "Task failed"
	case body == "Task completed" || strings.HasPrefix(body, "Task completed:"):
		icon, head = "✅", "Task completed"
	}
	who := a.label()
	if who == "" {
		who = title
	}
	if a.Agent != "" && a.Agent != who {
		who += " · " + a.Agent
	}
	fields := []string{field("🤖", "Agent", who+" · "+a.ID)}
	if info.Ticket != "" {
		fields = append(fields, field("🎫", "Ticket", info.Ticket))
	}
	if info.Project != "" {
		where := info.Project
		if info.Branch != "" {
			where += " · " + info.Branch
		}
		fields = append(fields, field("📁", "Project", where))
	}
	var text []string
	if question != "" {
		text = append(text, "<b>"+esc(question, 400)+"</b>")
	}
	if screen != "" {
		text = append(text, "<pre>"+esc(screen, bodyUnits-600)+"</pre>")
	}
	// A permission body is a raw hook reason ("permission_prompt"): never shown.
	if question == "" && screen == "" && !isPermission(body) && body != head {
		text = append(text, esc(body, bodyUnits))
	}
	return card(icon, head, fields, strings.Join(text, "\n"))
}

var boxChars = " \t│┃╭╮╰╯─━╌┄|"

// dialogContext is the few screen lines above the dialog's first option: the
// command or file the question is about.
func dialogContext(screen, question string, opts []dialogOpt) string {
	lines := strings.Split(screen, "\n")
	end := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if len(opts) > 0 && strings.Contains(lines[i], opts[0].Label) {
			end = i
			break
		}
	}
	if end < 0 {
		return ""
	}
	var out []string
	for i := end - 1; i >= 0 && len(out) < 8; i-- {
		l := strings.Trim(lines[i], boxChars)
		if l == "" || (question != "" && strings.Contains(question, l)) || !utf8.ValidString(l) {
			continue
		}
		out = append(out, l)
	}
	slices.Reverse(out)
	return strings.Join(out, "\n")
}

// ── ticket and plain notices ──────────────────────────────────────────────

// ticketHead matches the first line tasks.statusNotice writes.
var ticketHead = regexp.MustCompile(`^(?:✅|⛔) (.+) · #(\S+) (done|blocked)(?: (?:in|at) (.+))?$`)

// noticeCard is a notification that belongs to no pane.
func noticeCard(title, body string) (string, any) {
	lines := strings.Split(body, "\n")
	m := ticketHead.FindStringSubmatch(lines[0])
	if m == nil || len(lines) < 2 {
		return card("🔔", title, nil, esc(body, bodyUnits)), nil
	}
	icon, extra := "✅", field("⏱", "Duration", m[4])
	if m[3] == "blocked" {
		icon, extra = "⛔", field("📍", "Blocked at", m[4])
	}
	fields := []string{field("🎫", "Ticket", lines[1]), field("📁", "Project", m[1])}
	if m[4] != "" {
		fields = append(fields, extra)
	}
	rest := strings.TrimSpace(strings.Join(lines[2:], "\n"))
	return card(icon, "Ticket #"+m[2]+" "+m[3], fields, esc(rest, bodyUnits)),
		keyboard([]button{btn("🎫 Tickets", "n|tickets"), btn("🤖 Agents", "n|agents")})
}

// ── menu views ────────────────────────────────────────────────────────────

type ticketRow struct{ Project, ID, Status, Title string }

var listTickets = readTickets // seam: tests fake the board

// readTickets lists the unfinished tickets of every registered repo.
func readTickets(ctx context.Context, dirs func() []string) []ticketRow {
	if dirs == nil {
		return nil
	}
	repos := dirs()
	slices.Sort(repos)
	var rows []ticketRow
	for _, d := range slices.Compact(repos) {
		out, err := tagentIn(ctx, d, "task", "ls")
		if err != nil {
			continue
		}
		rows = append(rows, parseTickets(filepath.Base(d), out)...)
	}
	return rows
}

// parseTickets reads `task ls` lines: id, status, assignees, title (tabbed).
func parseTickets(project, out string) []ticketRow {
	var rows []ticketRow
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, "\t", 4)
		if len(f) == 4 && f[1] != "done" && f[1] != "closed" {
			rows = append(rows, ticketRow{project, f[0], f[1], f[3]})
		}
	}
	return rows
}

func statusIcon(status tmux.PaneStatus) string {
	switch status {
	case tmux.StatusWaiting:
		return "🟡"
	case tmux.StatusRunning:
		return "🟢"
	case tmux.StatusError:
		return "🔴"
	}
	return "⚪"
}

// view renders one menu page: "home", "agents" or "tickets".
func view(ctx context.Context, cfg tgConfig, dirs func() []string, page, note string) (string, any) {
	nav := []button{btn("« Back", "n|home"), btn("🔄 Refresh", "n|"+page)}
	var text string
	var rows [][]button
	switch page {
	case "agents":
		var lines []string
		var row []button
		add := func(name string) {
			if nameRe.MatchString(name) && len("u|"+name) <= 64 { // Telegram callback_data cap
				if row = append(row, btn(trunc(name, 30), "u|"+name)); len(row) == 2 {
					rows, row = append(rows, row), nil
				}
			}
		}
		agents := listAgents()
		for i, a := range agents {
			if i == listMax {
				lines = append(lines, fmt.Sprintf("… %d more", len(agents)-listMax))
				break
			}
			lines = append(lines, statusIcon(a.Status)+" <b>"+esc(a.label(), 60)+"</b> · "+esc(a.Agent, 30)+" · "+esc(string(a.Status), 20))
			add(a.label())
		}
		if _, ok := matchAgent(cfg.defaultAgent); !ok {
			lines = append(lines, "⚪ <b>"+esc(cfg.defaultAgent, 60)+"</b> · not running (default)")
			add(cfg.defaultAgent)
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
		text = card("🤖", "Agents", nil, strings.Join(lines, "\n")+"\n"+rule+"\nTap an agent to chat with it.")
		rows = append(rows, nav)
	case "tickets":
		tickets := listTickets(ctx, dirs)
		var lines []string
		for i, t := range tickets {
			if i == listMax {
				lines = append(lines, fmt.Sprintf("… %d more", len(tickets)-listMax))
				break
			}
			lines = append(lines, "<b>#"+esc(t.ID, 20)+"</b> "+esc(t.Status, 20)+" · "+esc(t.Project, 30)+"\n"+esc(t.Title, 90))
		}
		if len(lines) == 0 {
			lines = []string{"No open tickets."}
		}
		text = card("🎫", "Open tickets", nil, strings.Join(lines, "\n"))
		rows = [][]button{nav}
	default:
		agents := map[tmux.PaneStatus]int{}
		for _, a := range listAgents() {
			agents[a.Status]++
		}
		tickets := listTickets(ctx, dirs)
		blocked := 0
		for _, t := range tickets {
			if t.Status == "blocked" {
				blocked++
			}
		}
		fields := []string{
			field("🟡", "Waiting for you", fmt.Sprint(agents[tmux.StatusWaiting])),
			field("🟢", "Running", fmt.Sprint(agents[tmux.StatusRunning])),
			field("⚪", "Idle", fmt.Sprint(agents[tmux.StatusIdle])),
			field("🎫", "Open tickets", fmt.Sprint(len(tickets))),
			field("⛔", "Blocked tickets", fmt.Sprint(blocked)),
		}
		if target := currentTarget(note); target != "" {
			fields = append(fields, field("💬", "Chatting with", target))
		}
		text = card("📊", "tagent · "+time.Now().Format("15:04"), fields, esc(helpText, 600))
		rows = [][]button{{btn("🤖 Agents", "n|agents"), btn("🎫 Tickets", "n|tickets")}, {btn("🔄 Refresh", "n|home")}}
	}
	return text, keyboard(rows...)
}

// currentTarget is the chat's chosen agent; chat is its id as a string.
func currentTarget(chat string) string { return loadState().Targets[chat] }

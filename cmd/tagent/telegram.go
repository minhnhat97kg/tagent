package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"tagent/internal/board"
	"tagent/internal/intake"
	"tagent/internal/notify"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
	"time"
)

const telegramHelp = "/list — projects\n/chat <name|number> — select project\n/read [n] — shared conversation (default 30)\n/stop — detach (conversation remains)\nDrafts and commands require explicit local approval in dashboard Chat.\n/no — cancel proposal\n/help — commands\nPlain text talks to the project receptionist, also visible in dashboard Chat. A sole project is selected automatically. No terminal forwarding."
const telegramProject = "@agent_telegram_project"

var telegramRun = intake.Run

type telegramControl struct {
	ctx                    context.Context
	client                 *http.Client
	raw, base, chat, token string
	identity               string
}

func (t *telegramControl) owner() bool {
	p, _ := tmux.GetOption("@agent_scrape_pid")
	raw, _ := tmux.GetOption("@agent_notify_url")
	return p == strconv.Itoa(os.Getpid()) && raw == t.raw && t.ctx.Err() == nil
}
func (t *telegramControl) endpoint() string { return fmt.Sprintf("%x", sha256.Sum256([]byte(t.raw))) }
func (t *telegramControl) say(s string) bool {
	return t.owner() && notify.TelegramCall(t.ctx, t.client, t.base, "sendMessage", url.Values{"chat_id": {t.chat}, "text": {notify.TelegramCompletion(s, t.token)}}, nil)
}
func telegramProjects() []string {
	dirs := board.Registered()
	for _, s := range tmux.QuerySessions() {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.Path != "" {
					dirs = append(dirs, tasks.MainRoot(p.Path))
				}
			}
		}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, dir := range dirs {
		dir = tasks.MainRoot(dir)
		info, e := os.Stat(dir)
		if e != nil || !info.IsDir() {
			continue
		}
		scope := tasks.Scope(dir)
		if !seen[scope] {
			seen[scope] = true
			out = append(out, dir)
		}
	}
	sort.Strings(out)
	return out
}
func (t *telegramControl) selectProject(dir string) bool {
	scope := tasks.Scope(dir)
	s, e := intake.LoadConversation(scope)
	if e != nil {
		t.say(e.Error())
		return false
	}
	// Publish selection and skip old history only after pending state is sent.
	// On HTTP failure the input offset stays unchanged, so selection is retried.
	if s.Status == "proposal" {
		if !t.say("Pending proposal:\n" + s.Raw + "\nApprove locally in dashboard Chat; /no cancels.") {
			return false
		}
	} else if s.Response.Question != "" {
		if !t.say(s.Response.Question) {
			return false
		}
	}
	if !t.owner() || !tmux.SetOptionResult(telegramProject, dir) {
		return false
	}
	revision := s.Revision
	_, e = intake.DeliveryCursor(scope, t.endpoint(), &revision)
	return e == nil
}
func (t *telegramControl) command(text string) bool {
	if !t.owner() {
		return false
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return true
	}
	cmd := fields[0]
	rest := strings.TrimSpace(strings.TrimPrefix(text, cmd))
	projects := telegramProjects()
	selected, _ := tmux.GetOption(telegramProject)
	valid := false
	for _, p := range projects {
		valid = valid || p == selected
	}
	if !valid {
		selected = ""
	}
	switch cmd {
	case "/help":
		return t.say(telegramHelp)
	case "/list":
		var b strings.Builder
		for i, p := range projects {
			fmt.Fprintf(&b, "%d. %s (%s)\n", i+1, filepath.Base(p), p)
		}
		if b.Len() == 0 {
			b.WriteString("No projects.")
		}
		return t.say(b.String())
	case "/chat":
		target := ""
		if n, e := strconv.Atoi(rest); e == nil {
			if n > 0 && n <= len(projects) {
				target = projects[n-1]
			}
		} else {
			for _, p := range projects {
				if rest == filepath.Base(p) || rest == p {
					if target != "" {
						return t.say("Ambiguous project name; use /list and its number.")
					}
					target = p
				}
			}
		}
		if target == "" {
			return t.say("Use /list then /chat <name|number>.")
		}
		if !t.selectProject(target) {
			return false
		}
		return t.say("Receptionist selected: " + filepath.Base(target))
	case "/stop":
		if !tmux.SetOptionResult(telegramProject, "") {
			return false
		}
		return t.say("Detached; shared conversation remains.")
	}
	if selected == "" && len(projects) == 1 {
		selected = projects[0]
		if !t.selectProject(selected) {
			return false
		}
	}
	if selected == "" {
		return t.say("Select a project with /list and /chat first.")
	}
	scope := tasks.Scope(selected)
	s, e := intake.LoadConversation(scope)
	if e != nil {
		return t.say(e.Error())
	}
	switch cmd {
	case "/read":
		n := 30
		if rest != "" {
			n, e = strconv.Atoi(rest)
			if e != nil || n < 1 || n > 48 {
				return t.say("Use /read [1-48].")
			}
		}
		var b strings.Builder
		for _, m := range s.Messages[max(0, len(s.Messages)-n):] {
			fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Text)
		}
		if b.Len() == 0 {
			b.WriteString("No conversation yet.")
		}
		return t.say(b.String())
	case "/no":
		_, e = intake.CancelProposal(scope, s.Revision)
		if e != nil {
			return t.say(e.Error())
		}
		return true
	case "ok", "/yes":
		return t.say("Drafts and commands require explicit local approval in dashboard Chat; /no cancels.")
	default:
		if strings.HasPrefix(cmd, "/") {
			return t.say(telegramHelp)
		}
		command, ok := tmux.GetOption("@agent_intake_llm")
		if !ok {
			command = "ollama run --nowordwrap --think=false bonsai-2:9b"
		}
		if command == "off" {
			return t.say("Receptionist disabled.")
		}
		s, e = intake.ReserveSubmission(scope, text, t.identity)
		if e != nil {
			if e == intake.ErrStale {
				return true
			}
			return t.say(e.Error())
		}
		sessions := tmux.QuerySessions()
		snapshot := intake.Snapshot(selected, scope, sessions, sessions == nil)
		names := []string{}
		for _, w := range workflow.List(tasks.MainRoot(selected)) {
			names = append(names, w.Name)
		}
		prompt := intake.ConversationPrompt(s, snapshot)
		// Polling continues while the existing intake runner works. Recheck ownership
		// before committing; endpoint takeover must not deliver stale results.
		owner := *t
		owner.ctx = context.Background()
		go func() {
			raw, err := telegramRun(command, prompt)
			if owner.owner() {
				intake.CompleteSubmission(scope, s.Generation, raw, err, names)
			}
		}()
		return true
	}
}
func (t *telegramControl) completion() {
	if !t.owner() {
		return
	}
	dir, _ := tmux.GetOption(telegramProject)
	if dir == "" {
		return
	}
	scope := tasks.Scope(dir)
	s, e := intake.LoadConversation(scope)
	if e != nil {
		return
	}
	cursor, e := intake.DeliveryCursor(scope, t.endpoint(), nil)
	if e != nil {
		return
	}
	for _, m := range s.Messages {
		if m.Revision <= cursor {
			continue
		}
		if m.Role != "user" {
			text := m.Text
			if s.Status == "proposal" {
				text += "\nApprove locally in dashboard Chat; /no cancels."
			}
			if !t.say(text) {
				return
			}
		}
		if !t.owner() {
			return
		}
		r := m.Revision
		if _, e = intake.DeliveryCursor(scope, t.endpoint(), &r); e != nil {
			return
		}
	}
}
func (t *telegramControl) updates(u []notify.TelegramUpdate) {
	sort.Slice(u, func(i, j int) bool { return u[i].ID < u[j].ID })
	offset, _ := tmux.GetOption("@agent_telegram_offset")
	next, _ := strconv.ParseInt(offset, 10, 64)
	for _, v := range u {
		if v.ID < next || !t.owner() {
			continue
		}
		if strconv.FormatInt(v.Message.Chat.ID, 10) == t.chat && v.Message.Chat.Type == "private" && v.Message.Text != "" {
			t.identity = t.endpoint() + ":" + strconv.FormatInt(v.ID, 10)
			if !t.command(v.Message.Text) {
				return
			}
		}
		next = v.ID + 1
		if !t.owner() || !tmux.SetOptionResult("@agent_telegram_offset", strconv.FormatInt(next, 10)) {
			return
		}
	}
}
func runTelegram(ctx context.Context) {
	for ctx.Err() == nil {
		raw, _ := tmux.GetOption("@agent_notify_url")
		base, chat, token, ok := notify.TelegramTarget(raw)
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		t := telegramControl{ctx: ctx, client: &http.Client{Timeout: 25 * time.Second}, raw: raw, base: base, chat: chat, token: token}
		if !t.owner() {
			return
		}
		// Upgrade from the removed pane bridge even when the endpoint is unchanged.
		if pane, _ := tmux.GetOption("@agent_telegram_pane"); pane != "" {
			if !tmux.SetOptionResult("@agent_telegram_pane", "") {
				return
			}
		}
		old, _ := tmux.GetOption("@agent_telegram_scope")
		if old != t.endpoint() {
			for _, key := range []string{telegramProject, "@agent_telegram_pane", "@agent_telegram_offset", "@agent_telegram_seen", "@agent_telegram_list"} {
				if !tmux.SetOptionResult(key, "") {
					return
				}
			}
			if !tmux.SetOptionResult("@agent_telegram_scope", t.endpoint()) {
				return
			}
		}
		poll, cancel := context.WithCancel(ctx)
		updates := make(chan []notify.TelegramUpdate, 1)
		offset, _ := tmux.GetOption("@agent_telegram_offset")
		go func() {
			var u []notify.TelegramUpdate
			notify.TelegramCall(poll, t.client, base, "getUpdates", url.Values{"offset": {offset}, "timeout": {"20"}, "allowed_updates": {"[\"message\"]"}}, &u)
			updates <- u
		}()
		ticker := time.NewTicker(time.Second)
		waiting := true
		for waiting {
			select {
			case <-ctx.Done():
				waiting = false
			case <-ticker.C:
				if !t.owner() {
					waiting = false
				} else {
					t.completion()
				}
			case u := <-updates:
				t.updates(u)
				waiting = false
			}
		}
		ticker.Stop()
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

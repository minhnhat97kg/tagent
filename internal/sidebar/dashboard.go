package sidebar

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/metrics"
	"tagent/internal/quota"
	"tagent/internal/roles"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

type metricMsg struct {
	Scope string
	State metrics.State
}

func (m Model) metricRefreshCmd() tea.Cmd {
	scope := m.notesScope
	if scope == "" {
		if id, ok := paneIDAt(m.rows, m.cursor); ok {
			if p, found := paneInfoByID(m.sessions, id); found && p.Path != "" {
				scope = tasks.Scope(p.Path)
			}
		}
	}
	sessions := m.sessions
	return func() tea.Msg {
		if scope == "" {
			return metricMsg{}
		}
		state := metrics.RefreshOrRead(scope, sessions)
		return metricMsg{Scope: scope, State: state}
	}
}

func (m Model) startSelectedTaskCmd() tea.Cmd {
	if m.taskCursor >= len(m.notes) || m.notesScope == "" {
		return nil
	}
	id := m.notes[m.taskCursor].ID
	paneID, ok := paneIDAt(m.rows, m.cursor)
	if !ok {
		return nil
	}
	p, ok := paneInfoByID(m.sessions, paneID)
	if !ok || p.Path == "" {
		return nil
	}
	return func() tea.Msg {
		bin, err := os.Executable()
		if err != nil {
			return refreshRequestMsg{}
		}
		cmd := exec.Command(bin, "task", "start", id, paneID)
		cmd.Dir = p.Path
		_ = cmd.Run()
		return refreshRequestMsg{}
	}
}

func (m Model) dashboardView() string {
	if m.taskForm != "" {
		return m.taskFormView()
	}
	if m.boardMode {
		return m.boardView()
	}
	w, h := m.width, m.height
	if w <= 0 {
		w = fallbackWidth
	}
	if h <= 0 {
		h = 20
	}
	if h == 1 {
		return fitLine("tagent", w)
	}
	g := m.dashGeometry()
	repo := m.notesScope
	if i := strings.LastIndexByte(repo, '-'); i > 0 {
		repo = repo[:i]
	}
	if repo == "" {
		repo = "current repo"
	}
	bar := m.topBar(repo, max(6, w-1))

	agents := m.dashboardPanel(fmt.Sprintf("Agents · %d", paneRowCount(m.rows)), m.agentRows(g.agents.w-4, g.agents.h-2), g.agents.w, g.agents.h, m.agentFocus == 0)
	usage := m.dashboardPanel("▥ Usage · "+m.rangeText(), g.usageLines, g.usage.w, g.usage.h, m.agentFocus == 3)
	var left []string
	left = append(left, agents, usage)
	if g.comp.h > 0 {
		_, avatar := m.companionPanel(g.comp.w)
		left = append(left, m.dashboardPanel("◈ Companion", avatar, g.comp.w, g.comp.h, false))
	}
	if g.roles.h > 0 {
		lines, _ := m.roleLines(g.roles.w-4, g.roles.h-2)
		left = append(left, m.dashboardPanel("Agent definitions", lines, g.roles.w, g.roles.h, m.agentFocus == 1))
	}
	taskTitle := fmt.Sprintf("☷ Tasks · %d", len(m.notes))
	if m.taskTitle {
		taskTitle += " · Enter board"
	}
	// Same kanban columns as the board, under a one-line header, then Workflows.
	head := m.boardStyle(m.theme.TextPrimary).Bold(true).Render(boardFit(" "+taskTitle, g.tasks.w))
	tasksCard := head + "\n" + m.boardColumnView(g.tasks.w, g.tasks.h-1, m.taskFocus, m.theme.BorderFocus, m.theme.Border)
	if g.wf.h > 0 {
		tasksCard += "\n" + m.workflowCard(g.wf.w, g.wf.h)
	}
	var body string
	if g.wide {
		body = lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(left, "\n"), tasksCard)
	} else {
		// Stacked: Agents, Usage, Tasks, then Companion/Roles when rows remain.
		parts := []string{left[0], left[1], tasksCard}
		body = strings.Join(append(parts, left[2:]...), "\n")
	}
	help := " Tab focus · ↑↓ select · Enter open · N new agent · l log · i prompt · b board"
	if m.agentFocus == 3 {
		help = " h/l or [ ] range · Tab focus"
	}
	if m.agentFocus == 1 {
		help = " ↑↓ role · e edit instruction · Tab focus"
	}
	if m.agentFocus == 4 {
		help = m.keyHints("j/k workflow", "enter view", "a add", "e edit", "x delete", "Tab focus")
	}
	if m.taskFocus {
		help = m.ticketHints()
	}
	if m.taskDelete {
		help = " Delete task?  y confirm · n cancel"
	}
	if m.taskNotice != "" {
		help = " " + m.taskNotice
	}
	footer := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render(fitLine(help, max(6, w-1)))
	result := strings.Split(bar+"\n"+body+"\n"+footer, "\n")
	if len(result) > h {
		result = append(result[:h-1], footer)
	}
	for len(result) < h {
		result = append(result, "")
	}
	return strings.Join(result, "\n")
}

// topBar is the one-line header: "tagent · repo" left, key hints right.
func (m Model) topBar(repo string, width int) string {
	left := lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true).Render("tagent") +
		lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render(" · "+repo)
	right := "? help · , settings"
	if width < 40+lipgloss.Width(right) {
		return fitLine(left, width)
	}
	pad := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", pad) + lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render(right)
}

// rangeText is "1d [7d] 30d" as plain text (the Usage card's header hint).
func (m Model) rangeText() string {
	var parts []string
	for _, d := range []int{1, 7, 30} {
		if d == m.chartDaysOrDefault() {
			parts = append(parts, fmt.Sprintf("[%dd]", d))
		} else {
			parts = append(parts, fmt.Sprintf("%dd", d))
		}
	}
	return strings.Join(parts, " ")
}

// meterLine renders "5h ▰▰▱▱▱ 42%" like the web Meter: green <70, amber >=70, red >=90, muted when unknown.
func (m Model) meter(label string, used *float64) string {
	const cells = 5
	muted := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	if used == nil {
		return muted.Render(label + " " + strings.Repeat("▱", cells) + " –")
	}
	v := max(0, min(100, *used))
	color := m.theme.StatusRunning
	if v >= 90 {
		color = m.theme.StatusError
	} else if v >= 70 {
		color = m.theme.StatusWaiting
	}
	n := int(v*cells/100 + 0.5)
	bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("▰", n)) +
		lipgloss.NewStyle().Foreground(m.theme.Border).Render(strings.Repeat("▱", cells-n))
	return muted.Render(label+" ") + bar + lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Render(fmt.Sprintf(" %.0f%%", v))
}

// providerMeters is the web Usage rows: "Claude 5h ▰▰▱▱▱ 42%" over "       7d ▰▱▱▱▱ 12%".
func (m Model) providerMeters(name string, p quota.Provider) []string {
	pad := fmt.Sprintf("%-6s", name)
	l1 := lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true).Render(pad) + " " + m.meter("5h", p.Five.Used)
	l2 := strings.Repeat(" ", 6) + " " + m.meter("7d", p.Week.Used)
	if p.Stale {
		l2 += lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render(" stale")
	}
	return []string{l1, l2}
}

func (m Model) chartDaysOrDefault() int {
	if m.chartDays == 0 {
		return 7
	}
	return m.chartDays
}

// chartLine is the sparkline row: the last day (today) is bright, earlier days muted.
func (m Model) chartLine(width int) string {
	window := metrics.TokenWindow(m.metricState, time.Now(), m.chartDaysOrDefault())
	runes := []rune(window.Sparkline)
	spark := ""
	if n := len(runes); n > 0 {
		spark = lipgloss.NewStyle().Foreground(m.theme.TextDim).Render(string(runes[:n-1])) +
			lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Render(string(runes[n-1:]))
	}
	if width < 28 {
		return " " + spark
	}
	return " " + spark + lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render("  "+compactTokens(window.Total)+" in "+fmt.Sprintf("%dd", m.chartDaysOrDefault()))
}

func sumInts(v []int64) int64 {
	var n int64
	for _, x := range v {
		n += x
	}
	return n
}
func compactTokens(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprint(n)
}
func fitLine(s string, width int) string {
	if width < 1 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}
func (m Model) agentLines(width, height int) []string {
	out := []string{m.sectionTitle("◉ Running agents", width, !m.taskFocus)}
	cursor := m.cursor
	if m.taskFocus {
		cursor = -1
	}
	rows := strings.Split(renderRowsAt(m.rows, width, cursor, m.theme, m.icons, m.animFrame), "\n")
	if len(rows) == 1 && rows[0] == emptyHint {
		rows = nil
	}
	if len(rows) == 0 {
		rows = []string{"  no agents running"}
	}
	start := 0
	if len(rows) > height-1 {
		// Keep the cursor row and the child rows under it (an approval's
		// answers, subagents) in view.
		idx, _ := rowIndexOfCursor(m.rows, m.cursor)
		end := idx
		for end+1 < len(m.rows) && (m.rows[end+1].IsInfoChild || m.rows[end+1].IsSubagentChild) {
			end++
		}
		start = min(idx, max(0, end-(height-2)))
		start = min(start, len(rows)-(height-1))
	}
	for _, row := range rows[start:min(len(rows), start+height-1)] {
		out = append(out, fitLine(row, width))
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}
func statusGlyph(s string) string {
	switch s {
	case "done":
		return "✓"
	case "doing":
		return "◐"
	case "blocked":
		return "✕"
	}
	return "○"
}

// taskUsage is "tokens 12.3k" (≥ when some runs lack usage, — when none).
func (m Model) taskUsage(id string) string {
	n, missing := metrics.TaskTokens(m.metricState, id)
	if missing == 0 {
		for _, r := range m.metricState.Runs {
			if r.TaskID == id {
				return "tokens " + compactTokens(n)
			}
		}
	} else if n > 0 {
		return "tokens ≥" + compactTokens(n)
	}
	return "tokens —"
}

func (m Model) taskDetail(width int) []string {
	if len(m.notes) == 0 || m.taskCursor >= len(m.notes) {
		return []string{m.sectionTitle("≡ Detail", width, false), "  select a task"}
	}
	t := m.notes[m.taskCursor]
	assignee := "unassigned"
	if len(t.Assignees) > 0 {
		assignee = strings.Join(t.Assignees, ", ")
	}
	return []string{m.sectionTitle("≡ Detail", width, false), fitLine(" "+t.Text, width), fitLine(" "+t.Status+" · "+assignee+" · "+m.taskUsage(t.ID), width), fitLine(" "+t.ID, width)}
}

func (m Model) sectionTitle(label string, width int, focused bool) string {
	st := lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true)
	return st.Render(fitLine(" "+label, width))
}

// dashboardPanel is a web-style card: rounded Border-coloured frame (BorderFocus
// when focused); the top edge carries the bold header, "Title · hint" shows the
// hint muted at the right. Same height as before: header sits on the top edge.
func (m Model) dashboardPanel(title string, lines []string, width, height int, focused bool) string {
	width = max(4, width)
	height = max(3, height)
	color := m.theme.Border
	if focused {
		color = m.theme.BorderFocus
	}
	var content []string
	for _, line := range lines {
		if len(content) >= height-2 {
			break
		}
		content = append(content, fitLine(line, width-4))
	}
	for len(content) < height-2 {
		content = append(content, "")
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), false, true, true, true).BorderForeground(color).Padding(0, 1).Width(width - 2).Render(strings.Join(content, "\n"))
	title, hint, _ := strings.Cut(title, " · ")
	bc := lipgloss.NewStyle().Foreground(color)
	label := " " + fitLine(title, max(1, width-6)) + " "
	right := ""
	if hint != "" {
		right = " " + fitLine(hint, max(1, width-6-lipgloss.Width(label))) + " "
		if lipgloss.Width(label)+lipgloss.Width(right) > width-4 {
			right = ""
		}
	}
	fill := max(0, width-2-1-lipgloss.Width(label)-lipgloss.Width(right))
	top := bc.Render("╭─") + lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true).Render(label) +
		bc.Render(strings.Repeat("─", fill)) + lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render(right) + bc.Render("╮")
	return top + "\n" + box
}

// agentRows is the Agents card body: running agents (definitions live in Roles).
func (m Model) agentRows(width, height int) []string {
	running := m
	running.rows = nil
	for _, row := range m.rows {
		if !row.IsRepoAgent && !row.IsHeader { // window headers ("1 split") are noise here
			running.rows = append(running.rows, row)
		}
	}
	return running.agentLines(width, height+1)[1:]
}

func (m Model) roleCount() int {
	n := 0
	for _, row := range m.rows {
		if row.IsRepoAgent && row.RepoAgentName != "" {
			n++
		}
	}
	return n
}

// roleLines is the Roles card body (agent definitions) scrolled so the
// selected one stays visible; start is the index of the first line shown.
func (m Model) roleLines(width, visible int) ([]string, int) {
	var defs []string
	group := ""
	for _, row := range m.rows {
		if !row.IsRepoAgent {
			continue
		}
		if row.RepoAgentName == "" {
			group = row.Header
			continue
		}
		mark, tag := "⌂ ", "local"
		if group == "Global agents" {
			mark, tag = "◇ ", "global"
		}
		if m.agentFocus == 1 && len(defs) == m.definitionCursor {
			mark = "▶ "
		}
		left := mark + row.RepoAgentName
		pad := max(1, width-lipgloss.Width(left)-lipgloss.Width(tag))
		defs = append(defs, left+strings.Repeat(" ", pad)+tag)
	}
	start := 0
	if m.agentFocus == 1 && visible > 0 && len(defs) > visible {
		start = min(max(0, m.definitionCursor-visible+1), len(defs)-visible)
	}
	return defs[start:], start
}

func (m Model) focusedDefinitions() []roles.Definition {
	harness, _ := tmux.GetOption(tmux.AgentOption)
	defs := roles.ListGlobal(harness)
	selected, _ := paneIDAt(m.rows, m.cursor)
	if dir := m.focusDir(selected); dir != "" {
		defs = append(defs, tasks.LocalDefinitions(dir, harness)...)
	}
	return defs
}

// companionPanel returns the Companion panel's height (0 when the avatar is
// off) and its lines.
func (m Model) companionPanel(width int) (int, []string) {
	if !m.avatar.enabled() {
		return 0, nil
	}
	preview := m
	preview.height = 0
	avatar := preview.renderAvatar(width - 4)
	if n := len(avatar); n > 3 && avatar[n-1] == "" {
		avatar = avatar[:n-1] // the sidebar's spacer row, not needed in a panel
	}
	return len(avatar) + 2, avatar
}

type rect struct{ x, y, w, h int }

// dashGeom is every card's rectangle (h == 0: not shown), shared by the view
// and mouse hit-testing. y counts from the top bar (row 0).
type dashGeom struct {
	wide                                  bool
	agents, usage, comp, roles, tasks, wf rect
	usageLines                            []string
}

// dashGeometry mirrors the web home: wide = left column (Agents over Usage,
// then Companion/Roles when rows remain) beside Tasks (+ Workflows);
// narrow = Agents, Usage, Tasks stacked, Companion/Roles after when rows remain.
func (m Model) dashGeometry() dashGeom {
	w, h := m.width, m.height
	if w <= 0 {
		w = fallbackWidth
	}
	if h <= 0 {
		h = 20
	}
	width := max(6, w-1)
	bh := max(6, h-2) // minus top bar and footer
	g := dashGeom{wide: w >= 90}
	left := width
	if g.wide {
		left = min(44, max(38, width*40/100))
	}
	g.usageLines = m.usageLines(left-4, bh)
	usageH := len(g.usageLines) + 2
	rolesH := 0
	if n := m.roleCount(); n > 0 || m.agentFocus == 1 {
		rolesH = max(3, min(n, 4)+2)
	}
	compH, _ := m.companionPanel(left)
	const minAgents, minTasks = 5, 6
	if g.wide {
		if compH > 0 && bh-usageH-rolesH-compH < 8 {
			compH = 0
		}
		if rolesH > 0 && m.agentFocus != 1 && bh-usageH-rolesH < 6 {
			rolesH = 0
		}
		agentsH := max(minAgents, bh-usageH-rolesH-compH)
		y := 1
		stack := func(r *rect, hh int) {
			if hh > 0 {
				*r = rect{0, y, left, hh}
				y += hh
			}
		}
		stack(&g.agents, agentsH)
		stack(&g.usage, usageH)
		stack(&g.comp, compH)
		stack(&g.roles, rolesH)
		g.tasks = rect{left, 1, width - left, bh}
	} else {
		extras := 0
		if compH > 0 && bh-usageH-compH >= minAgents+minTasks+8 {
			extras += compH
		} else {
			compH = 0
		}
		if rolesH > 0 && bh-usageH-extras-rolesH >= minAgents+minTasks+4 {
			extras += rolesH
		} else {
			rolesH = 0
		}
		rem := bh - usageH - extras
		agentsH := max(minAgents, rem*2/5)
		tasksH := max(minTasks, rem-agentsH)
		y := 1
		g.agents = rect{0, y, width, agentsH}
		y += agentsH
		g.usage = rect{0, y, width, usageH}
		y += usageH
		g.tasks = rect{0, y, width, tasksH}
		y += tasksH
		if compH > 0 {
			g.comp = rect{0, y, width, compH}
			y += compH
		}
		if rolesH > 0 {
			g.roles = rect{0, y, width, rolesH}
		}
	}
	// Workflows card under the columns when they keep 8+ rows.
	if wh := m.workflowCardHeight(g.tasks.h - 1); wh > 0 {
		g.wf = rect{g.tasks.x, g.tasks.y + g.tasks.h - wh, g.tasks.w, wh}
		g.tasks.h -= wh
	}
	return g
}

// usageLines is the Usage card body: what needs the user, then provider
// quotas (when the pane is tall enough) and tokens with the sparkline.
func (m Model) usageLines(width, bodyHeight int) []string {
	dim := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	attention, now := metrics.OverviewFacts(m.scopePanes(""), m.notes, m.metricState)
	var out []string
	if attention.Needed {
		out = append(out, lipgloss.NewStyle().Foreground(m.theme.StatusConfirm).Bold(true).Render(fitLine(m.icons.Warning+" "+attention.Text, width)))
	} else {
		out = append(out, dim.Render(fitLine("✓ "+attention.Text, width)))
	}
	if len(now) > 0 {
		out = append(out, lipgloss.NewStyle().Foreground(m.theme.StatusRunning).Render(fitLine("Now  "+strings.Join(now, " · "), width)))
	}
	if bodyHeight >= 22 { // short panes keep the space for agents and tasks
		q := quota.Get()
		out = append(out, m.providerMeters("Claude", q.Claude)...)
		out = append(out, m.providerMeters("Codex", q.Codex)...)
	}
	today := metrics.TokenWindow(m.metricState, time.Now(), 7).Today
	out = append(out, dim.Render("Tokens today ")+lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true).Render(compactTokens(today)), m.chartLine(width))
	return out
}

func (r rect) contains(x, y int) bool {
	return r.h > 0 && x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

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
	inner := max(1, w-3)
	repo := m.notesScope
	if i := strings.LastIndexByte(repo, '-'); i > 0 {
		repo = repo[:i]
	}
	if repo == "" {
		repo = "current repo"
	}
	overview := m.overviewLines(inner - 2)
	width := max(6, w-1)
	top := m.dashboardPanel("▥ Overview · "+repo, overview, width, len(overview)+2, false)
	avatarRows, avatar := m.companionPanel(width)
	if avatarRows > 0 {
		top = m.dashboardPanel("◈ Companion", avatar, width, avatarRows, false) + "\n" + top
	}
	g := m.dashGeometry(avatarRows)
	taskTitle := fmt.Sprintf("☷ Tasks · %d", len(m.notes))
	if m.taskTitle {
		taskTitle += " · Enter board"
	}
	var body string
	if w >= 90 {
		agents := m.agentPanels(g.left, g.bodyHeight)
		taskRows := m.taskLines(width-g.left-4, g.second-2)
		right := m.dashboardPanel(taskTitle, taskRows[1:], width-g.left, g.second, m.taskFocus)
		body = lipgloss.JoinHorizontal(lipgloss.Top, agents, right)
	} else {
		agents := m.agentPanels(width, g.first)
		taskRows := m.taskLines(width-4, g.second-2)
		body = agents + "\n" + m.dashboardPanel(taskTitle, taskRows[1:], width, g.second, m.taskFocus)
	}
	help := " ? help · Tab focus · ↑↓ select · Enter open · l log · i prompt"
	if m.agentFocus == 1 {
		help = " ↑↓ agent · e edit instruction"
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
	footer := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render(fitLine(help, width))
	result := strings.Split(top+"\n"+body+"\n"+footer, "\n")
	if len(result) > h {
		result = append(result[:h-1], footer)
	}
	for len(result) < h {
		result = append(result, "")
	}
	return strings.Join(result, "\n")

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

func (m Model) providerMeters(name string, p quota.Provider) string {
	s := lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true).Render(name) + " " + m.meter("5h", p.Five.Used) + "  " + m.meter("7d", p.Week.Used)
	if p.Stale {
		s += lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render(" stale")
	}
	return s
}

func (m Model) chartDaysOrDefault() int {
	if m.chartDays == 0 {
		return 7
	}
	return m.chartDays
}

// rangeSelector is "1d [7d] 30d" with the active range emphasised.
func (m Model) rangeSelector() string {
	on := lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true)
	off := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	var parts []string
	for _, d := range []int{1, 7, 30} {
		if d == m.chartDaysOrDefault() {
			parts = append(parts, on.Render(fmt.Sprintf("[%dd]", d)))
		} else {
			parts = append(parts, off.Render(fmt.Sprintf("%dd", d)))
		}
	}
	return strings.Join(parts, " ")
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
func (m Model) taskLines(width, height int) []string {
	out := []string{m.sectionTitle("☷ Tasks", width, m.taskFocus)}
	if len(m.notes) == 0 {
		out = append(out, m.boardStyle(m.theme.TextMuted).Render(" No tasks yet."))
	}
	// Web look: flat list in board-column order, one row per ticket.
	var lines []string
	cursorLine := 0
	for _, status := range m.boardColumns() {
		for _, idx := range m.boardStatusCards(status) {
			if idx == m.taskCursor {
				cursorLine = len(lines)
			}
			lines = append(lines, m.taskRow(idx, status, width))
		}
	}
	start := max(0, cursorLine-max(1, height-3))
	for i := start; i < len(lines) && len(out) < height; i++ {
		out = append(out, lines[i])
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

// taskRow is the web TasksPanel row: ● status, short id, title, then blocked tag, live dot and role avatar.
func (m Model) taskRow(idx int, status string, width int) string {
	t := m.notes[idx]
	th := m.theme
	paint := func(c lipgloss.Color) lipgloss.Style {
		st := lipgloss.NewStyle().Foreground(c)
		if idx == m.taskCursor {
			st = st.Background(th.SelectionBg)
		}
		return st
	}
	dot := m.boardColor(status)
	switch {
	case m.boardConfig.Lane(status) == "done":
		dot = th.TextDim
	case strings.HasPrefix(status, "review"):
		dot = th.StatusWaiting
	}
	text := th.TextPrimary
	if m.boardConfig.Lane(status) == "done" {
		text = th.TextDim
	}
	id := t.ID
	if strings.Trim(id, "0123456789") != "" && len(id) > 4 {
		id = id[max(0, len(id)-4):]
	}
	id = "#" + id
	type seg struct {
		s string
		c lipgloss.Color
	}
	var right []seg
	if status == "blocked" {
		right = append(right, seg{" blocked", th.StatusError})
	}
	if strings.HasPrefix(ansi.Strip(m.boardWorker(t)), "●") { // live = active pane
		right = append(right, seg{" ●", th.StatusRunning})
	}
	role := ""
	if len(t.Assignees) > 0 {
		role = t.Assignees[0]
	} else {
		for _, s := range m.boardConfig.Statuses {
			if s.ID == status {
				role = s.Role
			}
		}
	}
	if role != "" {
		r := []rune(strings.ToUpper(role))
		right = append(right, seg{" " + string(r[:min(2, len(r))]), th.TextMuted})
	}
	rw := 0
	for _, r := range right {
		rw += lipgloss.Width(r.s)
	}
	const idW = 5
	room := max(0, width-2-2-idW-1-rw)
	title := ansi.Truncate(strings.Join(strings.Fields(boardTitle(t)), " "), room, "…")
	var b strings.Builder
	b.WriteString(paint(th.TextPrimary).Render("  "))
	b.WriteString(paint(dot).Render("● "))
	b.WriteString(paint(th.TextMuted).Render(fmt.Sprintf("%-*s ", idW, id)))
	b.WriteString(paint(text).Render(title + strings.Repeat(" ", max(0, room-ansi.StringWidth(title)))))
	for _, r := range right {
		b.WriteString(paint(r.c).Render(r.s))
	}
	b.WriteString(paint(text).Render(strings.Repeat(" ", max(0, width-lipgloss.Width(b.String())))))
	return b.String()
}

func statusGlyph(s string) string {
	switch s {
	case "done":
		return "✔"
	case "doing":
		return "◐"
	case "blocked":
		return "✖"
	}
	return "○"
}
func (m Model) taskDetail(width int) []string {
	if len(m.notes) == 0 || m.taskCursor >= len(m.notes) {
		return []string{m.sectionTitle("≡ Detail", width, false), "  select a task"}
	}
	t := m.notes[m.taskCursor]
	n, missing := metrics.TaskTokens(m.metricState, t.ID)
	usage := "tokens —"
	if missing == 0 {
		found := false
		for _, r := range m.metricState.Runs {
			if r.TaskID == t.ID {
				found = true
				break
			}
		}
		if found {
			usage = "tokens " + compactTokens(n)
		}
	} else if n > 0 {
		usage = "tokens ≥" + compactTokens(n)
	}
	assignee := "unassigned"
	if len(t.Assignees) > 0 {
		assignee = strings.Join(t.Assignees, ", ")
	}
	return []string{m.sectionTitle("≡ Detail", width, false), fitLine(" "+t.Text, width), fitLine(" "+t.Status+" · "+assignee+" · "+usage, width), fitLine(" "+t.ID, width)}
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

func (m Model) agentPanels(width, height int) string {
	running := m
	running.rows = nil
	var defs []string
	group := ""
	for _, row := range m.rows {
		if !row.IsRepoAgent {
			if !row.IsHeader { // window headers ("1 split") are noise here
				running.rows = append(running.rows, row)
			}
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
		pad := max(1, width-4-lipgloss.Width(left)-lipgloss.Width(tag))
		defs = append(defs, left+strings.Repeat(" ", pad)+tag)
	}
	small := max(4, height/3)
	// Scroll the definitions so the selected one stays visible.
	if visible := small - 2; m.agentFocus == 1 && len(defs) > visible {
		start := min(max(0, m.definitionCursor-visible+1), len(defs)-visible)
		defs = defs[start:]
	}
	runHeight := max(4, height-small)
	rows := running.agentLines(width-4, runHeight-2)
	return m.dashboardPanel("◉ Running agents", rows[1:], width, runHeight, m.agentFocus == 0) + "\n" + m.dashboardPanel("◇ Agents", defs, width, small, m.agentFocus == 1)
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

type dashGeom struct{ top, bodyHeight, left, first, second int }

// dashGeometry is the dashboard layout shared by the view and mouse
// hit-testing: top = rows above the body, left = agents column width (wide),
// first/second = panel heights when stacked (narrow); wide tasks take the whole right column.
func (m Model) dashGeometry(avatarRows int) dashGeom {
	w, h := m.width, m.height
	if w <= 0 {
		w = fallbackWidth
	}
	if h <= 0 {
		h = 20
	}
	width := max(6, w-1)
	g := dashGeom{top: avatarRows + len(m.overviewLines(max(1, w-5))) + 2, bodyHeight: max(6, h-len(m.overviewLines(max(1, w-5)))-3-avatarRows), left: width / 2}
	if w >= 90 {
		g.second = g.bodyHeight
		return g
	}
	g.second = max(6, g.bodyHeight/2) // the list scrolls
	g.first = max(8, g.bodyHeight-g.second)
	return g
}

// overviewLines is the Overview panel: what needs the user first, then the
// state of agents, tasks and the repo, then recent usage.
func (m Model) overviewLines(width int) []string {
	dim := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	attention, now := metrics.OverviewFacts(m.scopePanes(""), m.notes, m.metricState)
	var out []string
	if attention.Needed {
		out = append(out, lipgloss.NewStyle().Foreground(m.theme.StatusConfirm).Bold(true).Render(fitLine(m.icons.Warning+" "+attention.Text, width)))
	} else {
		out = append(out, dim.Render("✓ "+attention.Text))
	}
	if len(now) > 0 {
		out = append(out, lipgloss.NewStyle().Foreground(m.theme.StatusRunning).Render(fitLine("Now  "+strings.Join(now, " · "), width)))
	}
	if m.height >= 30 { // short panes keep the space for agents and tasks
		q := quota.Get()
		out = append(out, fitLine(m.providerMeters("Claude", q.Claude)+"   "+m.providerMeters("Codex", q.Codex), width))
	}
	// Tokens: today and the window the charts show.
	today := metrics.TokenWindow(m.metricState, time.Now(), 7).Today
	tokens := lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render("Tokens today ") + lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true).Render(compactTokens(today))
	if sel := m.rangeSelector(); width >= 28 {
		tokens += strings.Repeat(" ", max(2, width-lipgloss.Width(tokens)-lipgloss.Width(sel))) + sel
	}
	out = append(out, fitLine(tokens, width), m.chartLine(width))
	return out
}

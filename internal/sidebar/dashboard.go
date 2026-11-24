package sidebar

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/metrics"
	"tagent/internal/quota"
	"tagent/internal/tasks"
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

	focus := func(f int) bool { return m.focus == f }
	usageName, uw := m.cardName(paneNum(3), "▥ Usage")
	rng, _ := m.tabStrip([]string{"1d", "7d", "30d"}, slices.Index([]int{1, 7, 30}, m.chartDaysOrDefault()), 3+uw+2)
	agentsName, _ := m.cardName(paneNum(0), "Agents")
	agentsPos := position(m.cursor, paneRowCount(m.rows))
	if slices.Contains(slices.Collect(maps.Values(m.sizeOwners)), true) {
		agentsPos += " · f handoff"
	}
	card := func(r rect, title, pos string, lines []string, focused bool) string {
		return m.boardPanel(title, pos, lines, r.w, r.h, focused)
	}
	cards := map[string]string{
		"agents": card(g.agents, agentsName, agentsPos, m.agentRows(g.agents.w-4, g.agents.h-2), focus(0)),
		"usage":  card(g.usage, usageName+"  "+rng, "", g.usageLines, focus(3)),
		"tasks":  m.tasksCard(g.tasks.w, g.tasks.h, paneNum(2), focus(2)),
		"wf":     m.workflowCard(g.wf.w, g.wf.h, paneNum(4)),
	}
	if g.comp.h > 0 {
		_, avatar := m.companionPanel(g.comp.w)
		cards["comp"] = card(g.comp, m.boardStyle(m.theme.TextPrimary).Bold(true).Render("◈ Companion"), "", avatar, false)
	}
	stack := func(names ...string) string {
		var parts []string
		for _, n := range names {
			if c, ok := cards[n]; ok {
				parts = append(parts, c)
			}
		}
		return strings.Join(parts, "\n")
	}
	var body string
	if m.agentsOpen && m.agentDetail { // like the task detail: the whole pane
		body = m.workflowCard(w, max(3, h-2), 0)
	} else if g.wide {
		body = lipgloss.JoinHorizontal(lipgloss.Top, stack("agents", "usage", "comp"), stack("tasks", "wf"))
	} else {
		body = stack("agents", "usage", "tasks", "wf", "comp")
	}
	help := m.footerHints()
	if m.taskDelete {
		help = " Delete task?  y confirm · n cancel"
	}
	if m.taskNotice != "" {
		help = " " + m.taskNotice
	}
	footer := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render(fitLine(help, max(6, w-1)))
	result := strings.Split(bar+"\n"+body, "\n")
	result = result[:min(len(result), h-1)]
	for len(result) < h-1 { // safety net: the cards normally fill the body
		result = append(result, "")
	}
	result = append(result, footer)
	return strings.Join(result, "\n")
}

// topBar is the one-line header: "tagent · repo" left, key hints right.
func (m Model) topBar(repo string, width int) string {
	left := lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true).Render("tagent") +
		lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render(" · "+repo)
	right := "? help · , settings"
	if width < 40+lipgloss.Width(right) { // narrow: the web status outranks the key hints
		right = m.webStatus
	} else if m.webStatus != "" {
		right = m.webStatus + " · " + right
	}
	if right == "" || width < lipgloss.Width(left)+1+lipgloss.Width(right) {
		return fitLine(left, width)
	}
	pad := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", pad) + lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render(right)
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

// quotaCell is "● 88%": a dot colored like the web Meter (green <70, amber >=70, red >=90, muted when unknown).
func (m Model) quotaCell(used *float64) string {
	if used == nil {
		return lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render("○  –")
	}
	v := max(0, min(100, *used))
	color := m.theme.StatusRunning
	if v >= 90 {
		color = m.theme.StatusError
	} else if v >= 70 {
		color = m.theme.StatusWaiting
	}
	return lipgloss.NewStyle().Foreground(color).Render("●") + lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Render(fmt.Sprintf(" %3.0f%%", v))
}

// quotaGrid is the provider quotas as a grid: a 5h/7d header over one row per provider.
func (m Model) quotaGrid(ps map[string]quota.Provider) []string {
	dim := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	out := []string{dim.Render(fmt.Sprintf("%-7s %-7s %s", "", "5h", "7d"))}
	for _, name := range []string{"Claude", "Codex"} {
		p := ps[name]
		row := lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true).Render(fmt.Sprintf("%-7s", name)) + " " + m.quotaCell(p.Five.Used) + "  " + m.quotaCell(p.Week.Used)
		if p.Stale {
			row += dim.Render(" stale")
		}
		out = append(out, row)
	}
	return out
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
	out := []string{m.sectionTitle("◉ Running agents", width, m.focus != cardTasks)}
	cursor := m.cursor
	if m.focus == cardTasks {
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
		// No row is selected while another card has focus: drop the cursor column.
		if cursor == -1 {
			row = strings.TrimPrefix(row, noCursorMarker)
		}
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

// taskUsage is "tokens 12.3k" (≥ when some jobs lack usage, — when none).
func (m Model) taskUsage(t tasks.Task) string {
	u := metrics.TicketUsageOf(m.metricState, t, m.boardConfig, m.boardHistory, time.Now())
	switch {
	case len(u.Jobs) == 0 || u.Total.Total == 0 && u.Partial:
		return "tokens —"
	case u.Partial:
		return "tokens ≥" + compactTokens(u.Total.Total)
	}
	return "tokens " + compactTokens(u.Total.Total)
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
	return []string{m.sectionTitle("≡ Detail", width, false), fitLine(" "+t.Text, width), fitLine(" "+t.Status+" · "+assignee+" · "+m.taskUsage(t), width), fitLine(" "+t.ID, width)}
}

func (m Model) sectionTitle(label string, width int, focused bool) string {
	st := lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true)
	return st.Render(fitLine(" "+label, width))
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
	wide                           bool
	agents, usage, comp, tasks, wf rect
	usageLines                     []string
}

// focusOrder is the Tab order (focus values): Agents, Usage, Tasks,
// Workflows. Its index + 1 is the card's [n] number.
const (
	cardAgents    = 0
	cardUsage     = 3
	cardTasks     = 2
	cardWorkflows = 4
)

var focusOrder = []int{cardAgents, cardUsage, cardTasks, cardWorkflows}

func paneNum(focus int) int { return slices.Index(focusOrder, focus) + 1 }

// Card minimum heights (borders included): a tall column gives the spare rows
// to the cards that grow, a short one keeps these for as long as it can.
const (
	minAgentsH = 8
	minUsageH  = 8
	minTasksH  = 6
	minWfH     = 8
)

// dashGeometry: wide = left column (Agents, Usage, Companion)
// beside Tasks over Workflows; narrow or short = all cards stacked in Tab
// order. Every card is always present and the cards together fill the body
// (h-2 rows) exactly: each keeps a minimum height, Tasks and Workflows take
// the spare rows, and a short column shrinks the cards instead of leaving a
// gap under the last one (split).
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
	compH, _ := m.companionPanel(left)
	if compH > 0 && bh-compH < map[bool]int{true: 15, false: 24}[g.wide] {
		compH = 0
	}
	// Card sizes follow the viewport, not the content: Usage and the bottom
	// Workflows card are fixed small, Agents takes a set share (all spare rows
	// in the left column when wide), Tasks takes the rest. Lists scroll inside.
	// The focused card is the last to lose rows in a short column.
	last := func(c constraint, focus int) constraint { return c.Last(m.focus == focus) }
	agentsC := func(c constraint) constraint { return last(c.AtLeast(minAgentsH), 0) }
	usageC := func(n int) constraint { return last(Fixed(n).AtLeast(minUsageH), 3) }
	tasksC := last(Fill(1).AtLeast(minTasksH), 2)
	wfC := last(Fixed(minWfH), 4)
	if m.agentsOpen && m.agentDetail { // the detail replaces the list: it takes the room Tasks would use
		wfC = Fill(4).AtLeast(minWfH).Last(true)
	}
	set := func(rs []rect, dst ...*rect) {
		for i, d := range dst {
			*d = rs[i]
		}
	}
	body := rect{0, 1, width, bh}
	if g.wide {
		cols := hsplit(body, Fixed(left), Fill(1))
		lc, rc := cols[0], cols[1]
		lc.h -= compH
		set(vsplit(lc, agentsC(Fill(1)), usageC(minUsageH)), &g.agents, &g.usage)
		if compH > 0 {
			g.comp = rect{0, 1 + bh - compH, left, compH}
		}
		set(vsplit(rc, tasksC, wfC), &g.tasks, &g.wf)
	} else {
		body.h -= compH
		set(vsplit(body, agentsC(Percent(28)), usageC(max(minUsageH, usageH)), tasksC, wfC), &g.agents, &g.usage, &g.tasks, &g.wf)
		if compH > 0 {
			g.comp = rect{0, 1 + bh - compH, width, compH}
		}
	}
	return g
}

// quotaGet reads the provider quotas; tests stub it so the golden does not depend on this machine.
var quotaGet = quota.Get

// usageLines is the Usage card body: what runs now, then provider
// quotas (when the pane is tall enough) and tokens with the sparkline.
func (m Model) usageLines(width, bodyHeight int) []string {
	dim := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	_, now := metrics.OverviewFacts(m.scopePanes(""), m.notes, m.metricState)
	var out []string
	if len(now) > 0 {
		out = append(out, lipgloss.NewStyle().Foreground(m.theme.StatusRunning).Render(fitLine("Now  "+strings.Join(now, " · "), width)))
	}
	if bodyHeight >= 22 { // short panes keep the space for agents and tasks
		q := quotaGet()
		out = append(out, m.quotaGrid(map[string]quota.Provider{"Claude": q.Claude, "Codex": q.Codex})...)
	}
	today := metrics.TokenWindow(m.metricState, time.Now(), 7).Today
	out = append(out, dim.Render("Tokens today ")+lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true).Render(compactTokens(today)), m.chartLine(width))
	return out
}

func (r rect) contains(x, y int) bool {
	return r.h > 0 && x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

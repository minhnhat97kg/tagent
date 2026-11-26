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

func (m Model) dashboardView() string {
	if m.taskForm != "" {
		return m.taskFormView()
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
	sx, sy := g.clampScroll(m.scrollX, m.scrollY)
	if m.layoutEdit { // the pane being moved shows the heavy border; the companion has no card focus
		m.focus = cardFocus(m.layoutSel)
	}
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
	card := func(r rect, title, pos string, lines []string, focused bool) string {
		return m.boardPanel(title, pos, lines, r.w, r.h, focused)
	}
	cards := map[string]string{
		"agents":    m.agentsCard(g.agents.w, g.agents.h, focus(0)),
		"usage":     card(g.usage, usageName+"  "+rng, "", g.usageLines, focus(3)),
		"tasks":     m.tasksCard(g.tasks.w, g.tasks.h, paneNum(2), focus(2)),
		"workflows": m.workflowCard(g.wf.w, g.wf.h, paneNum(4)),
	}
	if g.comp.h > 0 {
		_, avatar := m.companionPanel(g.comp.w)
		cards["comp"] = card(g.comp, m.boardStyle(m.theme.TextPrimary).Bold(true).Render("◈ Companion"), "", avatar, m.layoutEdit && m.layoutSel == "comp")
	}
	var body string
	if m.subView() {
		body = m.subViewBody(w, max(3, h-2))
	} else {
		body = strings.Join(composite(g.rects(), cards, sx, sy, g.bw, g.bh), "\n")
	}
	help := m.footerHints()
	if m.taskNotice != "" {
		help = " " + m.taskNotice
	}
	switch { // a confirm prompt is never hidden by a notice
	case m.workflowDelete != "":
		help = "Delete workflow " + m.workflowDelete + "? y / n"
	case m.agentDelete != "":
		help = "Delete agent " + m.agentDelete + "? y / n"
	case m.taskDelete:
		help = " Delete task?  y confirm · n cancel"
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

// subViewBody is a sub-view's body (subView), w columns by h rows: the board
// config, the workflow view, the ticket detail (beside the tickets when wide) or
// an agent's detail.
func (m Model) subViewBody(w, h int) string {
	switch {
	case m.boardConfigView:
		lines := m.boardConfigLines(w - 4)
		rows := max(0, h-2)
		start := min(m.boardConfigScroll, max(0, len(lines)-rows))
		return m.boardPanel("Board config · .tasks/board.yaml", "B", lines[start:], w, h, true)
	case m.boardFlow:
		label := ".tasks/board.yaml"
		if m.flowWorkflow != "" {
			label = m.flowWorkflow
		} else if len(m.notes) > 0 && m.notes[m.taskCursor].Workflow != "" {
			label = m.notes[m.taskCursor].Workflow
		}
		return m.boardPanel("Workflow · "+label, "", m.flowLines(w-4), w, h, true)
	case m.boardDrawer && w >= 100:
		dw := w * 2 / 5
		left := m.tasksCardAt(min(tasksSizeFor(w-dw, h), sizeList), w-dw, h, 0, false)
		right := m.boardPanel(m.detailTabTitle(), "", m.boardDetail(dw-4, h-2), dw, h, true)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	case m.boardDrawer:
		return m.boardPanel(m.detailTabTitle(), "", m.boardDetail(w-4, h-2), w, h, true)
	}
	return m.agentsCard(w, h, true) // the agent detail
}

// agentsCard is the Agents card at width x height: the Running agents, or the
// Definitions tab (t swaps; the detail takes the whole body, see subView).
func (m Model) agentsCard(width, height int, focused bool) string {
	tabs, _ := m.tabStrip([]string{"Running", "Definitions"}, b2i(m.agentsOpen), 3)
	if m.agentsOpen {
		pos := position(m.agentCursor, len(m.agents))
		if m.agentDetail {
			pos = ""
		}
		return m.boardPanel(tabs, pos, m.agentDefsLines(width-4, height-2), width, height, focused)
	}
	pos := position(m.cursor, paneRowCount(m.rows))
	if slices.Contains(slices.Collect(maps.Values(m.sizeOwners)), true) {
		pos += " · f handoff"
	}
	return m.boardPanel(tabs, pos, m.agentRows(width-4, height-2), width, height, focused)
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
// Under 3 body rows (a card under 5 rows) it is a count: agents, running, waiting, other.
func (m Model) agentRows(width, height int) []string {
	if height < 3 {
		total, active, waiting := 0, 0, 0
		for _, row := range m.rows {
			if !isSelectablePaneRow(row) {
				continue
			}
			total++
			switch {
			case row.Status == tmux.StatusWaiting:
				waiting++
			case row.Status.IsActive():
				active++
			}
		}
		text := fmt.Sprintf("%d agents · %d running · %d waiting · %d other", total, active, waiting, total-active-waiting)
		return strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
	}
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
	bw, bh                         int // the body (the viewport the canvas scrolls in), in columns and rows
	agents, usage, comp, tasks, wf rect
	usageLines                     []string
}

// rects are the cards by layout id (see layoutIDs); a card with h 0 is not shown.
func (g dashGeom) rects() map[string]rect {
	return map[string]rect{"agents": g.agents, "usage": g.usage, "comp": g.comp, "tasks": g.tasks, "workflows": g.wf}
}

// interior reports whether (x, y) is inside a card's border.
func (g dashGeom) interior(x, y int) bool {
	for _, r := range g.rects() {
		if r.h > 0 && x > r.x && x < r.x+r.w-1 && y > r.y && y < r.y+r.h-1 {
			return true
		}
	}
	return false
}

// clampScroll keeps the canvas offset on the canvas: it may overflow right and
// down, never past the far edge of its last card.
func (g dashGeom) clampScroll(x, y int) (int, int) {
	maxX, maxY := 0, 0
	for _, r := range g.rects() {
		if r.h > 0 {
			maxX = max(maxX, r.x+r.w-g.bw)
			maxY = max(maxY, r.y+r.h-1-g.bh)
		}
	}
	return max(0, min(x, maxX)), max(0, min(y, maxY))
}

// composite draws the window of the canvas at (scrollX, scrollY) that is width x
// height: each line is the cards covering its row, left to right, with spaces
// between them. rects are screen rows (row 0 is the top bar), so y 1 is the
// first body row.
func composite(rects map[string]rect, cards map[string]string, scrollX, scrollY, width, height int) []string {
	lines := make(map[string][]string, len(cards))
	for id, c := range cards {
		lines[id] = strings.Split(c, "\n")
	}
	out := make([]string, height)
	for i := range out {
		row := scrollY + 1 + i
		var covering []string
		for id, r := range rects {
			if _, ok := lines[id]; ok && r.h > 0 && row >= r.y && row < r.y+r.h {
				covering = append(covering, id)
			}
		}
		slices.SortFunc(covering, func(a, b string) int { return rects[a].x - rects[b].x })
		var b strings.Builder
		col := 0
		for _, id := range covering {
			r := rects[id]
			cell := ""
			if k := row - r.y; k < len(lines[id]) {
				cell = lines[id][k]
			}
			b.WriteString(strings.Repeat(" ", max(0, r.x-col)))
			b.WriteString(ansi.Truncate(cell, r.w, ""))
			b.WriteString(strings.Repeat(" ", max(0, r.w-ansi.StringWidth(cell))))
			col = r.x + r.w
		}
		b.WriteString(strings.Repeat(" ", max(0, scrollX+width-col)))
		out[i] = ansi.Cut(b.String(), scrollX, scrollX+width)
	}
	return out
}

// cardFocus is the focus value of a card's layout id (-1: none, the companion).
func cardFocus(id string) int {
	for f, c := range cardIDs {
		if c == id {
			return f
		}
	}
	return -1
}

// cardIDs are the Tab-order cards by focus value, as layout ids.
var cardIDs = map[int]string{cardAgents: "agents", cardUsage: "usage", cardTasks: "tasks", cardWorkflows: "workflows"}

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

// dashGeometry is the cards' rectangles: the saved layout (layout.json, merged
// onto the default) or the layout being edited; with neither, the default
// (defaultGeometry).
func (m Model) dashGeometry() dashGeom {
	def := m.defaultGeometry()
	if m.maximized { // the focused card fills the body; the others and the companion are hidden
		g := dashGeom{wide: def.wide, bw: def.bw, bh: def.bh}
		full := rect{0, 1, def.bw, def.bh}
		switch m.focus {
		case cardAgents:
			g.agents = full
		case cardUsage:
			g.usage = full
		case cardTasks:
			g.tasks = full
		case cardWorkflows:
			g.wf = full
		}
		g.usageLines = m.usageLines(g.usage.w-4, g.usage.h-2)
		return g
	}
	if boxes, ok := m.customBoxes(def); ok {
		return m.geometryFrom(boxes, def)
	}
	return def
}

// defaultGeometry: wide = left column (Agents, Usage, Companion)
// beside Tasks over Workflows; narrow or short = all cards stacked in Tab
// order. Every card is always present and the cards together fill the body
// (h-2 rows) exactly: each keeps a minimum height, Tasks and Workflows take
// the spare rows, and a short column shrinks the cards instead of leaving a
// gap under the last one (split).
func (m Model) defaultGeometry() dashGeom {
	w, h := m.width, m.height
	if w <= 0 {
		w = fallbackWidth
	}
	if h <= 0 {
		h = 20
	}
	width := max(6, w-1)
	bh := max(6, h-2) // minus top bar and footer
	g := dashGeom{wide: m.wideLayout(), bw: width, bh: bh}
	left := width
	if g.wide {
		left = min(44, max(38, width*40/100))
	}
	usageRows := 3 // the card's height: 6 body rows on a tall pane fit the quota grid, else 3
	if bh >= 22 {
		usageRows = 6
	}
	usageH := len(m.usageLines(left-4, usageRows)) + 2
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
	g.usageLines = m.usageLines(g.usage.w-4, g.usage.h-2)
	return g
}

// quotaGet reads the provider quotas; tests stub it so the golden does not depend on this machine.
var quotaGet = quota.Get

// usageLines is the Usage card body for rows body rows: what runs now, then the
// provider quotas, tokens today and the sparkline. Now and tokens always show;
// the sparkline comes next if one row is left, then the quota grid if its three
// rows and width fit too.
func (m Model) usageLines(width, rows int) []string {
	dim := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	_, now := metrics.OverviewFacts(m.scopePanes(""), m.notes, m.metricState)
	var out []string
	if len(now) > 0 {
		out = append(out, lipgloss.NewStyle().Foreground(m.theme.StatusRunning).Render(fitLine("Now  "+strings.Join(now, " · "), width)))
	}
	left := rows - len(out) - 1 // the tokens row
	chart := left >= 1
	left -= b2i(chart)
	if left >= 3 {
		q := quotaGet()
		grid := m.quotaGrid(map[string]quota.Provider{"Claude": q.Claude, "Codex": q.Codex})
		fits := true
		for _, l := range grid {
			fits = fits && ansi.StringWidth(l) <= width
		}
		if fits {
			out = append(out, grid...)
		}
	}
	today := metrics.TokenWindow(m.metricState, time.Now(), 7).Today
	out = append(out, dim.Render("Tokens today ")+lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Bold(true).Render(compactTokens(today)))
	if chart {
		out = append(out, m.chartLine(width))
	}
	return out
}

func (r rect) contains(x, y int) bool {
	return r.h > 0 && x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

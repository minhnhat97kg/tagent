package sidebar

import (
	"fmt"
	"os"
	"os/exec"
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

	focus := func(f int) bool { return m.agentFocus == f }
	usageName, uw := m.cardName(paneNum(3), "▥ Usage")
	rng, _ := m.tabStrip([]string{"1d", "7d", "30d"}, slices.Index([]int{1, 7, 30}, m.chartDaysOrDefault()), 3+uw+2)
	agentsName, _ := m.cardName(paneNum(0), "Agents")
	card := func(r rect, title, pos string, lines []string, focused bool) string {
		return m.boardPanel(title, pos, lines, r.w, r.h, focused)
	}
	cards := map[string]string{
		"agents": card(g.agents, agentsName, position(m.cursor, paneRowCount(m.rows)), m.agentRows(g.agents.w-4, g.agents.h-2), focus(0)),
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
	if g.wide {
		body = lipgloss.JoinHorizontal(lipgloss.Top, stack("agents", "usage", "comp"), stack("tasks", "wf"))
	} else {
		body = stack("agents", "usage", "tasks", "wf", "comp")
	}
	help := " Tab/S-Tab panel · ↑↓ select · Enter open · N new agent · l log · i prompt · b board"
	if m.agentFocus == 3 {
		help = " h/l or [ ] range · Tab/S-Tab panel"
	}
	if m.agentFocus == 4 {
		help = m.keyHints("h/l agents", "enter view", "a add", "Tab focus", "? help")
		if m.agentsOpen {
			help = m.keyHints("h/l workflows", "enter detail", "m model", "a add", "Tab focus", "? help")
		}
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
	if width < 40+lipgloss.Width(right) {
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

// focusOrder is the Tab order (agentFocus values): Agents, Usage, Tasks,
// Workflows. Its index + 1 is the card's [n] number.
var focusOrder = []int{0, 3, 2, 4}

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
// gap under the last one (fitHeights).
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
	// Need is the content height (+2 borders); low is what the card keeps even
	// when the column is short. Weight splits the spare rows: Tasks and
	// Workflows grow first (weight 0 cards only grow when the column has
	// nothing else to fill, e.g. Agents/Usage on the wide left column).
	trim := func(l []string) int {
		for len(l) > 0 && strings.TrimSpace(l[len(l)-1]) == "" {
			l = l[:len(l)-1]
		}
		return len(l)
	}
	wfLines, _, _ := m.stripPanel(width - 4)
	if g.wide {
		wfLines, _, _ = m.stripPanel(width - left - 4)
	}
	tw := width - 4
	if g.wide {
		tw = width - left - 4
	}
	lanes, _ := m.laneLayout(tw)
	type spec struct {
		r      *rect
		focus  int
		need   int
		low    int
		weight int
	}
	agentsS := spec{&g.agents, 0, trim(m.agentRows(left-4, 1000)) + 2, minAgentsH, 0}
	usageS := spec{&g.usage, 3, usageH, minUsageH, 0}
	tasksS := spec{&g.tasks, 2, len(lanes) + 2, minTasksH, 3}
	wfS := spec{&g.wf, 4, min(len(wfLines)+2, minWfH), minWfH, 0} // the bottom card stays small: ~6 rows
	column := func(x, w, y, total int, specs ...spec) {
		low, need, weight := make([]int, len(specs)), make([]int, len(specs)), make([]int, len(specs))
		fi := 0
		for i, s := range specs {
			low[i], need[i], weight[i] = s.low, max(3, s.need), s.weight
			if s.focus == m.agentFocus {
				fi = i
			}
		}
		for i, hh := range fitHeights(total, low, need, weight, fi) {
			*specs[i].r = rect{x, y, w, hh}
			y += hh
		}
	}
	if g.wide {
		column(0, left, 1, bh-compH, agentsS, usageS)
		if compH > 0 {
			g.comp = rect{0, 1 + bh - compH, left, compH}
		}
		column(left, width-left, 1, bh, tasksS, wfS)
	} else {
		// Stacked: Tasks can list hundreds of rows; cap it so Agents and
		// Workflows keep their share and spare rows go to Workflows first.
		tasksS.need = min(tasksS.need, max(minTasksH, (bh-compH)*25/100))
		agentsS.weight, tasksS.weight = 2, 2
		column(0, width, 1, bh-compH, agentsS, usageS, tasksS, wfS)
		if compH > 0 {
			g.comp = rect{0, 1 + bh - compH, width, compH}
		}
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
	} else {
		out = append(out, "") // a fixed row count keeps the card height steady
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

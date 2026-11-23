package sidebar

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/metrics"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
)

type boardConfigMsg struct {
	config tasks.BoardConfig
	err    error
}

func (m Model) openBoard() Model {
	m.boardMode, m.taskFocus, m.agentFocus = true, true, 2
	m.boardDrawer = false
	return m.boardContext()
}

func (m Model) boardContext() Model {
	oldDir := m.boardDir
	m.boardDir = m.repoDir
	if m.boardDir == "" {
		if id, ok := paneIDAt(m.rows, m.cursor); ok {
			if p, found := paneInfoByID(m.sessions, id); found {
				m.boardDir = p.Path
			}
		}
	}
	if m.boardDir == "" {
		m.boardDir, _ = os.Getwd()
	}
	if m.notesScope == "" || oldDir != "" && oldDir != m.boardDir {
		m.notesScope = tasks.Scope(m.boardDir)
		m.notes = tasks.List(m.notesScope)
		m.boardHistory, _ = tasks.ReadEvents(m.notesScope, 0)
		m.attachmentsKey = ""
	}
	m.boardConfig = tasks.BoardConfig{}
	cfg, err := tasks.LoadBoardConfig(m.boardDir)
	if err != nil {
		m.taskNotice = "! Board config: " + err.Error()
	} else {
		m.boardConfig = cfg
	}
	m.setWorkflows(workflow.List(tasks.MainRoot(m.boardDir)))
	m.taskCursor = max(0, min(m.taskCursor, len(m.notes)-1))
	return m
}

func reloadBoardConfig(dir string, editorErr error) boardConfigMsg {
	cfg, err := tasks.LoadBoardConfig(dir)
	if err == nil {
		err = editorErr
	}
	return boardConfigMsg{cfg, err}
}

func boardConfigEditor(dir string) (*exec.Cmd, error) {
	path := tasks.BoardConfigPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_, err = f.WriteString(tasks.BoardExample)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	} else if os.IsExist(err) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", path)
	cmd.Dir = dir
	return cmd, nil
}

// editBoardConfig opens the YAML in a tmux popup over the pane (works in the
// sidebar and shared dashboards, where ExecProcess cannot take the screen).
func editBoardConfig(dir, pane string) tea.Cmd {
	cmd, err := boardConfigEditor(dir)
	if err != nil {
		return func() tea.Msg { return boardConfigMsg{err: err} }
	}
	if pane == "" {
		return tea.ExecProcess(cmd, func(err error) tea.Msg { return reloadBoardConfig(dir, err) })
	}
	return func() tea.Msg {
		script := "cd " + shellQuote(dir) + " && sh -c " + shellQuote(cmd.Args[2]) + " sh " + shellQuote(cmd.Args[4])
		tmux.RunTmux("display-popup", "-E", "-w", "90%", "-h", "90%", "-t", pane, script)
		return reloadBoardConfig(dir, nil)
	}
}

// boardColumns are the configured lanes plus any status tickets still carry
// that the config no longer lists, so no ticket becomes unreachable.
func (m Model) boardColumns() []string {
	cols := slices.Clone(m.configuredColumns())
	for _, t := range m.notes {
		if !slices.Contains(cols, t.Status) {
			cols = append(cols, t.Status)
		}
	}
	return cols
}

func (m Model) configuredColumns() []string {
	if len(m.boardConfig.Columns) > 0 {
		return m.boardConfig.Columns
	}
	if len(m.boardConfig.Statuses) > 0 {
		var columns []string
		for _, s := range m.boardConfig.Statuses {
			columns = append(columns, s.ID)
		}
		return columns
	}
	return tasks.DefaultColumns()
}

// boardGlyph: many statuses, four icons (todo, in progress, blocked, done).
func (m Model) boardGlyph(status string) (string, lipgloss.Color) {
	switch {
	case status == "blocked":
		return "✕", m.theme.StatusError
	case m.boardConfig.Lane(status) == "done":
		return "✓", m.theme.StatusDone
	case m.boardConfig.Lane(status) == "todo":
		return "○", m.theme.TextMuted
	}
	return "◐", m.theme.Accent
}
func (m Model) boardColor(status string) lipgloss.Color { _, c := m.boardGlyph(status); return c }

func boardTitle(t tasks.Task) string {
	if t.Title != "" {
		return t.Title
	}
	return strings.SplitN(t.Text, "\n", 2)[0]
}
func (m Model) boardStyle(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// The board has three lanes however many statuses a workflow defines; a card
// shows its real status.
var boardLanes = []string{"todo", "in progress", "done"}

func (m Model) laneOf(status string) string {
	if l := m.boardConfig.Lane(status); l == "todo" || l == "done" {
		return l
	}
	return "in progress"
}

// laneCards lists the ticket indexes in a lane, in ticket order.
func (m Model) laneCards(lane string) []int {
	var cards []int
	for i, t := range m.notes {
		if m.laneOf(t.Status) == lane {
			cards = append(cards, i)
		}
	}
	return cards
}

func (m Model) activeLane() string {
	if m.taskCursor >= 0 && m.taskCursor < len(m.notes) {
		return m.laneOf(m.notes[m.taskCursor].Status)
	}
	return boardLanes[0]
}

// setNotes replaces the tickets and keeps the selection on the same ticket id.
func (m *Model) setNotes(notes []tasks.Task) {
	id := ""
	if m.taskCursor >= 0 && m.taskCursor < len(m.notes) {
		id = m.notes[m.taskCursor].ID
	}
	m.notes = notes
	if i := slices.IndexFunc(notes, func(t tasks.Task) bool { return t.ID == id }); id != "" && i >= 0 {
		m.taskCursor = i
	}
	m.taskCursor = max(0, min(m.taskCursor, len(notes)-1))
}

// setWorkflows replaces the workflow list and keeps the selection on the same name.
func (m *Model) setWorkflows(ws []workflow.Workflow) {
	name := ""
	if m.workflowCursor >= 0 && m.workflowCursor < len(m.boardWorkflows) {
		name = m.boardWorkflows[m.workflowCursor].Name
	}
	m.boardWorkflows = ws
	if i := slices.IndexFunc(ws, func(w workflow.Workflow) bool { return w.Name == name }); name != "" && i >= 0 {
		m.workflowCursor = i
	}
	m.workflowCursor = max(0, min(m.workflowCursor, len(ws)-1))
}

// laneStep moves the selection within its lane.
func (m Model) laneStep(step int) Model {
	if len(m.notes) == 0 {
		return m
	}
	order := m.laneCards(m.activeLane())
	pos := slices.Index(order, m.taskCursor)
	m.taskCursor = order[max(0, min(len(order)-1, pos+step))]
	m.detailScroll = 0
	return m
}

// selectLane jumps to a lane at the same row; an empty lane has nothing to select.
func (m Model) selectLane(lane string) Model {
	if len(m.notes) == 0 {
		return m
	}
	row := max(0, slices.Index(m.laneCards(m.activeLane()), m.taskCursor))
	if cards := m.laneCards(lane); len(cards) > 0 {
		m.taskCursor = cards[min(row, len(cards)-1)]
		m.detailScroll = 0
	}
	return m
}

// boardNavigateColumn (h/l) moves to the nearest lane that has tickets.
func (m Model) boardNavigateColumn(key string) Model {
	step := 1
	if key == "h" {
		step = -1
	}
	for i := slices.Index(boardLanes, m.activeLane()) + step; i >= 0 && i < len(boardLanes); i += step {
		if len(m.laneCards(boardLanes[i])) > 0 {
			return m.selectLane(boardLanes[i])
		}
	}
	return m
}

type tabSpan struct{ x0, x1 int }

// tabStrip is lazygit's "a · [b] · c" title: the active tab bracketed and bold,
// the others muted. Spans are cell offsets from x, for clicks.
func (m Model) tabStrip(labels []string, active, x int) (string, []tabSpan) {
	muted := m.boardStyle(m.theme.TextMuted)
	var b strings.Builder
	var spans []tabSpan
	for i, l := range labels {
		if i > 0 {
			b.WriteString("  ")
			x += 2
		}
		st, txt := muted, l
		if i == active {
			st, txt = m.boardStyle(m.theme.TextPrimary).Bold(true), "["+l+"]"
		}
		b.WriteString(st.Render(txt))
		spans = append(spans, tabSpan{x, x + ansi.StringWidth(txt)})
		x += ansi.StringWidth(txt)
	}
	return b.String(), spans
}

// cardName is "name" and its width (num is ignored: cards carry no number)
// and the title text starts at x = 3 inside the card (after "╭─ ").
func (m Model) cardName(num int, name string) (string, int) {
	s := m.boardStyle(m.theme.TextPrimary).Bold(true).Render(name)
	return s, ansi.StringWidth(s) // num is unused: cards are not numbered
}

// tasksTitle is "☷ Tasks  [todo 1]  doing 2  done 32" (short lane names so it fits narrow cards).
func (m Model) tasksTitle(num int) (string, []tabSpan) {
	name, w := m.cardName(num, "☷ Tasks")
	labels := make([]string, len(boardLanes))
	for i, l := range boardLanes {
		labels[i] = fmt.Sprintf("%s %d", laneShort(l), len(m.laneCards(l)))
	}
	tabs, spans := m.tabStrip(labels, slices.Index(boardLanes, m.activeLane()), 3+w+2)
	return name + "  " + tabs, spans
}

// laneShort is the lane's name on its tab.
func laneShort(lane string) string {
	if lane == "in progress" {
		return "doing"
	}
	return lane
}

// position is lazygit's "3 of 32", empty for an empty list.
func position(i, n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d of %d", i+1, n)
}

// boardCardLine is a card on one line: "● review #4 Title" (done lane, collapsed cards).
func (m Model) boardCardLine(idx, width int) string {
	t := m.notes[idx]
	mark, style := "  ", m.boardStyle(m.theme.TextPrimary)
	if idx == m.taskCursor {
		mark, style = "▌ ", style.Bold(true).Foreground(m.theme.Accent)
	}
	status := "●" // a lane's own name (todo, done) would repeat the tab: show the status only when it adds something
	if t.Status != m.laneOf(t.Status) {
		status += " " + t.Status
	}
	return boardFit(style.Render(mark)+m.boardStyle(m.boardColor(t.Status)).Render(status)+m.boardStyle(m.theme.TextMuted).Render(" #"+t.ID+" ")+style.Render(boardTitle(t)), width)
}

// boardCard: title, then "● status #id role worker"; a blocked card adds its question.
func (m Model) boardCard(idx, width int) []string {
	t := m.notes[idx]
	if lane := m.laneOf(t.Status); lane == "done" || t.Status == lane { // todo and done cards: one line
		return []string{m.boardCardLine(idx, width)}
	}
	mark := "  "
	style := m.boardStyle(m.theme.TextPrimary)
	if idx == m.taskCursor {
		mark = "▌ "
		style = style.Bold(true).Foreground(m.theme.Accent)
	}
	role := ""
	for _, s := range m.boardConfig.Statuses {
		if s.ID == t.Status {
			role = s.Role
		}
	}
	if role == "" && len(t.Assignees) > 0 {
		role = t.Assignees[0]
	}
	muted := m.boardStyle(m.theme.TextMuted)
	meta := "  " + m.boardStyle(m.boardColor(t.Status)).Render("● "+t.Status) + muted.Render(" #"+t.ID+" "+role)
	if worker := m.boardWorker(t); worker != "" {
		meta += " " + worker
	}
	if len(t.Comments) > 0 {
		meta += muted.Render(fmt.Sprintf(" %s%d", m.icons.Message, len(t.Comments)))
	}
	lines := []string{boardFit(style.Render(mark+boardTitle(t)), width), boardFit(meta, width)}
	if t.Status == "blocked" {
		cfg, _ := tasks.ConfigForTask(m.boardDir, m.boardConfig, t)
		_, _, question := tasks.BlockContext(t, cfg, m.boardHistory)
		if question == "" {
			question = "Blocked: reply with @role"
		}
		lines = append(lines, boardFit(style.Render("  "+strings.Join(strings.Fields(question), " ")), width))
	}
	return lines
}

// laneLayout is the active lane's cards as lines, with the card index owning each line.
func (m Model) laneLayout(width int) (lines []string, owner []int) {
	for _, idx := range m.laneCards(m.activeLane()) {
		for _, l := range m.boardCard(idx, width) {
			lines, owner = append(lines, l), append(owner, idx)
		}
	}
	return
}

// laneWindow is the first visible line, keeping the selected card in view.
func (m Model) laneWindow(owner []int, rows int) int {
	last := slices.Index(owner, m.taskCursor)
	for i := last; i >= 0 && i < len(owner) && owner[i] == m.taskCursor; i++ {
		last = i
	}
	return max(0, last+1-rows)
}

// laneLines is the Tasks card body: the active lane's cards, or just the
// selected one when the card is collapsed to a single row.
func (m Model) laneLines(width, rows int) []string {
	if rows <= 1 {
		if m.taskCursor < len(m.notes) {
			return []string{m.boardCardLine(m.taskCursor, width)}
		}
		return nil
	}
	lines, owner := m.laneLayout(width)
	return lines[min(m.laneWindow(owner, rows), len(lines)):]
}

// tasksCard is the lane card: tabs in the title, "3 of 32" on the bottom border.
func (m Model) tasksCard(width, height, num int, focused bool) string {
	title, _ := m.tasksTitle(num)
	cards := m.laneCards(m.activeLane())
	return m.boardPanel(title, position(slices.Index(cards, m.taskCursor), len(cards)), m.laneLines(width-4, height-2), width, height, focused)
}

// tasksCardClick handles a click at (x, y) inside a Tasks card of this size
// (y 0: the title row, whose tabs pick a lane). card reports a click on a ticket.
func (m Model) tasksCardClick(x, y, width, height, num int) (next Model, card bool) {
	if y == 0 {
		_, spans := m.tasksTitle(num)
		for i, s := range spans {
			if x >= s.x0 && x < s.x1 {
				return m.selectLane(boardLanes[i]), false
			}
		}
		return m, false
	}
	if y >= height-1 || height < 4 {
		return m, false
	}
	lines, owner := m.laneLayout(width - 4)
	if i := m.laneWindow(owner, height-2) + y - 1; i < len(lines) {
		m.taskCursor = owner[i]
		m.detailScroll = 0
		return m, true
	}
	return m, false
}

// workflowLines lists the repo's and global workflows: name, scope, jobs and
// how many tickets are running through each.
func (m Model) workflowLines(width int) []string {
	if len(m.boardWorkflows) == 0 {
		return []string{m.boardStyle(m.theme.TextDim).Render("No workflows · tagent workflow new <name>")}
	}
	dim := m.boardStyle(m.theme.TextMuted)
	var lines []string
	for i, w := range m.boardWorkflows {
		scope := "local"
		if w.Global {
			scope = "global"
		}
		info := fmt.Sprintf("%s · %d jobs", scope, len(w.Jobs))
		if w.Err != nil {
			info = m.boardStyle(m.theme.StatusError).Render("error: " + w.Err.Error())
		}
		active := 0
		for _, t := range m.notes {
			if t.Workflow == w.Name && m.boardConfig.Lane(t.Status) != "done" && t.Status != "todo" {
				active++
			}
		}
		run := ""
		if active > 0 {
			run = "  " + m.boardStyle(m.theme.StatusRunning).Render(fmt.Sprintf("● %d running", active))
		}
		mark := "▸ "
		if m.workflowFocus && i == m.workflowCursor {
			mark = "❯ "
		}
		lines = append(lines, boardFit(mark+m.boardStyle(m.theme.TextPrimary).Bold(true).Render(w.Name)+"  "+dim.Render(info)+run, width))
	}
	return lines
}

// flowLines draws the workflow two lines per status, like ticket rows:
// icon, status, who acts there and how many tickets sit in it; then where it can go.
func (m Model) flowLines(width int) []string {
	selectedWorkflow := ""
	if len(m.notes) > 0 {
		selectedWorkflow = m.notes[m.taskCursor].Workflow
	}
	if m.flowWorkflow != "" {
		w := workflow.Load(m.flowWorkflow)
		if w.Err != nil {
			return []string{w.Err.Error()}
		}
		cfg, err := tasks.ConfigForTask(m.boardDir, m.boardConfig, tasks.Task{Workflow: w.File})
		if err != nil {
			return []string{err.Error()}
		}
		m.boardConfig = cfg
		selectedWorkflow = w.Name
	} else if len(m.notes) > 0 {
		cfg, err := tasks.ConfigForTask(m.boardDir, m.boardConfig, m.notes[m.taskCursor])
		if err != nil {
			return []string{err.Error()}
		}
		m.boardConfig = cfg
	}
	statuses := m.boardConfig.Statuses
	if len(statuses) == 0 {
		for _, id := range m.boardColumns() {
			statuses = append(statuses, tasks.Status{ID: id})
		}
	}
	idw := 0
	for _, s := range statuses {
		idw = max(idw, ansi.StringWidth(s.ID))
	}
	dim := m.boardStyle(m.theme.TextMuted)
	var lines []string
	for _, s := range statuses {
		glyph, color := m.boardGlyph(s.ID)
		who := "you move it"
		switch {
		case s.Role != "":
			who = s.Role
			if r, ok := m.boardConfig.Roles[s.Role]; ok {
				who += " · " + strings.TrimSpace(r.Agent+" "+r.Model)
			}
			working, panes := 0, 0
			for _, r := range m.rows {
				if agent, ticket, ok := strings.Cut(r.BoardTarget, "#"); ok && agent == s.Role && r.PaneID != "" {
					same := false
					for _, t := range m.notes {
						if t.ID == ticket && t.Workflow == selectedWorkflow {
							same = true
						}
					}
					if !same {
						continue
					}
					panes++
					if r.Status.IsActive() {
						working++
					}
				}
			}
			switch {
			case working > 0:
				who += " · " + m.boardStyle(m.theme.StatusRunning).Render(fmt.Sprintf("● %d working", working))
			case panes > 0:
				who += " · ○ idle"
			}
		case s.Run != "":
			who = "ci: " + strings.SplitN(s.Run, "\n", 2)[0]
			if s.Job != "" {
				who = "run: " + strings.SplitN(s.Run, "\n", 2)[0]
			}
		case s.ID == "blocked":
			who = "waiting for you"
		case m.boardConfig.Lane(s.ID) == "done":
			who = "finished"
		}
		n := 0
		for _, t := range m.notes {
			if t.Status == s.ID && t.Workflow == selectedWorkflow {
				n++
			}
		}
		count := ""
		if n > 0 {
			count = fmt.Sprintf("  (%d)", n)
		}
		lines = append(lines, boardFit(m.boardStyle(color).Render(glyph)+" "+m.boardStyle(m.theme.TextPrimary).Bold(true).Render(s.ID+strings.Repeat(" ", idw-ansi.StringWidth(s.ID)))+"  "+who+count, width))
		var next string
		switch {
		case s.Run != "":
			next = "pass → " + s.Pass + " · fail → " + s.Fail
		case s.Next != nil && len(s.Next) == 0:
			next = "end"
		case s.Next != nil:
			next = "→ " + strings.Join(s.Next, ", ")
		case m.boardConfig.Lane(s.ID) == "done":
			next = "end"
		default:
			next = "→ any status"
		}
		if len(s.Needs) > 0 {
			next = "needs " + strings.Join(s.Needs, ", ") + " · " + next
		}
		if s.Job != "" && s.Run == "" {
			next += " · fail → " + s.Failure
		}
		if s.MaxLoops > 0 {
			next += fmt.Sprintf(" · max %d loops, then blocked", s.MaxLoops)
		}
		lines = append(lines, boardFit("  "+strings.Repeat(" ", idw)+dim.Render(next), width))
	}
	return lines
}

// boardFit operates on terminal cells, without Lip Gloss wrapping a second time.
func boardFit(line string, width int) string {
	if width <= 0 {
		return ""
	}
	line = strings.ReplaceAll(strings.ReplaceAll(line, "\n", " "), "\r", "")
	line = ansi.Truncate(line, width, "…")
	return line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
}

// boardPanel draws a card with exact dimensions even in a short split: the
// title (already styled, tabs included) sits inside the top border, pos
// ("3 of 32") on the bottom border. Dashboard and board use their own colours.
func (m Model) boardPanel(title, pos string, lines []string, width, height int, focused bool) string {
	focus, idle := m.theme.BorderFocus, m.theme.Border
	if m.boardMode {
		focus, idle = m.theme.Accent, m.theme.TextDim
	}
	color := idle
	if focused {
		color = focus
	}
	border := m.boardStyle(color)
	// The focused card has a heavy border (┏━┓ ┃ ┗━┛), the others a light rounded one.
	tl, tr, bl, br, hz, vt := "╭", "╮", "╰", "╯", "─", "│"
	if focused {
		tl, tr, bl, br, hz, vt = "┏", "┓", "┗", "┛", "━", "┃"
	}
	width, height = max(4, width), max(3, height)
	label := ansi.Truncate(" "+title+" ", width-3, "…")
	out := []string{border.Render(tl+hz) + label + border.Render(strings.Repeat(hz, max(0, width-3-ansi.StringWidth(label)))+tr)}
	for i := 0; i < height-2; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, border.Render(vt)+" "+boardFit(line, width-4)+" "+border.Render(vt))
	}
	bottom := border.Render(bl + strings.Repeat(hz, width-2) + br)
	if p := " " + pos + " "; pos != "" && ansi.StringWidth(p) <= width-4 {
		bottom = border.Render(bl+strings.Repeat(hz, width-3-ansi.StringWidth(p))) + m.boardStyle(m.theme.TextMuted).Render(p) + border.Render(hz+br)
	}
	return strings.Join(append(out, bottom), "\n")
}

// Card heights: every card has a fixed height at rest and a larger one when
// focused. Only terminal size and focus decide them, never ticket counts.
const flexH = 1 << 20

// accordion splits total rows among cards: all at their rest height, the focused
// one grown to want, the leftover to the first flex card (else the focused one).
// Short terminals shrink the unfocused cards (last first) down to 3 rows.
func accordion(total int, rest, want []int, focus int) []int {
	n := len(rest)
	h := slices.Clone(rest)
	over := slices.Max([]int{0, sum(h) - total})
	for pass := 0; pass < 2 && over > 0; pass++ {
		for i := n - 1; i >= 0 && over > 0; i-- {
			if (i != focus) == (pass == 0) {
				cut := min(over, max(0, h[i]-3))
				h[i] -= cut
				over -= cut
			}
		}
	}
	rem := total - sum(h)
	short := rem <= 0
	if rem > 0 {
		grow := min(rem, want[focus]-h[focus])
		h[focus] += grow
		rem -= grow
	}
	// Portrait mode: a focused card still short of its height borrows from the
	// others, down to 3 rows each (not when a flex card already took the leftover).
	if need := want[focus] - h[focus]; need > 0 && (short || want[focus] < flexH) {
		for i := n - 1; i >= 0 && need > 0; i-- {
			if i != focus {
				cut := min(need, max(0, h[i]-3))
				h[i] -= cut
				h[focus] += cut
				need -= cut
			}
		}
	}
	if rem > 0 {
		to := focus
		for i := range want {
			if want[i] >= flexH {
				to = i
				break
			}
		}
		h[to] += rem
	}
	return h
}

// fitHeights fills exactly total rows with the cards of one column, so no gap
// is ever left above the footer. Every card starts at its min height, grows
// toward its need in column order (fixed content is not clipped just because a
// lower card wants the rows) and the spare rows left are split by weight, with
// Tasks and Workflows (weight > 0) taking them first. A column too short for
// the minimums shrinks the unfocused cards first, always the tallest one, down
// to a row.
func fitHeights(total int, low, need, weight []int, focus int) []int {
	h := make([]int, len(low))
	for i := range h {
		h[i] = max(1, low[i])
	}
	left := total - sum(h)
	if left < 0 {
		for pass := 0; pass < 2 && left < 0; pass++ { // unfocused cards, then the focused one
			for left < 0 {
				pick := -1
				for i := range h {
					if (i != focus) == (pass == 1) || h[i] <= 1 {
						continue
					}
					if pick < 0 || h[i] > h[pick] {
						pick = i
					}
				}
				if pick < 0 {
					break
				}
				h[pick]--
				left++
			}
		}
		return h
	}
	for i := range h {
		grow := min(left, max(0, need[i]-h[i]))
		h[i] += grow
		left -= grow
	}
	if left > 0 { // spare rows: Tasks and Workflows first, in proportion to weight
		var slots []int
		for i, w := range weight {
			for k := 0; k < w; k++ {
				slots = append(slots, i)
			}
		}
		if len(slots) == 0 { // a column of fixed cards only: they still fill it
			for i := range h {
				slots = append(slots, i)
			}
		}
		for k := 0; left > 0; k++ {
			h[slots[k%len(slots)]]++
			left--
		}
	}
	return h
}

func sum(v []int) (n int) {
	for _, x := range v {
		n += x
	}
	return
}

// boardSplit is the board screen's Tasks and Workflows card heights.
func (m Model) boardSplit(h int) (tasksH, wfH int) {
	focus := 0
	if m.workflowFocus {
		focus = 1
	}
	hs := accordion(h, []int{6, 3}, []int{flexH, 8}, focus)
	return hs[0], hs[1]
}

// workflowCard is the Workflows / Agents card (lazygit-style tabs, "t" swaps),
// windowed around its cursor. num is the Tab-order number (0: none).
func (m Model) workflowCard(width, height, num int) string {
	lines, cursor, n := m.stripPanel(width - 4)
	name, w := m.cardName(num, "")
	tabs, _ := m.tabStrip([]string{"Workflows", "Agents"}, b2i(m.agentsOpen), 3+w)
	start := 0
	if !(m.agentsOpen && m.agentDetail) {
		start = max(0, cursor-max(1, height-2)+1)
	}
	return m.boardPanel(name+tabs, position(cursor, n), lines[min(start, len(lines)):], width, height, m.workflowFocus)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (m Model) boardView() string {
	if m.height <= 0 {
		return ""
	}
	if m.width < 20 || m.height < 6 {
		lines := make([]string, m.height)
		lines[0] = "Board · " + m.notesScope
		if m.height > 3 {
			lines[1] = "too small — widen the pane"
		}
		if m.height >= 3 {
			lines[m.height-2] = m.keyHints("esc back")
			lines[m.height-1] = m.taskNotice
		}
		for i := range lines {
			lines[i] = boardFit(lines[i], m.width)
		}
		return strings.Join(lines, "\n")
	}
	h := m.height - 3
	var body string
	if m.boardFlow {
		label := ".tasks/board.yaml"
		if m.flowWorkflow != "" {
			label = m.flowWorkflow
		} else if len(m.notes) > 0 && m.notes[m.taskCursor].Workflow != "" {
			label = m.notes[m.taskCursor].Workflow
		}
		body = m.boardPanel("Workflow · "+label, "", m.flowLines(m.width-4), m.width, h, true)
	} else if m.boardDrawer && m.width >= 100 {
		// Wide panes: the ticket panel is a right pane beside the tickets.
		dw := m.width * 2 / 5
		left := m.tasksCard(m.width-dw, h, 0, false)
		right := m.boardPanel("Task · esc close", "", m.boardDetail(dw-4, h-2), dw, h, true)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	} else if m.boardDrawer {
		body = m.boardPanel("Task · esc close", "", m.boardDetail(m.width-4, h-2), m.width, h, true)
	} else {
		th, wh := m.boardSplit(h)
		body = m.tasksCard(m.width, th, 0, !m.workflowFocus) + "\n" + m.workflowCard(m.width, wh, 0)
	}
	hint := m.ticketHints()
	if m.workflowDelete != "" {
		hint = "Delete workflow " + m.workflowDelete + "? y / n"
	} else if m.agentDelete != "" {
		hint = "Delete agent " + m.agentDelete + "? y / n"
	} else if m.workflowFocus && m.agentsOpen && !m.boardFlow && !m.boardDrawer {
		hint = m.keyHints("enter detail", "m model", "e edit", "a add", "x delete", "t close", "Tab tickets", "? help")
	} else if m.workflowFocus && !m.boardFlow && !m.boardDrawer {
		hint = m.keyHints("enter view", "a add", "e edit", "t agents", "Tab tickets", "? help")
	}
	if m.boardFlow {
		hint = m.keyHints("esc close", "[/] browse", "e edit", "? help")
	} else if m.boardDrawer {
		hint = m.keyHints("esc close", "j/k scroll", "e edit", "C comment", "? help")
	}
	if m.taskDelete {
		hint = "Delete task? y confirm · n cancel"
	}
	triggers := m.taskNotice
	if triggers != "" {
		color := m.theme.Accent
		if strings.HasPrefix(triggers, "!") {
			color = "#ef4444"
		} else if strings.HasPrefix(triggers, "✓") {
			color = "#22c55e"
		}
		triggers = m.boardStyle(color).Render(triggers)
	}
	// Panel rendering must never push either footer row below the viewport.
	bodyLines := strings.Split(body, "\n")
	for len(bodyLines) < h {
		bodyLines = append(bodyLines, "")
	}
	out := []string{boardFit("Board · "+m.notesScope, m.width)}
	for _, line := range bodyLines[:h] {
		out = append(out, boardFit(line, m.width))
	}
	out = append(out, boardFit(hint, m.width), boardFit(triggers, m.width))
	return strings.Join(out, "\n")
}

func detailRelativeTime(at, now time.Time) string {
	if at.IsZero() {
		return "—"
	}
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	}
}

func (m Model) detailAgentColor(name string) lipgloss.Color {
	if agent, ok := tmux.ParseAgentType(name); ok {
		return m.theme.AgentColor(agent)
	}
	return m.theme.Accent
}

func (m Model) detailSection(label string, width int) []string {
	return strings.Split(ansi.Wrap(m.boardStyle(m.theme.TextPrimary).Bold(true).Render(label)+" "+m.boardStyle(m.theme.TextDim).Render(strings.Repeat("─", max(0, width-ansi.StringWidth(label)-1))), max(1, width), ""), "\n")
}

// boardWorker is the agent working the ticket and whether it is working now.
// Status-driven tickets have a pane per role (BoardTarget "<role>#<id>"); prefer
// the role of the current status, then any of the ticket's panes, then an
// assignee's pane for manually assigned tasks.
func (m Model) boardWorker(t tasks.Task) string {
	role := ""
	cfg, err := tasks.ConfigForTask(m.boardDir, m.boardConfig, t)
	if err != nil {
		return "! " + err.Error()
	}
	for _, s := range cfg.Statuses {
		if s.ID == t.Status {
			role = s.Role
		}
	}
	var pick *Row
	for i, r := range m.rows {
		if r.IsHeader || r.IsRepoAgent || r.PaneID == "" {
			continue
		}
		agent, task, _ := strings.Cut(r.BoardTarget, "#")
		switch {
		case task == t.ID && agent == role:
			pick = &m.rows[i]
		case pick == nil && task == t.ID:
			pick = &m.rows[i]
		case pick == nil && task == "" && (slices.Contains(t.Assignees, r.Base) || slices.Contains(t.Assignees, agent)):
			pick = &m.rows[i]
		}
	}
	if pick == nil {
		return ""
	}
	name, _, _ := strings.Cut(pick.BoardTarget, "#")
	if name == "" {
		name = pick.Base
	}
	if pick.Status.IsActive() {
		return m.boardStyle(m.theme.StatusRunning).Render("● " + name + " " + string(pick.Status))
	}
	return "○ " + name + " " + string(pick.Status)
}

// boardDetail is the drawer: a summary that never scrolls, the scrolling body
// and a sticky footer, exactly rows lines wide-as-w.
func (m Model) boardDetail(w, rows int) []string {
	if w <= 0 || rows <= 0 {
		return nil
	}
	if len(m.notes) == 0 {
		return []string{boardFit("No tasks yet", w)}
	}
	summary, body, bodyRows := m.detailLayout(m.notes[m.taskCursor], w, rows)
	out := append([]string{}, summary...)
	out = append(out, m.detailViewport(body, w, bodyRows, m.detailScroll)...)
	if rows >= 6 {
		out = append(out, m.detailFooter(w))
	}
	if len(out) > rows {
		out = out[:rows]
	}
	for len(out) < rows {
		out = append(out, "")
	}
	for i := range out {
		out[i] = boardFit(out[i], w)
	}
	return out
}

func (m Model) triggerSummary() string {
	var parts []string
	for _, t := range m.boardConfig.Triggers {
		event := t.On
		if t.When.To != "" {
			event += "→" + t.When.To
		}
		target := "agent " + t.Agent
		parts = append(parts, event+" ▸ "+target)
	}
	if len(parts) == 0 {
		return "Triggers: none · c edit board config"
	}
	return "Triggers: " + strings.Join(parts, " · ")
}

// Returns handled=false only for the existing Tasks actions, routed to their forms.
func (m Model) boardKey(key string) (Model, tea.Cmd, bool) {
	if key == "o" {
		return m, nil, false // the global expand key and button still work from the board
	}
	if m.boardMode && len(m.notes) > 0 && (key == "a" || key == "A") && !m.workflowFocus && m.workflowDelete == "" {
		m.taskForm = "attachments"
		if key == "A" {
			m.taskForm = "attach"
		}
		m.taskStep = 0
		m.taskInput = ""
		m.attachmentsKey = ""
		return m.syncAttachments(), nil, true
	}
	if m.boardMode && !m.boardFlow && !m.boardDrawer {
		if m.workflowDelete != "" || m.workflowFocus {
			return m.workflowPanelKey(key)
		}
		if key == "tab" || key == "shift+tab" {
			m.workflowFocus = true
			return m, nil, true
		}
	}
	if m.boardMode && m.boardDrawer {
		if m.detailTabKey(key) {
			return m, nil, true
		}
		switch key {
		case "s": // the dispatch noise stays folded until asked for
			if m.detailTab == detailTabActivity {
				m.detailSysOpen = !m.detailSysOpen
			}
			return m, nil, true
		case "enter", " ": // expand a long description in place, or start the ticket's chat
			switch m.detailTab {
			case detailTabDetails:
				m.detailDescOpen = !m.detailDescOpen
			case detailTabChat:
				if m.chat.Pane == "" && m.chat.available() && len(m.notes) > 0 {
					dir, id := m.boardDir, m.notes[m.taskCursor].ID
					m.taskNotice = "Starting the chat…"
					return m, startTicketChat(dir, id), true
				}
			}
			return m, nil, true
		case "i": // message the chat's agent; the form's footer says how to save it
			if m.detailTab == detailTabChat {
				if m.chat.started() {
					m.taskForm, m.taskInput = "chat", ""
				}
				return m, nil, true
			}
		case "S": // save the newest reply to the ticket as a comment
			if m.detailTab == detailTabChat {
				return m.saveChatReply(), nil, true
			}
		}
		if m.scrollDetail(key) {
			return m, nil, true
		}
	}
	if m.boardFlow {
		switch key {
		case "[", "]":
			ws := workflow.List(tasks.MainRoot(m.boardDir))
			if len(ws) > 0 {
				i := -1
				for n, w := range ws {
					if w.File == m.flowWorkflow || (m.flowWorkflow == "" && len(m.notes) > 0 && w.Name == m.notes[m.taskCursor].Workflow) {
						i = n
						break
					}
				}
				if i < 0 {
					if key == "]" {
						i = len(ws) - 1
					} else {
						i = 0
					}
				}
				if key == "]" {
					i = (i + 1) % len(ws)
				} else {
					i = (i + len(ws) - 1) % len(ws)
				}
				m.flowWorkflow = ws[i].File
			}
			return m, nil, true
		case "e", "c":
			path := m.flowWorkflow
			if path == "" && len(m.notes) > 0 && m.notes[m.taskCursor].Workflow != "" {
				w, e := workflow.Find(tasks.MainRoot(m.boardDir), m.notes[m.taskCursor].Workflow)
				if e != nil {
					m.taskNotice = e.Error()
					return m, nil, true
				}
				path = w.File
			}
			if path != "" {
				dir := m.boardDir
				return m, editFile(m.tmuxPane, path, func() tea.Msg { return reloadBoardConfig(dir, nil) }), true
			}
			return m, editBoardConfig(m.boardDir, m.tmuxPane), true
		case "ctrl+c":
			return m, tea.Quit, true
		case "esc", "w", "q":
			m.boardFlow = false
			if m.flowFromDash { // back to the dashboard's Workflows card
				m.flowFromDash, m.boardMode = false, false
				m.agentFocus, m.taskFocus, m.workflowFocus = 4, false, true
			}
		}
		return m, nil, true
	}
	switch key {
	case "t":
		m.boardMode = true
		m.agentsOpen = !m.agentsOpen
		m.workflowFocus, m.agentDetail, m.agentCursor = m.agentsOpen, false, 0
		if m.agentsOpen {
			m = m.loadAgents()
		}
		return m, nil, true
	case "w":
		m.boardMode = true
		m.boardFlow, m.boardDrawer = true, false
		m.flowWorkflow = ""
		return m, nil, true
	case "enter", "i":
		m.boardMode = true
		m.boardDrawer = !m.boardDrawer
		m.detailScroll = 0
		m.detailTab = detailTabDetails
		m.detailDescOpen, m.detailSysOpen = false, false
		return m, nil, true
	case "esc":
		if m.boardDrawer {
			m.boardDrawer = false
			return m, nil, true
		}
		m.boardMode = false
		return m, nil, true
	case "q", "b":
		m.boardMode, m.boardDrawer = false, false
		return m, nil, true
	case "C":
		if len(m.notes) > 0 {
			return m, editTaskText(m.notesScope, m.notes[m.taskCursor], m.tmuxPane, true), true
		}
		return m, nil, true
	case "c":
		if len(m.notes) > 0 && m.notes[m.taskCursor].Status == "blocked" {
			return m, editTaskText(m.notesScope, m.notes[m.taskCursor], m.tmuxPane, true), true
		}
		return m, editBoardConfig(m.boardDir, m.tmuxPane), true
	case "n":
		return m, editTaskText(m.notesScope, tasks.Task{}, m.tmuxPane, false), true
	case "s":
		if len(m.notes) > 0 {
			m.taskForm, m.taskStep = "start", 0
		}
		return m, nil, true
	case "e":
		if len(m.notes) > 0 {
			return m, editTaskText(m.notesScope, m.notes[m.taskCursor], m.tmuxPane, false), true
		}
		return m, nil, true
	case "x":
		m.taskDelete = len(m.notes) > 0
		return m, nil, true
	case "ctrl+c":
		return m, tea.Quit, true
	}
	step, move := 0, false
	switch key {
	case "j", "down":
		step = 1
	case "k", "up":
		step = -1
	case "h", "l":
		if !m.boardDrawer {
			m = m.boardNavigateColumn(key)
		}
		return m, nil, true
	case "H":
		step, move = -1, true
	case "L":
		step, move = 1, true
	default:
		return m, nil, true // Never let Board keys act on agents or Git.
	}
	if len(m.notes) == 0 {
		return m, nil, true
	}
	if move {
		cfg, e := tasks.ConfigForTask(m.boardDir, m.boardConfig, m.notes[m.taskCursor])
		if e != nil {
			m.taskNotice = e.Error()
			return m, nil, true
		}
		columns := cfg.Columns
		target := slices.Index(columns, m.notes[m.taskCursor].Status) + step
		if target >= 0 && target < len(columns) {
			scope, id, status := m.notesScope, m.notes[m.taskCursor].ID, columns[target]
			return m, func() tea.Msg {
				_, err := tasks.Update(scope, id, status, nil, tasks.MutationOptions{Actor: "user"})
				if err == nil && status != "doing" {
					err = metrics.End(scope, id)
				}
				return actionResult("Task moved to "+status, err)
			}, true
		}
		return m, nil, true
	}
	return m.laneStep(step), nil, true
}

// detailMaxScroll is how far the drawer body can scroll: the summary and the
// footer take their rows off the top of the window.
func (m Model) detailMaxScroll() int {
	if len(m.notes) == 0 {
		return 0
	}
	w, rows := m.width-4, max(0, m.height-5)
	_, body, bodyRows := m.detailLayout(m.notes[m.taskCursor], w, rows)
	return max(0, len(body)-max(0, bodyRows-2))
}

func (m Model) ticketHints() string {
	return m.keyHints("h/l lane", "j/k card", "n new", "s start", "enter detail", "e edit", "C comment", "H/L status", "a files", "x delete", "w workflow", "? help")
}

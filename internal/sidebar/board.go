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

	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
)

type boardConfigMsg struct {
	config tasks.BoardConfig
	err    error
}

func (m Model) boardContext() Model {
	oldDir := m.boardDir
	m.boardDir = m.repoDir
	if m.standalone && m.launchDir != "" {
		m.boardDir = m.launchDir
	}
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
	if m.notesScope == "" || oldDir != "" && oldDir != m.boardDir || m.standalone && m.notesScope != tasks.Scope(m.boardDir) {
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
		m.taskNotice = boardProblemNotice(m.boardDir, cfg, m.taskNotice)
	}
	m.setWorkflows(workflow.List(tasks.MainRoot(m.boardDir)))
	m.taskCursor = max(0, min(m.taskCursor, len(m.notes)-1))
	return m
}

// boardProblemNotice is the first thing wrong with the board or its workflows
// as a "!" notice (plus how many more), or current when all is well.
func boardProblemNotice(dir string, cfg tasks.BoardConfig, current string) string {
	problems := tasks.CheckBoard(dir, cfg)
	if len(problems) == 0 {
		return current
	}
	if len(problems) > 1 {
		return fmt.Sprintf("! %s (+%d more)", problems[0], len(problems)-1)
	}
	return "! " + problems[0]
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

// boardColumns are the fixed lanes: todo, in progress, blocked, done, closed.
func (m Model) boardColumns() []string { return m.boardConfig.Lanes() }

// boardGlyph: many statuses, five icons (todo, in progress, blocked, done, closed).
func (m Model) boardGlyph(status string) (string, lipgloss.Color) {
	switch {
	case status == "blocked":
		return "✕", m.theme.StatusError
	case m.boardConfig.Lane(status) == "done":
		return "✓", m.theme.StatusDone
	case m.boardConfig.Lane(status) == "todo":
		return "○", m.theme.TextMuted
	case m.boardConfig.Lane(status) == "closed":
		return "⊘", m.theme.TextMuted
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

// The board has five fixed lanes however many statuses a workflow defines; a
// card shows its real status.
var boardLanes = []string{"todo", "in progress", "blocked", "done", "closed"}

func (m Model) laneOf(status string) string { return m.boardConfig.Lane(status) }

// needState is a todo card's dependency badge: "waits #12, #15", "ready"
// once every dependency is done, or "" without dependencies. A dependency
// that no longer exists does not hold the card.
func (m Model) needState(t tasks.Task) string {
	if len(t.Needs) == 0 || t.Status != "todo" {
		return ""
	}
	var open []string
	for _, id := range t.Needs {
		done, found, closed := false, false, false
		for _, n := range m.notes {
			if n.ID == id {
				done, found, closed = n.Status == "done", true, n.Status == "closed"
				break
			}
		}
		if closed {
			open = append(open, "closed #"+id)
		} else if found && !done {
			open = append(open, "#"+id)
		}
	}
	if len(open) == 0 {
		return "ready"
	}
	return "waits " + strings.Join(open, ", ")
}

// laneCards lists the ticket indexes in a lane: priority first, then most
// recently updated (same order as the web board).
func (m Model) laneCards(lane string) []int {
	var cards []int
	for i, t := range m.notes {
		if m.laneOf(t.Status) == lane {
			cards = append(cards, i)
		}
	}
	slices.SortStableFunc(cards, func(a, b int) int {
		if r := tasks.PriorityRank(m.notes[a].Priority) - tasks.PriorityRank(m.notes[b].Priority); r != 0 {
			return r
		}
		return m.notes[b].Updated.Compare(m.notes[a].Updated)
	})
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
	if id == "" { // first load: open on work in progress, else todo, never on done history
		for _, lane := range []string{"in progress", "todo"} {
			if cards := m.laneCards(lane); len(cards) > 0 {
				m.taskCursor = cards[0]
				break
			}
		}
	}
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

// tasksTitle is "☷ Tasks  [todo 1]  doing 2  done 32". When that does not fit
// width (a narrow sidebar), the inactive tabs shrink to a glyph and a count.
func (m Model) tasksTitle(num, width int) (string, []tabSpan) {
	name, w := m.cardName(num, "☷ Tasks")
	active := slices.Index(boardLanes, m.activeLane())
	labels := make([]string, len(boardLanes))
	total := w + 2 - 2 // name, gap, then labels with 2-cell separators
	for i, l := range boardLanes {
		labels[i] = fmt.Sprintf("%s %d", laneShort(l), len(m.laneCards(l)))
		total += ansi.StringWidth(labels[i]) + 2
	}
	if total > width-4-8 { // border plus the "3 of 12" counter
		for i, l := range boardLanes {
			if i != active {
				labels[i] = fmt.Sprintf("%s%d", laneGlyph(l), len(m.laneCards(l)))
			}
		}
	}
	tabs, spans := m.tabStrip(labels, active, 3+w+2)
	return name + "  " + tabs, spans
}

func laneGlyph(lane string) string {
	switch lane {
	case "todo":
		return "○"
	case "in progress":
		return "▶"
	case "blocked":
		return "!"
	case "closed":
		return "⊘"
	}
	return "✓"
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
	badge := ""
	if mark := tasks.PriorityMark(t.Priority); mark != "" {
		badge += m.boardStyle(m.boardColor(t.Status)).Render(" " + mark)
	}
	if state := m.needState(t); state != "" {
		color := m.theme.TextMuted
		if state == "ready" {
			color = m.theme.StatusDone
		}
		badge += m.boardStyle(color).Render(" " + state)
	}
	if t.Workflow != "" {
		badge += m.boardStyle(m.theme.TextMuted).Render(" " + t.Workflow)
	}
	if t.AutoStart {
		badge += m.boardStyle(m.theme.TextMuted).Render(" auto")
	}
	return boardFit(style.Render(mark)+m.boardStyle(m.boardColor(t.Status)).Render(status)+m.boardStyle(m.theme.TextMuted).Render(" #"+t.ID)+badge+" "+style.Render(boardTitle(t)), width)
}

// boardCard: title, then "● status #id role worker"; a blocked card adds its question.
func (m Model) boardCard(idx, width int) []string {
	t := m.notes[idx]
	if lane := m.laneOf(t.Status); lane == "done" || lane == "todo" || lane == "closed" { // todo, done and closed cards: one line
		return []string{m.boardCardLine(idx, width)}
	}
	mark := "  "
	style := m.boardStyle(m.theme.TextPrimary)
	if idx == m.taskCursor {
		mark = "▌ "
		style = style.Bold(true).Foreground(m.theme.Accent)
	}
	role := ""
	if cfg, err := tasks.ConfigForTask(m.boardDir, m.boardConfig, t); err == nil {
		for _, s := range cfg.Statuses {
			if s.ID == t.Status {
				role = s.Role
			}
		}
	}
	if role == "" && len(t.Assignees) > 0 {
		role = t.Assignees[0]
	}
	muted := m.boardStyle(m.theme.TextMuted)
	meta := "  " + m.boardStyle(m.boardColor(t.Status)).Render("● "+t.Status) + muted.Render(" #"+t.ID+" "+role)
	if mark := tasks.PriorityMark(t.Priority); mark != "" {
		meta += m.boardStyle(m.boardColor(t.Status)).Render(" " + mark)
	}
	if state := m.needState(t); state != "" {
		meta += muted.Render(" " + state)
	}
	if t.Workflow != "" {
		meta += muted.Render(" " + t.Workflow)
	}
	if t.AutoStart {
		meta += muted.Render(" auto")
	}
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

// tasksSize is a Tasks card's tier, picked from its own rect (tasksSizeFor).
type tasksSize int

const (
	sizeCounts tasksSize = iota // the five lane counts
	sizeList                    // the active lane's cards
	sizeDetail                  // the lane list beside the selected ticket
	sizeBoard                   // four lanes side by side
)

// tasksSizeFor picks the Tasks tier from the card's width and height, borders included.
func tasksSizeFor(w, h int) tasksSize {
	switch {
	case h < 6:
		return sizeCounts
	case w < 64:
		return sizeList
	case w >= 100 && h >= 12:
		return sizeBoard
	}
	return sizeDetail
}

// tasksCols are a Tasks card body's column widths and the separator between them.
func tasksCols(size tasksSize, width int) ([]int, string) {
	inner := width - 4
	switch size {
	case sizeDetail:
		lw := (inner - 3) / 2
		return []int{lw, inner - lw - 3}, paneColSep
	case sizeBoard:
		cw := (inner - 3) / 4
		return []int{cw, cw, cw, inner - 3*cw - 3}, " "
	}
	return []int{inner}, ""
}

// tasksColAt is the column under card x (the border and padding come first), -1 in a gap.
func tasksColAt(size tasksSize, width, x int) int {
	widths, sep := tasksCols(size, width)
	start := 2
	for i, w := range widths {
		if x >= start && x < start+w {
			return i
		}
		start += w + ansi.StringWidth(sep)
	}
	return -1
}

// joinCols puts columns side by side, each cut or padded to its width, for rows lines.
func joinCols(cols [][]string, widths []int, sep string, rows int) []string {
	out := make([]string, rows)
	for r := range out {
		parts := make([]string, len(cols))
		for c, col := range cols {
			line := ""
			if r < len(col) {
				line = col[r]
			}
			parts[c] = boardFit(line, widths[c])
		}
		out[r] = strings.Join(parts, sep)
	}
	return out
}

// tasksBoardLanes are the four lanes of the board tier: closed takes done's column while a closed ticket is selected.
func (m Model) tasksBoardLanes() []string {
	lanes := slices.Clone(boardLanes[:4])
	if m.activeLane() == "closed" {
		lanes[3] = "closed"
	}
	return lanes
}

// laneCountLines is the counts tier: every lane with its count, the active lane bold, wrapped to width.
func (m Model) laneCountLines(width int) []string {
	active := m.activeLane()
	var lines []string
	cur := ""
	for _, l := range boardLanes {
		item := fmt.Sprintf("%s %s %d", laneGlyph(l), laneShort(l), len(m.laneCards(l)))
		style := m.boardStyle(m.theme.TextMuted)
		if l == active {
			style = m.boardStyle(m.theme.TextPrimary).Bold(true)
		}
		if cur != "" && ansi.StringWidth(cur)+2+ansi.StringWidth(item) > width {
			lines, cur = append(lines, cur), ""
		}
		if cur != "" {
			cur += "  "
		}
		cur += style.Render(item)
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// laneLayout is a lane's cards as lines, with the card index owning each line.
func (m Model) laneLayout(lane string, width int) (lines []string, owner []int) {
	for _, idx := range m.laneCards(lane) {
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

// laneLines is a lane's card lines from the window that keeps the selected card in view.
func (m Model) laneLines(lane string, width, rows int) []string {
	lines, owner := m.laneLayout(lane, width)
	return lines[min(m.laneWindow(owner, rows), len(lines)):]
}

// tasksCard is the Tasks card at the size its rect allows (tasksSizeFor): tabs in the title.
func (m Model) tasksCard(width, height, num int, focused bool) string {
	return m.tasksCardAt(tasksSizeFor(width, height), width, height, num, focused)
}

// tasksCardAt draws the Tasks card at a size; the title (tabs) is the same at every size.
// "3 of 32" sits on the bottom border of the list tiers only.
func (m Model) tasksCardAt(size tasksSize, width, height, num int, focused bool) string {
	title, _ := m.tasksTitle(num, width)
	active := m.activeLane()
	cards := m.laneCards(active)
	pos := position(slices.Index(cards, m.taskCursor), len(cards))
	widths, sep := tasksCols(size, width)
	var body []string
	switch size {
	case sizeCounts:
		body, pos = m.laneCountLines(width-4), ""
	case sizeList:
		body = m.laneLines(active, widths[0], height-2)
	case sizeDetail: // the preview is read-only: Details tab, top of the body, no composer
		d := m
		d.detailTab, d.detailScroll, d.commentBox = detailTabDetails, 0, false
		body = joinCols([][]string{m.laneLines(active, widths[0], height-2), d.boardDetail(widths[1], height-2)}, widths, sep, height-2)
	case sizeBoard:
		cols := make([][]string, len(widths))
		for c, lane := range m.tasksBoardLanes() {
			head := fmt.Sprintf("%s %d", laneShort(lane), len(m.laneCards(lane)))
			style := m.boardStyle(m.theme.TextMuted)
			if lane == active {
				head, style = "["+head+"]", m.boardStyle(m.theme.TextPrimary).Bold(true)
			}
			cols[c] = append([]string{style.Render(head)}, m.laneLines(lane, widths[c], height-3)...)
		}
		body = joinCols(cols, widths, sep, height-2)
	}
	return m.boardPanel(title, pos, body, width, height, focused)
}

// tasksCardClick handles a click at (x, y) inside a Tasks card of this size
// (y 0: the title row, whose tabs pick a lane). card reports a click on a ticket.
func (m Model) tasksCardClick(x, y, width, height, num int) (next Model, card bool) {
	if y == 0 {
		_, spans := m.tasksTitle(num, width)
		for i, s := range spans {
			if x >= s.x0 && x < s.x1 {
				return m.selectLane(boardLanes[i]), false
			}
		}
		return m, false
	}
	size := tasksSizeFor(width, height)
	if y >= height-1 || size == sizeCounts {
		return m, false
	}
	widths, _ := tasksCols(size, width)
	col := 0
	if size != sizeList { // the list ignores x, as it always has
		col = tasksColAt(size, width, x)
	}
	lane, rows, row := m.activeLane(), height-2, y-1 // row: the body row under the pointer
	switch {
	case col < 0, size == sizeDetail && col > 0:
		return m, false
	case size == sizeBoard:
		lane, rows, row = m.tasksBoardLanes()[col], height-3, y-2
		if y == 1 { // the lane header
			return m.selectLane(lane), false
		}
	}
	lines, owner := m.laneLayout(lane, widths[col])
	if i := m.laneWindow(owner, rows) + row; i < len(lines) {
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
			if t.Workflow == w.Name && !slices.Contains([]string{"done", "closed"}, m.boardConfig.Lane(t.Status)) && t.Status != "todo" {
				active++
			}
		}
		run := ""
		if active > 0 {
			run = "  " + m.boardStyle(m.theme.StatusRunning).Render(fmt.Sprintf("● %d running", active))
		}
		mark := "▸ "
		if m.focus == cardWorkflows && i == m.workflowCursor {
			mark = "❯ "
		}
		lines = append(lines, boardFit(mark+m.boardStyle(m.theme.TextPrimary).Bold(true).Render(w.Name)+"  "+dim.Render(info)+run, width))
	}
	return lines
}

// paneColSep separates the columns of a split card (Tasks detail, Workflows flow).
const paneColSep = " │ "

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
		for _, id := range m.boardConfig.Columns {
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
				if r.Tier != "" {
					who += " · tier " + r.Tier
				} else {
					who += " · " + strings.TrimSpace(r.Agent+" "+r.Model)
				}
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
			next += fmt.Sprintf(" · max %d rejections, then blocked", s.MaxLoops)
		}
		lines = append(lines, boardFit("  "+strings.Repeat(" ", idw)+dim.Render(next), width))
	}
	return lines
}

// boardConfigMaxScroll is the last scroll position of the config view.
func (m Model) boardConfigMaxScroll() int {
	return max(0, len(m.boardConfigLines(m.width-4))-max(0, m.height-4))
}

// boardConfigLines is the read-only config summary (B): the same
// tasks.BoardSummary the web config page renders.
func (m Model) boardConfigLines(width int) []string {
	sum := m.boardConfig.Summary()
	dim := m.boardStyle(m.theme.TextMuted)
	title := m.boardStyle(m.theme.TextPrimary).Bold(true)
	flow := sum.DefaultWorkflow
	if flow == "" {
		flow = "none (pick one when starting a ticket)"
	}
	lines := []string{
		boardFit(title.Render("Default workflow")+"  "+dim.Render(flow), width),
		boardFit(dim.Render(fmt.Sprintf("%d roles · %d triggers · lanes: todo, in progress, blocked, done", len(sum.Roles), len(sum.Triggers))), width),
		"", title.Render("Roles"),
	}
	if len(sum.Roles) == 0 {
		lines = append(lines, dim.Render("  none"))
	}
	for _, r := range sum.Roles {
		info := strings.TrimSpace(r.Agent + " " + r.Model)
		if r.Tier != "" {
			info = "tier " + r.Tier
		}
		if r.Effort != "" {
			info += " · effort " + r.Effort
		}
		if r.FreshAfter != nil {
			info += fmt.Sprintf(" · fresh after %d", *r.FreshAfter)
		}
		lines = append(lines, boardFit("  "+title.Render(r.Name)+"  "+dim.Render(info), width))
	}
	lines = append(lines, "", title.Render("Triggers"))
	if len(sum.Triggers) == 0 {
		lines = append(lines, dim.Render("  none"))
	}
	for _, t := range sum.Triggers {
		line := "  on " + t.On
		if t.When != "" {
			line += " " + t.When
		}
		line += " ▸ agent " + t.Agent
		lines = append(lines, boardFit(dim.Render(line), width))
	}
	lines = append(lines, "", dim.Render(fmt.Sprintf("max_fires_per_hour: %d · run: commands execute locally · E edit", sum.MaxFiresPerHour)))
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
// ("3 of 32") on the bottom border.
func (m Model) boardPanel(title, pos string, lines []string, width, height int, focused bool) string {
	focus, idle := m.theme.BorderFocus, m.theme.Border
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

// workflowCols are the Workflows card's list and job-flow widths. The flow shows
// from width 80 with a workflow to draw; flowW 0 means the list alone.
func (m Model) workflowCols(width int) (listW, flowW int) {
	inner := width - 4
	if width < 80 || len(m.boardWorkflows) == 0 {
		return inner, 0
	}
	sep := ansi.StringWidth(paneColSep)
	listW = (inner - sep) / 2
	return listW, inner - listW - sep
}

// jobFlowLines is the selected workflow's jobs, one line each: id, status, who
// runs it, what it needs. Rows past rows end in "↓ N more". It reads only what
// the workflow already holds.
func (m Model) jobFlowLines(width, rows int) []string {
	if len(m.boardWorkflows) == 0 {
		return nil
	}
	w := m.boardWorkflows[min(m.workflowCursor, len(m.boardWorkflows)-1)]
	dim := m.boardStyle(m.theme.TextMuted)
	var lines []string
	if w.Err != nil {
		lines = append(lines, boardFit(m.boardStyle(m.theme.StatusError).Render("error: "+w.Err.Error()), width))
	}
	for _, j := range w.Jobs {
		who := j.Agent
		switch {
		case j.Run != "":
			who = "run: " + strings.SplitN(j.Run, "\n", 2)[0]
		case j.Uses != "":
			who = "uses " + j.Uses
		}
		line := m.boardStyle(m.theme.TextPrimary).Bold(true).Render(j.ID) + dim.Render("  "+j.Status+"  "+who)
		if len(j.Needs) > 0 {
			line += dim.Render(" · needs " + strings.Join(j.Needs, ", "))
		}
		lines = append(lines, boardFit(line, width))
	}
	if len(lines) > rows {
		hidden := len(lines) - max(0, rows-1)
		lines = append(lines[:max(0, rows-1)], boardFit(dim.Render(fmt.Sprintf("↓ %d more", hidden)), width))
	}
	return lines
}

// workflowCard is the Workflows card, windowed around its cursor, with the
// selected workflow's job flow beside the list when there is room (workflowCols).
// num is the Tab-order number (0: none).
func (m Model) workflowCard(width, height, num int) string {
	listW, flowW := m.workflowCols(width)
	lines := m.workflowLines(listW)
	name, _ := m.cardName(num, "Workflows")
	start := max(0, m.workflowCursor-max(1, height-2)+1)
	body := lines[min(start, len(lines)):]
	if flowW > 0 {
		body = joinCols([][]string{body, m.jobFlowLines(flowW, max(0, height-2))}, []int{listW, flowW}, paneColSep, max(0, height-2))
	}
	return m.boardPanel(name, position(m.workflowCursor, len(m.boardWorkflows)), body, width, height, m.focus == cardWorkflows)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
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

// boardDetail is the drawer: a summary that never scrolls and the scrolling
// body, exactly rows lines wide-as-w.
func (m Model) boardDetail(w, rows int) []string {
	if w <= 0 || rows <= 0 {
		return nil
	}
	if len(m.notes) == 0 {
		return []string{boardFit("No tasks yet", w)}
	}
	// The comment composer is pinned above the footer so scrolling never hides it.
	var composer []string
	if m.commentBox && rows >= 8 {
		composer = m.commentBoxLines(w)
	}
	summary, body, bodyRows := m.detailLayout(m.notes[m.taskCursor], w, rows-len(composer))
	out := append([]string{}, summary...)
	out = append(out, m.detailViewport(body, w, bodyRows, m.detailScroll)...)
	out = append(out, composer...)
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

// Returns handled=false only for the existing Tasks actions, routed to their forms.
// boardKey routes a key to the focused pane (the same call handleKey makes).
func (m Model) boardKey(key string) (Model, tea.Cmd, bool) {
	return panes[m.focused()].Key(m, key)
}

// detailMaxScroll is how far the drawer body can scroll: the summary and the
// footer take their rows off the top of the window.
func (m Model) detailMaxScroll() int {
	if len(m.notes) == 0 {
		return 0
	}
	w, rows := m.width-4, max(0, m.height-4)
	if m.commentBox && rows >= 8 {
		rows -= len(m.commentBoxLines(w))
	}
	_, body, bodyRows := m.detailLayout(m.notes[m.taskCursor], w, rows)
	return max(0, len(body)-max(0, bodyRows-2))
}

// needCandidates lists the tickets the selected one may wait for (every ticket
// but itself), in ticket order.
func (m Model) needCandidates() []tasks.Task {
	if m.taskCursor < 0 || m.taskCursor >= len(m.notes) {
		return nil
	}
	id := m.notes[m.taskCursor].ID
	out := make([]tasks.Task, 0, len(m.notes))
	for _, t := range m.notes {
		if t.ID != id {
			out = append(out, t)
		}
	}
	return out
}

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
	m.boardMode, m.taskFocus, m.agentFocus, m.taskTitle = true, true, 2, false
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
	m.boardWorkflows = workflow.List(tasks.MainRoot(m.boardDir))
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

func (m Model) boardColumns() []string {
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

// boardOrder holds indices into notes (forms act on m.notes[taskCursor]):
// open tickets first, done last, each in board order.
func (m Model) boardOrder() []int {
	var open, done []int
	seen := map[int]bool{}
	for _, status := range m.boardColumns() {
		for _, i := range m.boardStatusCards(status) {
			seen[i] = true
			if m.boardConfig.Lane(status) == "done" {
				done = append(done, i)
			} else {
				open = append(open, i)
			}
		}
	}
	for i := range m.notes {
		if !seen[i] {
			open = append(open, i)
		}
	}
	return append(open, done...)
}

func (m Model) boardWidth() int { return m.width - max(44, m.width*35/100) }
func (m Model) boardWide() bool { return m.width >= 100 }

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

// boardRow is always two lines: icon + title, then status · owner · worker · comments · due.
func (m Model) boardRow(idx, width int) []string {
	t := m.notes[idx]
	glyph, color := m.boardGlyph(t.Status)
	mark, title := "  ", m.boardStyle(m.theme.TextPrimary)
	if idx == m.taskCursor {
		mark, title = m.boardStyle(m.theme.Accent).Render("▌ "), title.Bold(true)
	}
	meta := []string{t.Status}
	if t.Workflow != "" {
		meta = append(meta, t.Workflow)
	}
	if len(t.Assignees) > 0 {
		meta = append(meta, t.Assignees[0])
	}
	if w := m.boardWorker(t); w != "" {
		meta = append(meta, w)
	}
	if n := len(t.Comments); n > 0 {
		meta = append(meta, fmt.Sprintf("%s %d", m.icons.Message, n))
	}
	if due := t.Due; len(due) >= 10 {
		meta = append(meta, m.icons.Clock+" "+due[5:10])
	}
	if t.Status == "blocked" {
		cfg, _ := tasks.ConfigForTask(m.boardDir, m.boardConfig, t)
		_, role, reason := tasks.BlockContext(t, cfg, m.boardHistory)
		if reason == "" {
			reason = "Blocked: reply with @role"
			if role != "" {
				reason = "Blocked: reply with @" + role
			}
		}
		meta = []string{strings.Join(strings.Fields(reason), " ")}
	}
	return []string{
		boardFit(mark+m.boardStyle(color).Render(glyph)+" "+title.Render(boardTitle(t)), width),
		boardFit("    "+m.boardStyle(m.theme.TextMuted).Render(strings.Join(meta, " · ")), width),
	}
}

// boardStatusCards preserves ticket order within a configured status.
func (m Model) boardStatusCards(status string) []int {
	var cards []int
	for i, t := range m.notes {
		if t.Status == status {
			cards = append(cards, i)
		}
	}
	return cards
}

func (m Model) boardNavigateColumn(key string) Model {
	if len(m.notes) == 0 {
		return m
	}
	columns := m.boardColumns()
	col := slices.Index(columns, m.notes[m.taskCursor].Status)
	row := slices.Index(m.boardStatusCards(m.notes[m.taskCursor].Status), m.taskCursor)
	step := 1
	if key == "h" {
		step = -1
	}
	for col += step; col >= 0 && col < len(columns); col += step {
		cards := m.boardStatusCards(columns[col])
		if len(cards) > 0 {
			m.taskCursor = cards[min(max(0, row), len(cards)-1)]
			break
		}
	}
	m.detailScroll = 0
	return m
}

func (m Model) boardCard(idx, width int) []string {
	t := m.notes[idx]
	mark := "  "
	style := m.boardStyle(m.theme.TextPrimary)
	if idx == m.taskCursor {
		mark = "▌ "
		style = style.Bold(true).Foreground(m.theme.Accent)
	}
	if m.boardConfig.Lane(t.Status) == "done" {
		return []string{boardFit(style.Render(mark+"#"+t.ID+" "+boardTitle(t)), width)}
	}
	title := strings.Split(ansi.Wrap(boardTitle(t), max(1, width-2), ""), "\n")
	for len(title) < 2 {
		title = append(title, "")
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
	meta := "#" + t.ID + " " + role
	if worker := m.boardWorker(t); worker != "" {
		meta += " " + worker
	}
	if len(t.Comments) > 0 {
		meta += fmt.Sprintf(" %s%d", m.icons.Message, len(t.Comments))
	}
	if t.Status == "blocked" {
		cfg, _ := tasks.ConfigForTask(m.boardDir, m.boardConfig, t)
		_, _, question := tasks.BlockContext(t, cfg, m.boardHistory)
		if question == "" {
			question = "Blocked: reply with @role"
		}
		title[1] = strings.Join(strings.Fields(question), " ")
	}
	return []string{boardFit(style.Render(mark+title[0]), width), boardFit(style.Render("  "+title[1]), width), boardFit(m.boardStyle(m.theme.TextMuted).Render("  "+meta), width)}
}

func (m Model) boardColumnView(height int) string {
	columns := m.boardColumns()
	if len(columns) == 0 {
		return ""
	}
	// Window wide workflows around the selected status, retaining readable cards.
	visible := min(len(columns), max(1, m.width/24))
	selected := 0
	if len(m.notes) > 0 {
		selected = max(0, slices.Index(columns, m.notes[m.taskCursor].Status))
	}
	start := min(max(0, selected-visible+1), len(columns)-visible)
	width := m.width / visible
	var panels []string
	for col := start; col < start+visible; col++ {
		status := columns[col]
		cards := m.boardStatusCards(status)
		label := fmt.Sprintf("%s · %d", status, len(cards))
		for _, s := range m.boardConfig.Statuses {
			if s.ID == status {
				if s.Role != "" {
					label += " · " + s.Role
				}
				if s.Run != "" {
					label += " · " + s.Run
				}
			}
		}
		var lines []string
		selectedLine := 0
		cardHeight := 3
		if m.boardConfig.Lane(status) == "done" {
			cardHeight = 1
		}
		for _, idx := range cards {
			if idx == m.taskCursor {
				selectedLine = len(lines)
			}
			lines = append(lines, m.boardCard(idx, width-4)...)
		}
		offset := max(0, selectedLine+cardHeight-max(1, height-2))
		panelWidth := width
		if col == start+visible-1 {
			panelWidth = m.width - width*(visible-1)
		}
		panels = append(panels, m.boardPanel(m.boardStyle(m.boardColor(status)).Render(label), lines[min(offset, len(lines)):], panelWidth, height, !m.workflowFocus && col == selected))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, panels...)
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

// Board panels must have exact dimensions even when the terminal is a short split.
func (m Model) boardPanel(title string, lines []string, width, height int, focused bool) string {
	color := m.theme.TextDim
	if focused {
		color = m.theme.Accent
	}
	border := m.boardStyle(color)
	label := " " + title + " "
	label = ansi.Truncate(label, max(0, width-3), "…")
	top := border.Render("╭─") + label + border.Render(strings.Repeat("─", max(0, width-3-ansi.StringWidth(label)))+"╮")
	out := []string{top}
	for i := 0; i < height-2; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, border.Render("│")+" "+boardFit(line, width-4)+" "+border.Render("│"))
	}
	out = append(out, border.Render("╰"+strings.Repeat("─", max(0, width-2))+"╯"))
	return strings.Join(out, "\n")
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
		body = m.boardPanel("Workflow · "+label, m.flowLines(m.width-4), m.width, h, true)
	} else if m.boardDrawer {
		body = m.boardPanel("Task · esc close", m.boardDetail(m.width-4, h-2), m.width, h, true)
	} else if m.width >= 120 {
		wfLines := m.workflowLines(m.width - 4)
		wh := min(len(wfLines)+2, max(3, h/3))
		wfStart := 0
		if m.workflowFocus {
			wfStart = max(0, m.workflowCursor-max(1, wh-2)+1)
		}
		body = m.boardColumnView(h-wh) + "\n" + m.boardPanel(fmt.Sprintf("Workflows %d · a add · e edit", len(m.boardWorkflows)), wfLines[min(wfStart, len(wfLines)):], m.width, wh, m.workflowFocus)
	} else {
		lw := m.width
		if m.boardWide() {
			lw = m.boardWidth()
		}
		order := m.boardOrder()
		var lines []string
		selected := 0
		lastStatus := ""
		for _, idx := range order {
			if status := m.notes[idx].Status; status != lastStatus {
				lines = append(lines, m.boardStyle(m.boardColor(status)).Bold(true).Render(status))
				lastStatus = status
			}
			if idx == m.taskCursor {
				selected = len(lines)
			}
			lines = append(lines, m.boardRow(idx, lw-4)...)
		}
		if len(lines) == 0 {
			lines = []string{m.boardStyle(m.theme.TextDim).Render("No tickets · a add")}
		}
		// Workflows sit under the ticket list: one line each, at most a third of the height.
		wfLines := m.workflowLines(lw - 4)
		wh := min(len(wfLines)+2, max(3, h/3))
		lh := h - wh
		start := min(len(lines), max(0, selected+2-(lh-2)))
		wfStart := 0
		if m.workflowFocus {
			wfStart = max(0, m.workflowCursor-max(1, wh-2)+1)
		}
		list := m.boardPanel(fmt.Sprintf("Board %d", len(order)), lines[start:], lw, lh, !m.workflowFocus) + "\n" +
			m.boardPanel(fmt.Sprintf("Workflows %d · a add · e edit", len(m.boardWorkflows)), wfLines[min(wfStart, len(wfLines)):], lw, wh, m.workflowFocus)
		panels := []string{list}
		if m.boardWide() {
			panels = append(panels, m.boardPanel("Task", m.boardDetail(m.width-lw-4, h-2), m.width-lw, h, false))
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, panels...)
	}
	hint := m.ticketHints()
	if m.workflowDelete != "" {
		hint = "Delete workflow " + m.workflowDelete + "? y / n"
	} else if m.workflowFocus && !m.boardFlow && !m.boardDrawer {
		hint = m.keyHints("enter view", "a add", "e edit", "Tab tickets", "? help")
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

func (m Model) boardDetail(w, rows int) []string {
	if w <= 0 || rows <= 0 {
		return nil
	}
	if len(m.notes) == 0 {
		return []string{boardFit("No tasks yet", w)}
	}
	return m.detailViewport(m.taskDetailLines(m.notes[m.taskCursor], w), w, rows, m.detailScroll)
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
	if m.boardMode && (m.boardDrawer || m.boardWide()) && len(m.notes) > 0 && (key == "a" || key == "A") && !m.workflowFocus && m.workflowDelete == "" {
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
		if key == "tab" {
			m.workflowFocus = true
			return m, nil, true
		}
	}
	if m.boardMode && (m.boardDrawer || (m.boardWide() && key != "j" && key != "k" && key != "up" && key != "down" && key != "g" && key != "G")) {
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
		}
		return m, nil, true
	}
	switch key {
	case "w":
		m.boardMode = true
		m.boardFlow, m.boardDrawer = true, false
		m.flowWorkflow = ""
		return m, nil, true
	case "enter", "i":
		m.boardMode = true
		m.boardDrawer = !m.boardDrawer
		m.detailScroll = 0
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
		if m.width >= 120 && !m.boardDrawer {
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
	order := m.boardOrder()
	if m.boardMode && m.width >= 120 {
		order = m.boardStatusCards(m.notes[m.taskCursor].Status)
	}
	pos := slices.Index(order, m.taskCursor)
	m.taskCursor = order[max(0, min(len(order)-1, pos+step))]
	m.detailScroll = 0
	return m, nil, true
}

func (m Model) detailMaxScroll() int {
	if len(m.notes) == 0 {
		return 0
	}
	w, rows := max(10, m.width-4), max(0, m.height-6)
	w, rows = m.width-4, max(0, m.height-5)
	if !m.boardDrawer && m.boardWide() {
		w = m.width - m.boardWidth() - 4
	}
	return max(0, len(m.taskDetailLines(m.notes[m.taskCursor], w))-max(0, rows-2))
}

func (m Model) ticketHints() string {
	return m.keyHints("h/l column", "j/k card", "n new", "s start", "enter detail", "e edit", "C comment", "H/L status", "x delete", "w workflow", "? help")
}

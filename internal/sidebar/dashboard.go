package sidebar

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tagent/internal/agents"
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
	taskTitle := "☷ Tasks"
	if m.taskTitle {
		taskTitle += " · Enter board"
	}
	var body string
	chatPanel := func(w, h int) string {
		return m.dashboardPanel("Chat", m.intakeLines(w-4, h-2), w, h, m.agentFocus == 3)
	}
	if w >= 90 {
		agents := m.agentPanels(g.left, g.bodyHeight)
		taskRows := m.taskLines(width-g.left-4, g.second-2)
		right := m.dashboardPanel(taskTitle, taskRows[1:], width-g.left, g.second, m.taskFocus) + "\n" + chatPanel(width-g.left, g.chat)
		body = lipgloss.JoinHorizontal(lipgloss.Top, agents, right)
	} else {
		agents := m.agentPanels(width, g.first)
		taskRows := m.taskLines(width-4, g.second-2)
		body = agents + "\n" + m.dashboardPanel(taskTitle, taskRows[1:], width, g.second, m.taskFocus) + "\n" + chatPanel(width, g.chat)
	}
	help := " ? help · Tab focus · ↑↓ select · Enter open · l log · i prompt"
	if m.agentFocus == 1 {
		help = " ↑↓ agent · e edit instruction"
	}
	if m.agentFocus == 3 {
		help = " Enter send · Tab focus · Esc cancel"
		if lipgloss.Width(help) > width {
			help = " Enter send · Tab · Esc"
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

func (m Model) chartLine(width int) string {
	days := m.chartDays
	if days == 0 {
		days = 7
	}
	if days == 1 {
		days = 1
	}
	window := metrics.TokenWindow(m.metricState, time.Now(), days)
	label := "24h"
	if days == 7 {
		label = "7d"
	} else if days == 30 {
		label = "30d"
	}
	if width < 28 {
		return " " + label + " " + window.Sparkline
	}
	return " tokens " + label + "  " + window.Sparkline + "  " + compactTokens(window.Total)
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
	return lipgloss.NewStyle().MaxWidth(width).Render(truncateEllipsis(s, width))
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
	title := "☷ Tasks"
	if len(m.notes) > 0 {
		title = fmt.Sprintf("☷ Tasks · %d/%d", min(m.taskCursor, len(m.notes)-1)+1, len(m.notes))
	}
	out := []string{m.sectionTitle(title, width, m.taskFocus)}
	if len(m.notes) == 0 {
		out = append(out, "  no tasks · task add <text>")
	}
	var lines []string
	cursorLine := 0
	for _, status := range m.boardColumns() {
		cards := m.boardStatusCards(status)
		lines = append(lines, m.boardStyle(m.boardColor(status)).Bold(true).Render(fmt.Sprintf(" %s %d", status, len(cards))))
		for _, idx := range cards {
			t := m.notes[idx]
			mark := "  "
			if idx == m.taskCursor {
				mark = "▌ "
				cursorLine = len(lines)
			}
			row := fmt.Sprintf("%sTA-%s %s", mark, t.ID, boardTitle(t))
			if worker := m.boardWorker(t); worker != "" {
				row += " · " + worker
			} else if len(t.Assignees) > 0 {
				row += " · " + t.Assignees[0]
			} else {
				for _, s := range m.boardConfig.Statuses {
					if s.ID == status && s.Role != "" {
						row += " · " + s.Role
					}
				}
			}
			if status == "blocked" {
				cfg, _ := tasks.ConfigForTask(m.boardDir, m.boardConfig, t)
				_, _, reason := tasks.BlockContext(t, cfg, m.boardHistory)
				if reason != "" {
					row += " · " + strings.Join(strings.Fields(reason), " ")
				}
			}
			if len(t.Comments) > 0 {
				row += fmt.Sprintf(" · %s %d", m.icons.Message, len(t.Comments))
			}
			lines = append(lines, m.boardStyle(m.boardColor(status)).Render(boardFit(row, width)))
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

// taskStart is the first visible task when height lines (title included)
// show a list of n tasks with the cursor on one of them.
func taskStart(cursor, n, height int) int {
	if n <= height-1 {
		return 0
	}
	return min(max(0, cursor-(height-2)), n-(height-1))
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
	color := m.theme.SectionTitle
	if focused {
		label = " ▶ " + label + " · FOCUS "
		return lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Reverse(true).Width(width).Render(fitLine(label, width))
	}
	text := "─ " + label + " "
	return lipgloss.NewStyle().Foreground(color).Render(fitLine(text+strings.Repeat("─", max(0, width-lipgloss.Width(text))), width))
}

func (m Model) dashboardPanel(title string, lines []string, width, height int, focused bool) string {
	width = max(4, width)
	height = max(3, height)
	color := m.theme.TextDim
	if focused {
		color = m.theme.Accent
		title = "▶ " + title
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
	// The title sits in the top border: ╭─ Title ─────╮
	bc := lipgloss.NewStyle().Foreground(color)
	label := " " + fitLine(title, max(1, width-6)) + " "
	fill := max(0, width-2-1-lipgloss.Width(label))
	top := bc.Render("╭─") + lipgloss.NewStyle().Foreground(color).Bold(true).Render(label) + bc.Render(strings.Repeat("─", fill)+"╮")
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
	selected, _ := paneIDAt(m.rows, m.cursor)
	dir := m.focusDir(selected)
	if dir == "" {
		return roles.ListGlobal(harness)
	}
	var defs []roles.Definition
	for _, a := range agents.List(dir, harness, nil) {
		// Board-only roles have no instruction file; the dashboard opens board.yaml for those.
		defs = append(defs, roles.Definition{Name: a.Name, Path: cmp.Or(a.Path, tasks.BoardConfigPath(dir)), Instructions: a.Instructions, Harness: a.Harness})
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

type dashGeom struct{ top, bodyHeight, left, first, second, chat int }

// dashGeometry is the dashboard layout shared by the view and mouse
// hit-testing: top = rows above the body, left = agents column width (wide),
// first/second = panel heights when stacked (narrow).
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
	g.second = max(6, g.bodyHeight/3) // fixed share; the list scrolls
	// Chat occupies the rest of the right column or a slice under tasks.
	if w >= 90 {
		g.chat = max(6, g.bodyHeight-g.second)
		return g
	}
	g.chat = max(8, g.bodyHeight/3)
	g.first = max(8, g.bodyHeight-g.second-g.chat)
	return g
}

// overviewLines is the Overview panel: what needs the user first, then the
// state of agents, tasks and the repo, then recent usage.
func (m Model) overviewLines(width int) []string {
	dim := lipgloss.NewStyle().Foreground(m.theme.TextDim)
	attention, now := metrics.OverviewFacts(m.scopePanes(""), m.notes, m.metricState)
	var out []string
	if attention.Needed {
		out = append(out, lipgloss.NewStyle().Foreground(m.theme.StatusConfirm).Bold(true).Render(fitLine(m.icons.Warning+" "+attention.Text, width)))
	} else {
		out = append(out, dim.Render("✓ "+attention.Text))
	}
	if len(now) > 0 {
		out = append(out, lipgloss.NewStyle().Foreground(m.theme.StatusRunning).Render(fitLine("Now     "+strings.Join(now, " · "), width)))
	}
	if m.height >= 30 { // short panes keep the space for agents and tasks
		for _, l := range quota.Get().Lines() {
			out = append(out, fitLine(l, width))
		}
	}
	// Tokens: today and the window the charts show.
	today := metrics.TokenWindow(m.metricState, time.Now(), 7).Today
	out = append(out, fitLine("Tokens  today "+compactTokens(today), width),
		lipgloss.NewStyle().Foreground(m.theme.StatusRunning).Render(m.chartLine(width)))
	return out
}

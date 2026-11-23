package sidebar

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type shortcutSection struct {
	title string
	keys  [][2]string
}

// helpSections is the complete reference, available with a from contextual help.
var helpSections = []shortcutSection{
	{"Everywhere", [][2]string{{"?", "help"}, {",", "settings"}, {"q", "quit"}, {"Tab", "next panel"}, {"[ ]", "chart range (1 / 7 / 30 days)"}, {"o", "expand / collapse"}}},
	{"Agents", [][2]string{
		{"N", "new agent or receptionist (Chat)"}, {"j k ↑ ↓", "select agent"}, {"n", "next agent needing you"}, {"enter", "on ! row: answers; again: jump to pane"}, {"1-4", "answer open approval"}, {"space", "quick action on first alert"}, {"1-9", "jump to window"}, {"/", "filter"}, {"r", "switch repo"}, {"p", "pin"}, {"f", "handoff sizing to this terminal"}, {"v x", "select / delete worktrees"}, {"S", "clear subagents"}, {"u", "undo"}, {"l", "agent log"}, {"i", "send prompt"},
	}},
	{"Board columns vs workflows", [][2]string{{"board.yaml", "columns define the default flow"}, {"s / start", "attach a workflow to override one ticket's flow"}, {"choose", "board for routine work; workflow for a special sequence"}}},
	{"Agent definitions", [][2]string{{"j k", "move"}, {"e enter", "edit instruction file"}}},
	{"Tasks", ticketShortcuts([2]string{"Tab / b", "focus next panel / open board"})},
	{"Chat", [][2]string{{"Enter", "send / approve / create"}, {"ask", "which agents are running? (read-only answer)"}, {"!reload", "propose operation; approval required"}, {"snapshot", "latest local data, not history"}, {"Tab", "change focus"}, {"Esc", "close; Enter with text starts fresh after answer"}, {"e", "edit command / draft"}, {"r", "start draft"}}},
	{"Board · Tickets", ticketShortcuts([2]string{"Tab / esc", "focus workflows / back"})},
	{"Board · Workflows", [][2]string{{"Tab", "focus tickets"}, {"j k ↑ ↓", "select workflow"}, {"enter w", "view selected workflow"}, {"a", "new workflow name, then $EDITOR"}, {"e", "edit selected workflow"}, {"x", "delete workflow (y/n confirm)"}, {"esc q b", "back to dashboard"}}},
	{"Board · Workflow view", [][2]string{{"[ ]", "browse workflows"}, {"e c", "edit displayed workflow / board config"}, {"esc w q", "close workflow view"}, {"ctrl+c", "quit"}}},
	{"Board · Task detail", [][2]string{{"j k ↑ ↓", "scroll task detail"}, {"pgup pgdn", "scroll page"}, {"g G", "top / bottom"}, {"esc", "close detail drawer"}, {"enter i", "toggle detail drawer"}, {"e", "edit task"}, {"c / C", "blocked: @role reply resumes; C: comment"}, {"a / A", "view attachments / attach file"}}},
	{"Intake", [][2]string{{"enter", "submit request / answer; approve draft or command"}, {"r", "draft: create + start workflow"}, {"w", "draft: cycle workflow (including none)"}, {"e", "command: edit line; draft / malformed: $EDITOR"}, {"backspace", "delete input character"}, {"esc", "cancel intake"}}},
	{"Standalone (outside tmux)", [][2]string{{"enter", "type into selected agent"}, {"ctrl+] ctrl+\\", "back to list (or click it)"}, {"N", "start a new agent here"}, {"z", "layout: split · half · agent · dashboard"}}},
}

func (m Model) helpSection() shortcutSection {
	title := "Agents"
	switch {
	case m.taskForm == "intake":
		title = "Intake"
	case m.boardMode && m.boardDrawer:
		title = "Board · Task detail"
	case m.boardMode && m.boardFlow:
		title = "Board · Workflow view"
	case m.boardMode && (m.workflowFocus || m.taskForm == "workflow-new"):
		title = "Board · Workflows"
	case m.boardMode:
		title = "Board · Tickets"
	case m.taskForm != "":
		title = "Tasks"
	case m.dashboard && m.agentFocus == 3:
		title = "Chat"
	case m.dashboard && m.agentFocus == 1:
		title = "Agent definitions"
	case m.taskFocus || m.dashboard && m.agentFocus == 2:
		title = "Tasks"
	case m.standalone:
		title = "Standalone (outside tmux)"
	}
	for _, s := range helpSections {
		if s.title == title {
			return s
		}
	}
	return helpSections[1]
}

func (m Model) helpView() string {
	width := max(20, m.width-2)
	height := m.height
	if height <= 0 {
		height = 24
	}
	key := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(m.theme.TextDim)
	section := m.helpSection()
	sections := []shortcutSection{section}
	title := "? " + section.title
	if m.helpAll {
		sections = helpSections
		title = "? All shortcuts"
	}
	var lines []string
	for _, sec := range sections {
		if m.helpAll {
			lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(m.theme.TextPrimary).Render(sec.title))
		}
		for _, kv := range sec.keys {
			pad := strings.Repeat(" ", max(1, 12-lipgloss.Width(kv[0])))
			lines = append(lines, ansi.Truncate("  "+key.Render(kv[0])+pad+dim.Render(kv[1]), width-2, "…"))
		}
		lines = append(lines, "")
	}
	if !m.helpAll {
		for _, kv := range helpSections[0].keys[:3] {
			lines = append(lines, "  "+key.Render(kv[0])+"  "+dim.Render(kv[1]))
		}
	}
	room := max(3, height-4)
	start := min(max(0, m.helpScroll), max(0, len(lines)-room))
	end := min(len(lines), start+room)
	foot := "j/k scroll · any other key closes"
	if !m.helpAll {
		foot = "a: all shortcuts · j/k scroll · Esc close"
	}
	heading := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Accent).Render(title)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.Accent).Width(width).Render(heading + "\n" + strings.Join(lines[start:end], "\n") + "\n" + dim.Render(foot))
}

func ticketShortcuts(nav [2]string) [][2]string {
	return append([][2]string{{"h l", "select board column (wide pane)"}, {"j k ↑ ↓", "select ticket"}, {"n", "new via intake"}, {"s", "start: workflow picker"}, {"enter i", "ticket detail"}, {"e", "edit in $EDITOR"}, {"c / C", "blocked: @role reply resumes; C: comment"}, {"H L", "previous / next status"}, {"x", "delete (confirm)"}, {"w", "workflow view"}, {"?", "help"}}, nav)
}

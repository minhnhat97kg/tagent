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
	{"Everywhere", [][2]string{{"?", "help"}, {",", "settings"}, {"B", "board config (default workflow, agents, triggers)"}, {"q", "quit"}, {"Tab S-Tab", "next / previous card: Agents, Usage, Tasks, Workflows"}, {"[ ]", "chart range (1 / 7 / 30 days)"}, {"o", "expand / collapse (prefix + A)"}, {"b", "board (from the Tasks card)"}}},
	{"Agents", [][2]string{
		{"N", "new agent"}, {"j k ↑ ↓", "select agent"}, {"n", "next agent needing you"}, {"enter", "on ! row: answers; again: jump to pane"}, {"1-4", "answer open approval"}, {"space", "quick action on first alert"}, {"1-9", "jump to window"}, {"/", "filter"}, {"r", "switch repo"}, {"p", "pin"}, {"f", "handoff sizing to this terminal"}, {"v x", "select / delete worktrees"}, {"S", "clear subagents"}, {"u", "undo"}, {"l", "agent log"}, {"i", "send prompt"},
	}},
	{"Workflows vs board.yaml", [][2]string{{"workflows", ".tasks/workflows/*.yml define every flow (jobs, runs, loops)"}, {"board.yaml", "roles, triggers and default_workflow only"}, {"s / start", "start with default_workflow or pick another workflow"}}},
	{"Usage", [][2]string{{"h l [ ]", "chart range (1 / 7 / 30 days)"}, {"Tab", "next panel"}}},
	{"Tasks", ticketShortcuts([2]string{"Tab / b", "focus next panel / open board"})},
	{"Board · Tickets", ticketShortcuts([2]string{"Tab / esc", "focus workflows / back"})},
	{"Board · Agents", [][2]string{{"Tab", "focus tickets"}, {"j k ↑ ↓", "select agent"}, {"enter", "show detail"}, {"m", "model (new panes only)"}, {"e", "edit instructions in $EDITOR"}, {"a", "new agent"}, {"x", "delete agent (y/n confirm)"}, {"t", "swap Workflows / Agents tab (and back)"}, {"esc q b", "back to dashboard"}}},
	{"Board · Workflows", [][2]string{{"Tab S-Tab", "focus tickets (dashboard: next card)"}, {"t", "swap Workflows / Agents (dashboard also h l [ ])"}, {"j k ↑ ↓", "select workflow"}, {"enter w", "view selected workflow"}, {"B", "board config summary"}, {"a", "new workflow name, then $EDITOR"}, {"e", "edit selected workflow"}, {"x", "delete workflow (y/n confirm)"}, {"esc q b", "back to dashboard"}}},
	{"Board · Workflow view", [][2]string{{"[ ]", "browse workflows"}, {"e E", "edit displayed workflow (board config if none)"}, {"esc w q", "close workflow view"}, {"ctrl+c", "quit"}}},
	{"Board · Config view", [][2]string{{"B", "open from the board"}, {"enter", "pick the default workflow (saved to board.yaml)"}, {"j k ↑ ↓", "scroll default workflow, roles, triggers"}, {"g G", "top / bottom"}, {"E", "edit .tasks/board.yaml in $EDITOR"}, {"esc w q B", "close"}, {"ctrl+c", "quit"}}},
	{"Board · Task detail", [][2]string{{"tab h l 1-4", "switch Details / Chat / Activity / Run"}, {"j k ↑ ↓", "scroll task detail"}, {"pgup pgdn", "scroll page"}, {"g G", "top / bottom"}, {"esc", "close detail drawer"}, {"enter i", "toggle detail drawer; Chat: start the chat"}, {"i / S", "Chat: message the agent / save its reply to the ticket"}, {"p / D", "cycle priority / edit dependencies"}, {"e", "edit task"}, {"c", "Activity: comment box (@ suggests roles, enter posts, esc cancels)"}, {"c / C", "blocked: @role reply resumes; C: comment"}, {"a / A", "view attachments / attach file"}}},
	{"Standalone (outside tmux)", [][2]string{{"enter", "type into selected agent"}, {"ctrl+] ctrl+\\", "back to list (or click it)"}, {"N", "start a new agent here"}, {"z", "layout: split · half · agent · dashboard"}}},
}

func (m Model) helpSection() shortcutSection {
	title := "Agents"
	switch {
	case m.boardMode && m.boardDrawer:
		title = "Board · Task detail"
	case m.boardMode && m.boardConfigView:
		title = "Board · Config view"
	case m.boardMode && m.boardFlow:
		title = "Board · Workflow view"
	case m.boardMode && m.agentsOpen && (m.workflowFocus || m.taskForm == "agent-new" || m.taskForm == "agent-model"):
		title = "Board · Agents"
	case m.boardMode && (m.workflowFocus || m.taskForm == "workflow-new"):
		title = "Board · Workflows"
	case m.boardMode:
		title = "Board · Tickets"
	case m.taskForm != "":
		title = "Tasks"
	case m.dashboard && m.agentFocus == 4:
		title = "Board · Workflows"
	case m.dashboard && m.agentFocus == 3:
		title = "Usage"
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
	return append([][2]string{{"h l", "select lane: todo / in progress / blocked / done (or click its tab)"}, {"j k ↑ ↓", "select ticket"}, {"n", "new ticket ($EDITOR)"}, {"s", "start: workflow picker"}, {"p / D", "cycle priority / edit dependencies"}, {"enter i", "ticket detail"}, {"e", "edit in $EDITOR"}, {"c C", "comment on ticket (blocked: prefills @role reply)"}, {"H L", "previous / next status (each card shows its status)"}, {"x", "delete (confirm)"}, {"a / A", "attachments: view or open (images, pdf) / attach file"}, {"w", "workflow view"}, {"B", "board config summary (default workflow, roles, triggers; E edits)"}, {"?", "help"}}, nav)
}

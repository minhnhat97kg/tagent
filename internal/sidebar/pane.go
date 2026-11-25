package sidebar

import (
	"slices"

	tea "github.com/charmbracelet/bubbletea"
)

// A pane owns the keys of one card or view. handleKey tries the focused pane
// first and then the global keys (globalKey); no other pane sees the key.
type pane interface {
	Title() string // the ? help section
	Hints(m Model) []hint
	// Key reports whether the pane used the key; an unhandled key falls to the
	// global keys (quit, settings, expand, Tab, chart range).
	Key(m Model, key string) (Model, tea.Cmd, bool)
}

type paneID int

const (
	paneAgents paneID = iota // dashboard card and the narrow sidebar list
	paneUsage
	paneTasks // dashboard card
	paneWorkflows
	paneAgentsTab // the Workflows card swapped to agents
	paneBoardTickets
	paneBoardWorkflows
	paneBoardAgents
	paneWorkflowView
	paneBoardConfig
	paneTaskDetail
	paneLayoutEdit
)

// focused derives the pane that gets the keys from the screen and focus flags.
func (m Model) focused() paneID {
	if m.layoutEdit {
		return paneLayoutEdit
	}
	if m.boardMode {
		switch {
		case m.boardDrawer:
			return paneTaskDetail
		case m.boardFlow && m.boardConfigView:
			return paneBoardConfig
		case m.boardFlow:
			return paneWorkflowView
		case m.focus == cardWorkflows || m.workflowDelete != "":
			return map[bool]paneID{true: paneBoardAgents, false: paneBoardWorkflows}[m.agentsOpen]
		}
		return paneBoardTickets
	}
	if !m.dashboard {
		return paneAgents
	}
	switch {
	case m.focus == cardTasks:
		return paneTasks
	case m.focus == cardUsage:
		return paneUsage
	case m.focus == cardWorkflows:
		return map[bool]paneID{true: paneAgentsTab, false: paneWorkflows}[m.agentsOpen]
	}
	return paneAgents
}

// keyFn adapts a key function to the pane interface.
type keyFn func(m Model, key string) (Model, tea.Cmd, bool)

type simplePane struct {
	title string
	hints func(m Model) []hint
	key   keyFn
}

func (p simplePane) Title() string                                  { return p.title }
func (p simplePane) Hints(m Model) []hint                           { return p.hints(m) }
func (p simplePane) Key(m Model, key string) (Model, tea.Cmd, bool) { return p.key(m, key) }

var panes = map[paneID]pane{
	paneAgents:         simplePane{"Agents", Model.agentsHints, Model.agentsKey},
	paneUsage:          simplePane{"Usage", Model.usageHints, Model.usageKey},
	paneTasks:          simplePane{"Tasks", Model.tasksCardHints, Model.tasksCardKey},
	paneWorkflows:      simplePane{"Workflows", Model.dashWorkflowsHints, Model.dashWorkflowsKey},
	paneAgentsTab:      simplePane{"Workflows · Agents", Model.dashWorkflowsHints, Model.dashWorkflowsKey},
	paneBoardTickets:   simplePane{"Board · Tickets", Model.boardTicketsHints, Model.ticketsKey},
	paneBoardWorkflows: simplePane{"Board · Workflows", Model.boardWorkflowsHints, Model.workflowPanelKey},
	paneBoardAgents:    simplePane{"Board · Agents", Model.boardAgentsHints, Model.workflowPanelKey},
	paneWorkflowView:   simplePane{"Board · Workflow view", Model.flowHints, Model.flowKey},
	paneBoardConfig:    simplePane{"Board · Config view", Model.configHints, Model.configKey},
	paneTaskDetail:     simplePane{"Board · Task detail", Model.detailHints, Model.detailKey},
	paneLayoutEdit:     simplePane{"Layout", layoutHints, Model.layoutKey},
}

// hint is one key of a pane. Keys and Foot show in the footer line ("j/k card";
// empty: not in the footer); HelpKeys and Help are its row in ? (Help empty: not
// listed there). A footer key that the pane does not handle is a test failure.
type hint struct{ Keys, Foot, HelpKeys, Help string }

// footerHints is the key-hint line of the focused pane, generated from its Hints.
func (m Model) footerHints() string {
	if m.boardDrawer && m.commentBox { // the open composer owns the keys
		return m.keyHints("enter send", "tab @role", "esc cancel")
	}
	if m.taskForm == "attachments" {
		return m.keyHints("j/k select", "enter view", "esc cancel")
	}
	var pairs []string
	for _, h := range panes[m.focused()].Hints(m) {
		if h.Keys != "" && h.Foot != "" {
			pairs = append(pairs, h.Keys+" "+h.Foot)
		}
	}
	return m.keyHints(append(pairs, "? help")...)
}

// globalHints are the keys every pane shares (the "Everywhere" help section).
var globalHints = []hint{
	{"", "", "?", "help"}, {"", "", ",", "settings"}, {"", "", "B", "board config (default workflow, agents, triggers)"}, {"", "", "q", "quit"},
	{"", "", "Tab S-Tab", "next / previous card: Agents, Usage, Tasks, Workflows"}, {"", "", "[ ]", "chart range (1 / 7 / 30 days)"},
	{"", "", "o", "expand / collapse (prefix + A)"}, {"", "", "b", "board (from the Tasks card)"},
	{"", "", "M", "move and resize the panes (saved per width)"},
}

// globalKey is what every pane shares, tried after the focused pane.
func (m Model) globalKey(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "q", "ctrl+c":
		// cursorDirty still being set means the most recent @sidebar_cursor
		// write is waiting for the next AnimTick (debounce ~200ms): flush it
		// first or quitting right here loses it.
		if m.cursorDirty {
			return m, tea.Sequence(saveCursorCmd(m.cursor), tea.Quit), true
		}
		return m, tea.Quit, true
	case ",":
		return m, OpenSettings(m.tmuxPane), true
	case "o":
		// Expand this pane into the full dashboard (expand.go). The respawn
		// replaces this process, so nothing after it matters.
		if m.tmuxPane == "" {
			return m, nil, true
		}
		pane := m.tmuxPane
		expand := func() tea.Msg { SetExpanded(pane, true); return nil }
		if m.cursorDirty {
			return m, tea.Sequence(saveCursorCmd(m.cursor), expand), true
		}
		return m, expand, true
	}
	if !m.dashboard || m.boardMode {
		return m, nil, false
	}
	switch key {
	case "tab", "shift+tab":
		step := 1
		if key == "shift+tab" {
			step = len(focusOrder) - 1
		}
		m.focus = focusOrder[(slices.Index(focusOrder, m.focus)+step)%len(focusOrder)]
		return m.followPane(cardIDs[m.focus]), nil, true
	case "B": // board config, from any dashboard card; esc returns here
		m.boardMode, m.boardFlow, m.boardConfigView, m.boardDrawer = true, true, true, false
		m.flowWorkflow, m.boardConfigScroll, m.flowFromDash = "", 0, true
		return m, nil, true
	case "[", "]": // chart range, from any card
		return m.chartStep(key == "]"), nil, true
	case "M": // move and resize the panes (pane_layout.go); not from the full-pane detail
		if m.boardMode || (m.agentsOpen && m.agentDetail) {
			return m, nil, false
		}
		return m.startLayoutEdit(), nil, true
	}
	return m, nil, false
}

// chartStep widens (true) or narrows the usage chart: 1, 7, 30 days.
func (m Model) chartStep(wider bool) Model {
	switch {
	case wider && m.chartDays == 1, !wider && m.chartDays == 30:
		m.chartDays = 7
	case wider && m.chartDays == 7:
		m.chartDays = 30
	case !wider && m.chartDays == 7:
		m.chartDays = 1
	}
	return m
}

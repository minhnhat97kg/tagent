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
)

// focused derives the pane that gets the keys from the screen and focus flags.
func (m Model) focused() paneID {
	if m.boardMode {
		switch {
		case m.boardDrawer:
			return paneTaskDetail
		case m.boardFlow && m.boardConfigView:
			return paneBoardConfig
		case m.boardFlow:
			return paneWorkflowView
		case m.workflowFocus || m.workflowDelete != "":
			return map[bool]paneID{true: paneBoardAgents, false: paneBoardWorkflows}[m.agentsOpen]
		}
		return paneBoardTickets
	}
	if !m.dashboard {
		return paneAgents
	}
	switch {
	case m.taskFocus || m.agentFocus == 2:
		return paneTasks
	case m.agentFocus == 3:
		return paneUsage
	case m.agentFocus == 4:
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

func noHints(Model) []hint { return nil }

var panes = map[paneID]pane{
	paneAgents:         simplePane{"Agents", noHints, Model.agentsKey},
	paneUsage:          simplePane{"Usage", noHints, Model.usageKey},
	paneTasks:          simplePane{"Tasks", noHints, Model.tasksCardKey},
	paneWorkflows:      simplePane{"Board · Workflows", noHints, Model.dashWorkflowsKey},
	paneAgentsTab:      simplePane{"Board · Workflows", noHints, Model.dashWorkflowsKey},
	paneBoardTickets:   simplePane{"Board · Tickets", noHints, Model.ticketsKey},
	paneBoardWorkflows: simplePane{"Board · Workflows", noHints, Model.workflowPanelKey},
	paneBoardAgents:    simplePane{"Board · Agents", noHints, Model.workflowPanelKey},
	paneWorkflowView:   simplePane{"Board · Workflow view", noHints, Model.flowKey},
	paneBoardConfig:    simplePane{"Board · Config view", noHints, Model.configKey},
	paneTaskDetail:     simplePane{"Board · Task detail", noHints, Model.detailKey},
}

// hint is one key of a pane: Foot is its short label in the footer line
// (empty: the key is only listed by ?), Help the sentence ? shows.
type hint struct{ Keys, Foot, Help string }

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
		m.agentFocus = focusOrder[(slices.Index(focusOrder, m.agentFocus)+step)%len(focusOrder)]
		m.taskFocus = m.agentFocus == 2
		m.workflowFocus = m.agentFocus == 4
		return m, nil, true
	case "B": // board config, from any dashboard card; esc returns here
		m.boardMode, m.boardFlow, m.boardConfigView, m.boardDrawer = true, true, true, false
		m.flowWorkflow, m.boardConfigScroll, m.flowFromDash = "", 0, true
		return m, nil, true
	case "[", "]": // chart range, from any card
		return m.chartStep(key == "]"), nil, true
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

// usageKey: h/l and the arrows pick the chart range ([ ] work from any card).
func (m Model) usageKey(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "h", "left":
		return m.chartStep(false), nil, true
	case "l", "right":
		return m.chartStep(true), nil, true
	}
	return m, nil, false
}

// tasksCardKey is the dashboard Tasks card: the ticket keys without leaving
// the dashboard; b opens the board.
func (m Model) tasksCardKey(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "tab", "shift+tab", "q", "ctrl+c", ",", "o", "B", "[", "]":
		return m, nil, false
	case "b":
		return m.openBoard(), nil, true
	}
	if m.boardDir == "" || m.boardDir != m.repoDir && m.repoDir != "" {
		m = m.boardContext()
	}
	m.boardDrawer, m.boardFlow, m.workflowFocus = false, false, false
	m, cmd, _ := m.ticketsKey(key)
	return m, cmd, true
}

// dashWorkflowsKey is the dashboard's Workflows card and its Agents tab.
func (m Model) dashWorkflowsKey(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "esc", "backspace":
		if m.agentsOpen && m.agentDetail { // back from an agent's detail to the list
			m.agentDetail = false
			return m, nil, true
		}
		if key == "backspace" {
			return m, nil, true
		}
		m.agentFocus, m.taskFocus, m.workflowFocus = 2, true, false
		return m, nil, true
	case "t", "h", "l", "[", "]": // swap Workflows <-> Agents (same keys as the other cards' tabs)
		m.agentsOpen, m.agentDetail, m.agentCursor = !m.agentsOpen, false, 0
		if m.agentsOpen {
			m = m.loadAgents()
		}
		return m, nil, true
	case "j", "k", "up", "down", "a", "e", "x", "w", "enter", "y", "n", "m":
		m, cmd, _ := m.workflowPanelKey(key)
		if m.boardFlow {
			m.boardMode, m.boardDrawer, m.workflowFocus, m.flowFromDash = true, false, false, true
		}
		return m, cmd, true
	}
	return m, nil, false
}

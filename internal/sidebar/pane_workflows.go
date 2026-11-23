package sidebar

import tea "github.com/charmbracelet/bubbletea"

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

// dashWorkflowsHints: the dashboard Workflows card, its Agents tab and the agent detail.
func (m Model) dashWorkflowsHints() []hint {
	switch {
	case m.agentsOpen && m.agentDetail:
		return []hint{{"esc", "back", "esc", "back to the agent list"}, {"m", "model", "m", "model (new panes only)"}, {"e", "edit", "e", "edit instructions in $EDITOR"}, {"j/k", "agent", "j k ↑ ↓", "select agent"}}
	case m.agentsOpen:
		return []hint{
			{"h/l", "workflows", "t h l [ ]", "swap Workflows / Agents"}, {"enter", "detail", "enter", "show detail"}, {"m", "model", "m", "model (new panes only)"}, {"a", "add", "a", "new agent"},
			{"Tab", "focus", "", ""}, {"", "", "j k ↑ ↓", "select agent"}, {"", "", "e", "edit instructions in $EDITOR"}, {"", "", "x", "delete agent (y/n confirm)"}, {"", "", "esc", "back to Tasks"},
		}
	}
	return []hint{
		{"h/l", "agents", "t h l [ ]", "swap Workflows / Agents"}, {"enter", "view", "enter w", "view selected workflow"}, {"a", "add", "a", "new workflow name, then $EDITOR"}, {"B", "board config", "B", "board config summary"},
		{"Tab", "focus", "", ""}, {"", "", "j k ↑ ↓", "select workflow"}, {"", "", "e", "edit selected workflow"}, {"", "", "x", "delete workflow (y/n confirm)"}, {"", "", "esc", "back to Tasks"},
	}
}

func (m Model) boardWorkflowsHints() []hint {
	return []hint{
		{"enter", "view", "enter w", "view selected workflow"}, {"a", "add", "a", "new workflow name, then $EDITOR"}, {"e", "edit", "e", "edit selected workflow"}, {"x", "delete", "x", "delete workflow (y/n confirm)"},
		{"t", "agents", "t", "swap Workflows / Agents"}, {"B", "config", "B", "board config summary"}, {"Tab", "tickets", "Tab S-Tab", "focus tickets"},
		{"", "", "j k ↑ ↓", "select workflow"}, {"", "", "esc q b", "back to dashboard"},
	}
}

func (m Model) boardAgentsHints() []hint {
	return []hint{
		{"enter", "detail", "enter", "show detail"}, {"m", "model", "m", "model (new panes only)"}, {"e", "edit", "e", "edit instructions in $EDITOR"}, {"a", "add", "a", "new agent"}, {"x", "delete", "x", "delete agent (y/n confirm)"},
		{"t", "close", "t", "swap Workflows / Agents tab (and back)"}, {"Tab", "tickets", "Tab", "focus tickets"},
		{"", "", "j k ↑ ↓", "select agent"}, {"", "", "esc q b", "back to dashboard"},
	}
}

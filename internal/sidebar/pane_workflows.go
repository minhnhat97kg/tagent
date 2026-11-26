package sidebar

// workflowsHints: the Workflows card (the agent definitions live on the Agents card).
func (m Model) workflowsHints() []hint {
	return []hint{
		{"enter", "view", "enter w", "view selected workflow"}, {"a", "add", "a", "new workflow name, then $EDITOR"}, {"B", "board config", "B", "board config summary"},
		{"Tab", "focus", "", ""}, {"", "", "j k ↑ ↓", "select workflow"}, {"", "", "e", "edit selected workflow"}, {"", "", "x", "delete workflow (y/n confirm)"}, {"", "", "esc", "back to Tasks"},
	}
}

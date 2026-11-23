package sidebar

import tea "github.com/charmbracelet/bubbletea"

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

func (m Model) usageHints() []hint {
	return []hint{
		{"h/l", "or [ ] range", "h l [ ]", "chart range (1 / 7 / 30 days)"},
		{"Tab/S-Tab", "panel", "Tab", "next panel"},
	}
}

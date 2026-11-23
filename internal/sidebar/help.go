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

// helpOrder is the order of the sections in the full list.
var helpOrder = []paneID{paneAgents, paneUsage, paneTasks, paneWorkflows, paneAgentsTab, paneBoardTickets, paneBoardAgents, paneBoardWorkflows, paneWorkflowView, paneBoardConfig, paneTaskDetail}

// helpNotes and helpStandalone are the sections that are not a pane's keys.
var (
	helpNotes      = shortcutSection{"Workflows vs board.yaml", [][2]string{{"workflows", ".tasks/workflows/*.yml define every flow (jobs, runs, loops)"}, {"board.yaml", "roles, triggers and default_workflow only"}, {"s / start", "start with default_workflow or pick another workflow"}}}
	helpStandalone = shortcutSection{"Standalone (outside tmux)", [][2]string{{"enter", "type into selected agent"}, {"ctrl+] ctrl+\\", "back to list (or click it)"}, {"N", "start a new agent here"}, {"z", "layout: split · half · agent · dashboard"}}}
)

// paneSection is a pane's ? section, built from its Hints.
func (m Model) paneSection(id paneID) shortcutSection {
	p := panes[id]
	sec := shortcutSection{title: p.Title()}
	for _, h := range p.Hints(m) {
		if h.Help != "" {
			sec.keys = append(sec.keys, [2]string{h.HelpKeys, h.Help})
		}
	}
	return sec
}

func globalSection() shortcutSection {
	sec := shortcutSection{title: "Everywhere"}
	for _, h := range globalHints {
		sec.keys = append(sec.keys, [2]string{h.HelpKeys, h.Help})
	}
	return sec
}

// helpSections is the complete reference, available with a from contextual help.
func (m Model) helpSections() []shortcutSection {
	out := []shortcutSection{globalSection(), m.paneSection(paneAgents), helpNotes}
	for _, id := range helpOrder[1:] {
		out = append(out, m.paneSection(id))
	}
	return append(out, helpStandalone)
}

// helpSection is the focused pane's section.
func (m Model) helpSection() shortcutSection {
	switch {
	case m.standalone && !m.dashboard && !m.boardMode:
		return helpStandalone
	case m.taskForm != "" && !m.boardMode:
		return m.paneSection(paneTasks)
	}
	return m.paneSection(m.focused())
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
		sections = m.helpSections()
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
		for _, kv := range globalSection().keys[:3] {
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

package sidebar

import (
	"fmt"
	"slices"
	"strings"

	"tagent/internal/agents"
	"tagent/internal/roles"
	"tagent/internal/tmux"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The agents panel shares the board's bottom strip with workflows (t toggles it).
// Changes go through agents.Save, so the TUI and web write the same files.

type agentsEditedMsg struct{ err error }

func (m Model) agentHarness() string { h, _ := tmux.GetOption(tmux.AgentOption); return h }

func (m Model) loadAgents() Model {
	name := m.selectedAgentName()
	m.agents = agents.List(m.boardDir, m.agentHarness(), agents.LivePanes())
	if i := slices.IndexFunc(m.agents, func(a agents.Agent) bool { return a.Name == name }); name != "" && i >= 0 {
		m.agentCursor = i
	}
	m.agentCursor = max(0, min(m.agentCursor, len(m.agents)-1))
	return m
}

func (m Model) selectedAgent() (agents.Agent, bool) {
	if len(m.agents) == 0 {
		return agents.Agent{}, false
	}
	return m.agents[max(0, min(m.agentCursor, len(m.agents)-1))], true
}

// agentLines is the strip's content: one line per agent, or the selected agent's detail.
func (m Model) agentStripLines(width int) []string {
	if len(m.agents) == 0 {
		return []string{m.boardStyle(m.theme.TextDim).Render("No agents · a add")}
	}
	dim := m.boardStyle(m.theme.TextMuted)
	if a, ok := m.selectedAgent(); ok && m.agentDetail {
		used := strings.Join(a.UsedBy, ", ")
		if len(a.UsedBy) == 0 {
			used = "not used"
		}
		first, _, _ := strings.Cut(strings.TrimSpace(a.Instructions), "\n")
		row := func(label, value string) string {
			return boardFit(dim.Render(fmt.Sprintf("%-9s", label))+value, width)
		}
		model := a.Model
		if model == "" {
			model = "default"
		}
		running := dim.Render("idle")
		if a.Running > 0 {
			running = m.boardStyle(m.theme.StatusRunning).Render(fmt.Sprintf("● %d running", a.Running))
		}
		if first == "" {
			first = "no instructions"
		}
		title := m.boardStyle(m.theme.TextPrimary).Bold(true).Render(a.Name) + "  " + dim.Render(agentScope(a))
		if a.Source == "global" {
			title += dim.Render(" · read-only")
		}
		return []string{
			boardFit(title, width),
			row("Harness", a.Harness),
			row("Model", model),
			row("Used by", used),
			row("Live", running),
			row("Prompt", dim.Render(first)),
		}
	}
	// One aligned row per agent: name, harness, model, then the live count.
	nameW, harnessW := 0, 0
	for _, a := range m.agents {
		nameW, harnessW = max(nameW, lipgloss.Width(a.Name)), max(harnessW, lipgloss.Width(a.Harness))
	}
	var lines []string
	for i, a := range m.agents {
		mark := "  "
		if m.workflowFocus && i == m.agentCursor {
			mark = "❯ "
		}
		model := a.Model
		if model == "" {
			model = "default"
		} else if j := strings.LastIndex(model, "/"); j >= 0 {
			model = model[j+1:]
		}
		run := ""
		if a.Running > 0 {
			run = "  " + m.boardStyle(m.theme.StatusRunning).Render(fmt.Sprintf("● %d", a.Running))
		}
		name := m.boardStyle(m.theme.TextPrimary).Bold(true).Render(a.Name) + strings.Repeat(" ", nameW-lipgloss.Width(a.Name))
		lines = append(lines, boardFit(mark+name+"  "+dim.Render(a.Harness+strings.Repeat(" ", harnessW-lipgloss.Width(a.Harness))+"  "+model)+"  "+m.boardStyle(m.theme.TextDim).Render(agentScope(a))+run, width))
	}
	return lines
}

// agentScope is "global" (~/.config/tmux-agent) or "local" (this repo / board).
func agentScope(a agents.Agent) string {
	if a.Source == "global" {
		return "global"
	}
	return "local"
}

func agentSummary(a agents.Agent) string {
	model := a.Model
	if model == "" {
		model = "default model"
	}
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:] // drop the provider prefix, it only crowds the row
	}
	return a.Harness + " · " + model
}

// stripPanel is the bottom card's body: workflows, or agents while the agents panel is open.
func (m Model) stripPanel(width int) (lines []string, cursor, n int) {
	if m.agentsOpen {
		return m.agentStripLines(width), m.agentCursor, len(m.agents)
	}
	return m.workflowLines(width), m.workflowCursor, len(m.boardWorkflows)
}

func (m Model) agentPanelKey(key string) (Model, tea.Cmd, bool) {
	if m.agentDelete != "" {
		switch key {
		case "y":
			name, harness, dir := m.agentDelete, m.agentHarness(), m.boardDir
			m.agentDelete = ""
			return m, func() tea.Msg { return agentsEditedMsg{agents.Delete(dir, harness, name)} }, true
		case "n", "esc":
			m.agentDelete = ""
		}
		return m, nil, true
	}
	switch key {
	case "tab", "shift+tab":
		m.workflowFocus, m.agentsOpen, m.agentDetail = false, false, false
	case "j", "down":
		m.agentCursor = min(max(0, len(m.agents)-1), m.agentCursor+1)
	case "k", "up":
		m.agentCursor = max(0, m.agentCursor-1)
	case "enter":
		m.agentDetail = !m.agentDetail
	case "a":
		m.taskForm, m.taskInput = "agent-new", ""
	case "t":
		m.agentsOpen, m.workflowFocus, m.agentDetail = false, false, false
	case "esc", "q", "b":
		if m.agentDetail {
			m.agentDetail = false
			return m, nil, true
		}
		m.workflowFocus, m.agentsOpen = false, false
		m.boardMode = false
	case "ctrl+c":
		return m, tea.Quit, true
	case "m", "e", "x":
		a, ok := m.selectedAgent()
		if !ok {
			return m, nil, true
		}
		switch {
		case key == "x" && a.Source == "global", key == "m" && a.Source == "global":
			m.taskNotice = "! " + a.Name + " is global: read-only · copy it to the repo first"
		case key == "m":
			m.taskForm, m.taskInput = "agent-model", a.Model
		case key == "x":
			m.agentDelete = a.Name
		case key == "e":
			if a.Source == "global" {
				m.taskNotice = "! " + a.Name + " is global: read-only"
				return m, nil, true
			}
			edited, err := agents.EnsureFile(m.boardDir, m.agentHarness(), a)
			if err != nil {
				m.taskNotice = "! " + err.Error()
				return m, nil, true
			}
			return m, editFile(m.tmuxPane, edited.Path, func() tea.Msg { return agentsEditedMsg{} }), true
		}
	}
	return m, nil, true
}

// handleAgentForm takes the text for "m" (model) and "a" (new agent name).
func (m Model) handleAgentForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.taskForm = ""
	case tea.KeyBackspace, tea.KeyDelete:
		r := []rune(m.taskInput)
		if len(r) > 0 {
			m.taskInput = string(r[:len(r)-1])
		}
	case tea.KeyRunes:
		m.taskInput += string(key.Runes)
	case tea.KeySpace:
		m.taskInput += " "
	case tea.KeyEnter:
		input, form := strings.TrimSpace(m.taskInput), m.taskForm
		m.taskForm = ""
		if form == "agent-model" {
			a, ok := m.selectedAgent()
			if !ok {
				return m, nil
			}
			a.Model = input
			if err := agents.Save(m.boardDir, m.agentHarness(), a); err != nil {
				m.taskNotice = "! " + err.Error()
				return m, nil
			}
			m = m.loadAgents()
			m.taskNotice = "✓ Model saved · applies to new panes"
			return m, nil
		}
		harness := roles.Harness(m.agentHarness())
		if strings.TrimSpace(input) == "" {
			m.taskNotice = "! agent name required"
			return m, nil
		}
		if _, exists := agents.Find(m.boardDir, m.agentHarness(), input); exists {
			m.taskNotice = "! agent " + input + " already exists"
			return m, nil
		}
		a := agents.Agent{Name: input, Harness: harness, Source: "board"}
		var err error
		if harness == "pi" {
			err = agents.Save(m.boardDir, m.agentHarness(), a)
		} else {
			var created agents.Agent
			created, err = agents.EnsureFile(m.boardDir, m.agentHarness(), a)
			if err == nil {
				m = m.loadAgents()
				return m, editFile(m.tmuxPane, created.Path, func() tea.Msg { return agentsEditedMsg{} })
			}
		}
		if err != nil {
			m.taskNotice = "! " + err.Error()
			return m, nil
		}
		m = m.loadAgents()
		m.taskNotice = "✓ Added " + input
	}
	return m, nil
}

func (m Model) selectedAgentName() string {
	if a, ok := m.selectedAgent(); ok {
		return a.Name
	}
	return ""
}

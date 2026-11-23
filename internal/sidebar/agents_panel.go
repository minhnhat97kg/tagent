package sidebar

import (
	"fmt"
	"strings"

	"tagent/internal/agents"
	"tagent/internal/roles"
	"tagent/internal/tmux"

	tea "github.com/charmbracelet/bubbletea"
)

// The agents panel shares the board's bottom strip with workflows (t toggles it).
// Changes go through agents.Save, so the TUI and web write the same files.

type agentsEditedMsg struct{ err error }

func (m Model) agentHarness() string { h, _ := tmux.GetOption(tmux.AgentOption); return h }

func (m Model) loadAgents() Model {
	m.agents = agents.List(m.boardDir, m.agentHarness(), agents.LivePanes())
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
		if used == "" {
			used = "nothing"
		}
		first, _, _ := strings.Cut(strings.TrimSpace(a.Instructions), "\n")
		lines := []string{
			boardFit(m.boardStyle(m.theme.TextPrimary).Bold(true).Render(a.Name)+"  "+dim.Render(agentSummary(a)), width),
			boardFit(dim.Render("source "+a.Source+" · path "+a.Path), width),
			boardFit(dim.Render("running "+fmt.Sprint(a.Running)+" · used by "+used), width),
			boardFit(dim.Render("instructions: "+first), width),
			boardFit(dim.Render("model changes apply to new panes"), width),
		}
		return lines
	}
	var lines []string
	for i, a := range m.agents {
		mark := "▸ "
		if m.workflowFocus && i == m.agentCursor {
			mark = "❯ "
		}
		run := ""
		if a.Running > 0 {
			run = "  " + m.boardStyle(m.theme.StatusRunning).Render(fmt.Sprintf("● %d running", a.Running))
		}
		lines = append(lines, boardFit(mark+m.boardStyle(m.theme.TextPrimary).Bold(true).Render(a.Name)+"  "+dim.Render(agentSummary(a))+run, width))
	}
	return lines
}

func agentSummary(a agents.Agent) string {
	model := a.Model
	if model == "" {
		model = "default model"
	}
	return a.Harness + " · " + model + " · " + a.Source
}

// stripPanel is the board's bottom strip: workflows, or agents while the agents panel is open.
func (m Model) stripPanel(width int) (string, []string, int) {
	if m.agentsOpen {
		return fmt.Sprintf("Agents %d · m model · e edit · a add · x delete", len(m.agents)), m.agentStripLines(width), m.agentCursor
	}
	return fmt.Sprintf("Workflows %d · a add · e edit", len(m.boardWorkflows)), m.workflowLines(width), m.workflowCursor
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
	case "tab":
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

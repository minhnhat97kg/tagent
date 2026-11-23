package sidebar

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestContextualHelpSections(t *testing.T) {
	cases := []struct {
		name       string
		m          Model
		own, other string
	}{
		{"Agents", Model{}, "next agent needing you", "send / approve / create"},
		{"Agent definitions", Model{dashboard: true, agentFocus: 1}, "edit instruction file", "next agent needing you"},
		{"Tasks", Model{dashboard: true, agentFocus: 2, taskFocus: true}, "start: workflow picker", "select workflow"},
		{"Chat", Model{dashboard: true, agentFocus: 3}, "send / approve / create", "start: workflow picker"},
		{"Board · Tickets", Model{boardMode: true}, "previous / next status", "new workflow name"},
		{"Board · Workflows", Model{boardMode: true, workflowFocus: true}, "new workflow name", "previous / next status"},
		{"Board · Workflow view", Model{boardMode: true, workflowFocus: true, boardFlow: true}, "browse workflows", "delete workflow"},
		{"Board · Task detail", Model{boardMode: true, boardDrawer: true}, "scroll task detail", "browse workflows"},
		{"Intake", Model{boardMode: true, taskForm: "intake", intakeState: &intakeState{}}, "cycle workflow", "next agent needing you"},
		{"Standalone (outside tmux)", Model{standalone: true}, "start a new agent here", "send / approve / create"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.m
			m.width = 140
			m.height = 100
			m.theme = DefaultTheme()
			model, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
			hm := model.(Model)
			if tt.name == "Chat" {
				hm.helpOpen = true
			}
			view := hm.helpView()
			if !hm.helpOpen || hm.helpSection().title != tt.name || !strings.Contains(view, "? "+tt.name) || !strings.Contains(view, tt.own) || strings.Contains(view, tt.other) {
				t.Fatalf("wrong help: %s", view)
			}
			if !strings.Contains(view, "a: all shortcuts") {
				t.Fatal("no full-help footer")
			}
			model, _ = hm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
			hm = model.(Model)
			hm.height = 300
			view = hm.helpView()
			if !hm.helpAll || !strings.Contains(view, "All shortcuts") || !strings.Contains(view, tt.other) {
				t.Fatal("a did not show all")
			}
			model, _ = hm.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
			hm = model.(Model)
			if hm.helpOpen || hm.helpAll {
				t.Fatal("not closed")
			}
			model, _ = hm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
			if model.(Model).helpAll {
				t.Fatal("all view persisted on reopen")
			}
		})
	}
}
func TestHelpPreservesIntakeInput(t *testing.T) {
	m := Model{taskForm: "intake", taskInput: "idea", intakeState: &intakeState{Busy: true}}
	model, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = model.(Model)
	if !m.helpOpen || m.taskInput != "idea" {
		t.Fatal("help did not overlay intake")
	}
	model, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)
	if m.taskForm != "intake" || !m.intakeState.Busy {
		t.Fatal("help destroyed intake")
	}
}

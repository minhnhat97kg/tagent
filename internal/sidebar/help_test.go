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
		{"Agents", Model{}, "next agent needing you", "start: workflow picker"},
		{"Tasks", Model{dashboard: true, focus: cardTasks}, "start: workflow picker", "select workflow"},
		{"Board · Tickets", Model{boardMode: true}, "previous / next status", "new workflow name"},
		{"Board · Workflows", Model{boardMode: true, focus: cardWorkflows}, "new workflow name", "previous / next status"},
		{"Board · Workflow view", Model{boardMode: true, focus: cardWorkflows, boardFlow: true}, "browse workflows", "delete workflow"},
		{"Board · Task detail", Model{boardMode: true, boardDrawer: true}, "scroll task detail", "browse workflows"},
		{"Standalone (outside tmux)", Model{standalone: true}, "start a new agent here", "start: workflow picker"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.m
			m.width = 140
			m.height = 100
			m.theme = DefaultTheme()
			model, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
			hm := model.(Model)
			view := hm.helpView()
			if !hm.helpOpen || hm.helpSection().title != tt.name || !strings.Contains(view, tt.name) || !strings.Contains(view, tt.own) || strings.Contains(view, tt.other) {
				t.Fatalf("wrong help: %s", view)
			}
			if !strings.Contains(view, "all shortcuts") {
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

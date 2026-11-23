package watch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/config"
	"tagent/internal/roles"
	"tagent/internal/sidebar"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func TestBoardAssignAndAdvance(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	task, err := tasks.Add("project", "write tests", []string{"tester"})
	if err != nil {
		t.Fatal(err)
	}
	m := model{scope: "project", focus: focusNotes, tasks: tasks.List("project"), input: textinput.New(), theme: sidebar.DefaultTheme()}
	got := ansi.Strip(strings.Join(m.notesBox(50, 8), "\n"))
	if !strings.Contains(got, "todo") || !strings.Contains(got, "@tester") || !strings.Contains(got, "write tests") {
		t.Fatalf("board = %q", got)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	unchanged, err := tasks.Get("project", task.ID)
	if err != nil || unchanged.Status != "todo" {
		t.Fatalf("enter changed task status: %+v %v", unchanged, err)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	updated, err := tasks.Get("project", task.ID)
	if err != nil || updated.Status != "blocked" {
		t.Fatalf("status: %+v %v", updated, err)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if len(tasks.List("project")) != 0 {
		t.Fatal("task was not removed")
	}
}

func TestBoardClickSelectAndDelete(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	first, _ := tasks.Add("project", "first", []string{"a"})
	second, _ := tasks.Add("project", "second", []string{"b"})
	m := model{scope: "project", focus: focusNotes, tasks: tasks.List("project"), input: textinput.New(), theme: sidebar.DefaultTheme(), width: 120, height: 30}
	next, _ := m.Update(tea.MouseMsg{X: 5, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(model)
	if m.tasks[m.cursor[focusNotes]].ID != second.ID {
		t.Fatal("click did not select second task")
	}
	detail := ansi.Strip(strings.Join(m.noteDetailBox(60, 12), "\n"))
	if !strings.Contains(detail, "2/2") || !strings.Contains(detail, "second") {
		t.Fatalf("click did not show selected task detail: %s", detail)
	}
	next, _ = m.Update(tea.MouseMsg{X: 62, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	_ = next
	if _, err := tasks.Get("project", second.ID); err == nil {
		t.Fatal("delete button did not remove selected task")
	}
	if _, err := tasks.Get("project", first.ID); err != nil {
		t.Fatal("delete removed wrong task")
	}
}

func TestBoardDeepLinkSelectsTask(t *testing.T) {
	m := model{focus: focusNotes, boardTaskID: "second", theme: sidebar.DefaultTheme()}
	next, _ := m.Update(snapshot{scope: "repo", tasks: []tasks.Task{{ID: "first", Text: "First"}, {ID: "second", Text: "Second"}}})
	m = next.(model)
	if m.cursor[focusNotes] != 1 || m.boardTaskID != "" {
		t.Fatalf("deep link selected cursor %d, pending %q", m.cursor[focusNotes], m.boardTaskID)
	}
	if detail := ansi.Strip(strings.Join(m.noteDetailBox(50, 12), "\n")); !strings.Contains(detail, "Second") {
		t.Fatalf("detail: %s", detail)
	}
}

func TestBoardClickSelectNarrowLayout(t *testing.T) {
	m := model{focus: focusNotes, tasks: []tasks.Task{{ID: "first", Text: "First"}, {ID: "second", Text: "Second"}}, width: 75, height: 30, theme: sidebar.DefaultTheme()}
	next, _ := m.Update(tea.MouseMsg{X: 5, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(model)
	if m.cursor[focusNotes] != 1 {
		t.Fatalf("narrow click selected %d", m.cursor[focusNotes])
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Second") || !strings.Contains(view, "2/2") {
		t.Fatalf("selected detail missing: %s", view)
	}
}

func row(id, name string, st tmux.PaneStatus, unseen bool, doing string) agentRow {
	tool, text, _ := strings.Cut(doing, " ")
	return agentRow{PaneInfo: tmux.PaneInfo{PaneID: id, Name: name, Status: st, Unseen: unseen}, Tool: tool, Doing: text}
}

func TestDiffEmitsEvents(t *testing.T) {
	m := model{last: map[string]paneState{}}
	m.diff([]agentRow{row("%1", "rev", tmux.StatusIdle, false, "")})
	if len(m.events) != 0 {
		t.Fatalf("first poll must only prime, got %+v", m.events)
	}
	m.diff([]agentRow{row("%1", "rev", tmux.StatusRunning, false, "Read a.go"), row("%2", "", tmux.StatusIdle, false, "")})
	m.diff([]agentRow{row("%1", "rev", tmux.StatusRunning, false, "Edit a.go"), row("%2", "", tmux.StatusIdle, false, "")})
	m.diff([]agentRow{row("%1", "rev", tmux.StatusIdle, true, "")})

	var got []string
	for _, e := range m.events {
		got = append(got, e.who+": "+e.text)
	}
	want := "rev: idle → running|%2: appeared (, idle)|rev: Edit a.go|rev: running → done|%2: gone"
	if strings.Join(got, "|") != want {
		t.Errorf("events =\n%s\nwant\n%s", strings.Join(got, "|"), want)
	}
}

func TestRolesPanel(t *testing.T) {
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, ".claude", "agents"), 0755)
	os.WriteFile(filepath.Join(repo, ".claude", "agents", "reviewer.md"), []byte("---\nname: reviewer\n---\nReview changes"), 0644)
	os.WriteFile(filepath.Join(repo, ".claude", "agents", "tester.md"), []byte("---\nname: tester\n---\nRun tests"), 0644)
	m := model{repoDir: repo, harness: "claude", repoAgents: roles.ListLocal(repo), theme: sidebar.DefaultTheme(), icons: sidebar.DefaultIcons(), width: 120, height: 30, last: map[string]paneState{}}
	left := ansi.Strip(strings.Join(m.agentsBox(42, 20), "\n"))
	if !strings.Contains(left, "Repo agents · claude") || !strings.Contains(left, "reviewer") || !strings.Contains(left, "tester") {
		t.Fatalf("agents left pane lacks native definitions: %s", left)
	}
	nm, _ := m.updateKeys(tea.KeyMsg{Type: tea.KeyTab})
	nm, _ = nm.(model).updateKeys(tea.KeyMsg{Type: tea.KeyTab}) // agents → board → roles
	m = nm.(model)
	if m.focus != focusRoles || len(m.roles) == 0 {
		t.Fatalf("focus=%d roles=%v", m.focus, m.roles)
	}
	nm, _ = m.updateKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = nm.(model)
	out := ansi.Strip(m.View())
	sel := m.roles[1]
	for _, want := range []string{"Repo agents", "Role · " + sel, "claude", "start an agent", "edit in $EDITOR"} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q:\n%s", want, out)
		}
	}
}

// moveSetting puts the Settings cursor on setting idx.
func moveSetting(m model, idx int) model {
	for m.editor.Cursor() < idx {
		nm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = nm.(model)
	}
	for m.editor.Cursor() > idx {
		nm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = nm.(model)
	}
	return m
}

func settingIndex(key string) int {
	for i, s := range config.Settings {
		if s.Key == key {
			return i
		}
	}
	return -1
}

func TestSettingsTab(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	m := model{theme: sidebar.DefaultTheme(), icons: sidebar.DefaultIcons(), width: 120, height: 100, last: map[string]paneState{}}

	for i := 0; i < 3; i++ { // agents → board → roles → settings
		nm, _ := m.updateKeys(tea.KeyMsg{Type: tea.KeyTab})
		m = nm.(model)
	}
	if m.focus != focusSettings || m.editor == nil {
		t.Fatalf("focus=%d editor=%v", m.focus, m.editor)
	}

	body, _ := m.editor.Body(m.theme, 40, false)
	out := ansi.Strip(strings.Join(body, "\n"))
	for _, s := range config.Settings {
		if !strings.Contains(out, s.Label) {
			t.Errorf("settings list lacks %q", s.Label)
		}
	}
	if strings.Contains(out, "● on") || strings.Contains(out, "○ off") || strings.Contains(out, "‹ ") {
		t.Fatalf("settings list must show labels without values:\n%s", out)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "4 Settings") || !strings.Contains(view, "Setting · ") {
		t.Errorf("view lacks the Settings tab or its detail:\n%s", view)
	}

	// enter on a Bool toggles it and persists through config.Set.
	idx := settingIndex("@sidebar_auto_create")
	s := config.Settings[idx]
	fake.Set("", "set", "-g", s.Key, "off")
	m = moveSetting(m, idx)
	nm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if got := m.editor.Value(s.Key); got != "off" {
		t.Errorf("%s = %q after enter, want off", s.Key, got)
	}
	if flash, _ := m.editor.Flash(); !strings.Contains(flash, "saved") {
		t.Errorf("flash = %q, want a saved line", flash)
	}

	// enter on a Text setting edits inline; keys stay in the input; esc
	// leaves editing without leaving the dashboard.
	idx = settingIndex("@agent_model")
	m = moveSetting(m, idx)
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if !m.editor.Editing() || m.focus != focusSettings {
		t.Fatalf("editing=%v focus=%d after enter on a Text setting", m.editor.Editing(), m.focus)
	}
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("unique-value-123")})
	m = nm.(model)
	if m.focus != focusSettings || !m.editor.Editing() {
		t.Errorf("typing a digit left the input (focus=%d editing=%v)", m.focus, m.editor.Editing())
	}
	detail := ansi.Strip(strings.Join(m.settingsDetailBox(80, 20), "\n"))
	if !strings.Contains(detail, "unique-value-123") {
		t.Fatalf("detail must show the value being edited: %s", detail)
	}
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.editor.Editing() || m.focus != focusSettings {
		t.Errorf("esc left editing=%v focus=%d, want editing=false on settings", m.editor.Editing(), m.focus)
	}

	// , jumps to the Settings tab from anywhere.
	m = model{theme: sidebar.DefaultTheme(), icons: sidebar.DefaultIcons(), width: 120, height: 30, last: map[string]paneState{}}
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")})
	m = nm.(model)
	if m.focus != focusSettings || m.editor == nil {
		t.Errorf(", → focus=%d editor=%v", m.focus, m.editor)
	}
}

func TestMarkdownStripsMarkers(t *testing.T) {
	m := model{theme: sidebar.DefaultTheme()}
	got := ansi.Strip(strings.Join(m.markdown("You are **qa**.\n- run `go test ./...` then report", 80), "\n"))
	if got != "You are qa.\n• run go test ./... then report" {
		t.Errorf("got %q", got)
	}
}

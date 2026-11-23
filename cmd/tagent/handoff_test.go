package main

import (
	"bytes"
	"strings"
	"tagent/internal/tmux"
	"testing"
)

func TestCmdHandoffClearsOwner(t *testing.T) {
	fake := tmux.NewFake()
	restore := tmux.SetRunner(fake)
	defer restore()
	fake.Set("", "set-option", "-p", "-t", "%42", "@agent_size_owner", "")
	fake.Set("", "resize-window", "-A", "-t", "%42")
	t.Setenv("TMUX_PANE", "%42")
	var errs bytes.Buffer
	if code := cmdHandoff(nil, &errs); code != exitOK {
		t.Fatalf("code %d: %s", code, errs.String())
	}
	calls := []string{}
	for _, c := range fake.Calls() {
		calls = append(calls, strings.Join(c, " "))
	}
	if !strings.Contains(strings.Join(calls, "\n"), "set-option -p -t %42 @agent_size_owner ") {
		t.Fatal("owner not cleared")
	}
	if !strings.Contains(strings.Join(calls, "\n"), "resize-window -A -t %42") {
		t.Fatal("window not resized")
	}
}
func TestCmdHandoffRequiresPane(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	if cmdHandoff(nil, &bytes.Buffer{}) != exitUsage {
		t.Fatal("missing pane accepted")
	}
}

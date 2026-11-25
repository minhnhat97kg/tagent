package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"tagent/internal/board"
	"tagent/internal/roles"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workplace"
)

// A `workplace: folder` ticket runs in its own directory outside the repo: no
// worktree, no branch, one reused pane. Done attaches the files and removes it.
func TestWorkplaceFolderRunsOutsideRepo(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux unavailable")
	}
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@test")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@test")
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	for _, args := range [][]string{{"init", "-q"}, {"commit", "--allow-empty", "-qm", "baseline"}} {
		if _, err := git(dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	fake := filepath.Join(bin, "claude")
	os.WriteFile(fake, []byte("#!/bin/sh\ntmux set-option -p -t \"$TMUX_PANE\" @pane_agent claude\ntmux set-option -p -t \"$TMUX_PANE\" @pane_status idle\nwhile IFS= read -r line; do :; done\n"), 0700)
	if b, err := exec.Command("tmux", "-f", "/dev/null", "new-session", "-d", "-c", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(b))
	}
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	tmux.RunTmux("set-option", "-g", "@agent_scrape", "off")
	os.MkdirAll(roles.GlobalDir(), 0700)
	os.WriteFile(filepath.Join(roles.GlobalDir(), "planner.md"), []byte("Plan carefully."), 0600)
	os.WriteFile(filepath.Join(roles.GlobalDir(), "profiles.toml"), []byte("[planner]\nharness = '"+fake+"'\n"), 0600)

	wf := filepath.Join(dir, ".tasks", "workflows")
	os.MkdirAll(wf, 0700)
	os.WriteFile(filepath.Join(wf, "notes.yml"), []byte("name: notes\nworkplace: folder\njobs:\n  write:\n    agent: planner\n    prompt: Write\n"), 0600)

	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "notes question", nil, tasks.MutationOptions{Workflow: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.Move(scope, ticket.ID, "write", ""); err != nil {
		t.Fatal(err)
	}
	wp, err := tasks.TicketWorkplace(scope, ticket)
	if f, ok := wp.(workplace.Folder); !ok || err != nil || f.Keep {
		t.Fatalf("folder workflow: %#v %v", wp, err)
	}
	folder := wp.Output(dir, ticket.ID)

	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0700)
	a := board.Action{Dir: sub, Agent: "planner", Prompt: "first prompt"}
	a.Event.Task = ticket.ID
	for range 2 {
		if err := dispatchBoardAction(a); err != nil {
			t.Fatal(err)
		}
	}
	name := "planner-" + ticket.ID[max(0, len(ticket.ID)-4):]
	pane := tmux.ResolvePane(name)
	if pane == name || tmux.ResolvePane(name+"-2") != name+"-2" {
		t.Fatal("pane not reused")
	}
	found := false
	for _, s := range tmux.QuerySessions() {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.PaneID == pane {
					found = true
					if p.Path != folder {
						t.Fatalf("pane cwd %s, want %s", p.Path, folder)
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("pane not listed")
	}
	if _, err := os.Stat(filepath.Join(dir, workplace.Dir)); !os.IsNotExist(err) {
		t.Fatalf(".worktrees: %v", err)
	}
	if out, err := git(dir, "branch", "--list", "agent/workflow-*"); err != nil || out != "" {
		t.Fatalf("workflow branches: %q %v", out, err)
	}
	if got := tasks.Scope(folder); got != scope {
		t.Fatalf("folder scope %q, want %q", got, scope)
	}

	os.WriteFile(filepath.Join(folder, "note.md"), []byte("note"), 0600)
	if err := tasks.Move(scope, ticket.ID, "done", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("folder kept after done: %v", err)
	}
	list, err := tasks.AttachmentList(scope, dir, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range list {
		if a.Source == "comment" && a.Name == "note.md" && a.Error == "" {
			return
		}
	}
	t.Fatalf("note.md not attached: %+v", list)
}

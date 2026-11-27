package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/tasks"
	"tagent/internal/workplace"
)

func TestRemoteDockerLibraryNotAppliedWarns(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".tasks"), 0o700)
	os.WriteFile(filepath.Join(dir, ".tasks", "board.yaml"), []byte("roles:\n  dev: {agent: claude, skills: [demo], mcp: [web]}\n  plain: {agent: claude}\n"), 0o600)
	scope := tasks.Scope(dir)
	notices := func(o spawnOpts, role string) []string {
		t.Helper()
		ticket, err := tasks.Add(scope, "t", nil)
		if err != nil {
			t.Fatal(err)
		}
		o.ConfigDir, o.Ticket = dir, ticket.ID
		if err := noteUnappliedLibrary(o, role); err != nil {
			t.Fatal(err)
		}
		got, err := tasks.Get(scope, ticket.ID)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, c := range got.Comments {
			if strings.HasPrefix(c.Body, "library:") {
				out = append(out, c.Body)
			}
		}
		return out
	}
	remote := spawnOpts{Remote: workplace.Remote{Host: "h", Dir: "/d"}}
	for place, o := range map[string]spawnOpts{"remote": remote, "docker": {Container: "c"}} {
		want := "library: skills/mcp not applied on " + place + ": demo, web"
		if got := notices(o, "dev"); len(got) != 1 || got[0] != want {
			t.Errorf("%s: %q, want %q", place, got, want)
		}
	}
	if got := notices(spawnOpts{}, "dev"); len(got) != 0 {
		t.Errorf("local launch commented %q", got)
	}
	if got := notices(remote, "plain"); len(got) != 0 {
		t.Errorf("role without a selection commented %q", got)
	}
	remote.ConfigDir, remote.Ticket = dir, "missing"
	if err := noteUnappliedLibrary(remote, "dev"); err == nil {
		t.Error("a comment that cannot be written must fail the launch")
	}
}

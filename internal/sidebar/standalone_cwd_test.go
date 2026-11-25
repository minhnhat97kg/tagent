package sidebar

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

// Standalone launched from a repo (or a subdir of it), with a pane in $HOME at
// row 0 (the first pane of any tmux session). The board scope and notes must
// be the repo root, not the cursor pane's dir.
func TestStandaloneBoardScopeUsesLaunchCwd(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home, repo := t.TempDir(), t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	sub := filepath.Join(repo, "internal", "x")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	for name, cwd := range map[string]string{"repo root": repo, "subdir": sub} {
		t.Run(name, func(t *testing.T) {
			t.Chdir(cwd)
			m := newStandaloneModel()
			m.sessions = []tmux.SessionInfo{{SessionName: "home", Windows: []tmux.WindowInfo{{
				Panes: []tmux.PaneInfo{{PaneID: "%1", Path: home}},
			}}}}
			m.rows = []Row{{PaneID: "%1"}}
			m.cursor = 0
			m.repoDir = home // what detailMsg.Dir does when row 0 is selected
			m.boardDir = ""
			m.notesScope = tasks.Scope(home)

			m = m.boardContext()

			want, _ := filepath.EvalSymlinks(repo)
			got, _ := filepath.EvalSymlinks(m.boardDir)
			if got != want {
				t.Fatalf("boardDir = %q, want repo root %q (home pane %q)", got, want, home)
			}
			if m.notesScope != tasks.Scope(repo) {
				t.Fatalf("notesScope = %q, want %q", m.notesScope, tasks.Scope(repo))
			}
			if dir := m.newAgentCwd(); dir != m.launchDir {
				t.Fatalf("newAgentCwd = %q, want launch dir %q", dir, m.launchDir)
			}
			if dir := m.focusDir("%1"); dir != m.launchDir {
				t.Fatalf("focusDir = %q, want launch dir %q", dir, m.launchDir)
			}
		})
	}
}

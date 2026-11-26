package board

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"tagent/internal/sharedctx"
	"tagent/internal/tasks"
)

func Register(dir string) error {
	dir = tasks.MainRoot(dir)
	base := tasks.BoardDir(tasks.Scope(dir))
	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	return tasks.AtomicWrite(filepath.Join(base, "repo"), []byte(dir))
}

func Registered() []string {
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(sharedctx.Dir("global")), "*", "tasks", "repo"))
	var dirs []string
	for _, f := range files {
		if b, e := os.ReadFile(f); e == nil {
			if st, e := os.Stat(string(b)); e == nil && st.IsDir() {
				dirs = append(dirs, string(b))
			}
		}
	}
	return dirs
}

// hidden marks a workspace the user removed. Only Workspaces skips it; dispatch
// and Registered do not, so tickets in flight keep moving.
const hidden = "hidden"

func boardBase(root string) string { return tasks.BoardDir(tasks.Scope(root)) }

// AddWorkspace registers dir's main repo (a worktree lists its main repo) and
// unhides it. dir must be an absolute, existing folder; a leading ~ is home.
func AddWorkspace(dir string) (string, error) {
	dir, err := checkDir(dir)
	if err != nil {
		return "", err
	}
	if err := Register(dir); err != nil {
		return "", err
	}
	root := tasks.MainRoot(dir)
	os.Remove(filepath.Join(boardBase(root), hidden))
	return root, nil
}

// RemoveWorkspace hides dir's main repo from Workspaces. Ticket files stay.
func RemoveWorkspace(dir string) error {
	dir, err := checkDir(dir)
	if err != nil {
		return err
	}
	base := boardBase(tasks.MainRoot(dir))
	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(base, hidden), nil, 0600)
}

// Workspaces are the repos to show: first, then the registered ones, mapped to
// their main repo, deduplicated, without the hidden ones.
func Workspaces(first ...string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, d := range slices.Concat(first, Registered()) {
		if d == "" {
			continue
		}
		root := tasks.MainRoot(d)
		if seen[root] {
			continue
		}
		seen[root] = true
		if _, err := os.Stat(filepath.Join(boardBase(root), hidden)); err == nil {
			continue
		}
		out = append(out, root)
	}
	return out
}

func checkDir(dir string) (string, error) {
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, dir[1:])
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("%q: path must be absolute", dir)
	}
	dir = filepath.Clean(dir)
	st, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%s is not a folder", dir)
	}
	return dir, nil
}

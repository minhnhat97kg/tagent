// Package workplace owns where a ticket's or an agent's work lives on disk: a
// git worktree under Dir, the main root for `workplace: none`, or a plain folder
// for `workplace: folder`. It is a leaf (stdlib only) so tasks, cmd and worktree
// can all use it.
package workplace

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Dir is the repo-relative directory holding worktrees.
const Dir = ".worktrees"

// Workplace is where a ticket's work is kept, merged and removed.
type Workplace interface {
	Create(root, id string) (string, error)
	Output(root, id string) string
	Gate(root, id, title string) (note string, err error)
	Cleanup(root, id string) []string
}

// Git keeps each ticket in a git worktree on branch agent/workflow-<id>. Keep
// (`cleanup: keep`) leaves the worktree and branch in place after done.
type Git struct{ Keep bool }

var _ Workplace = Git{}

// None runs the ticket in the main root: no worktree, no branch, nothing to merge.
type None struct{}

var _ Workplace = None{}

func (None) Create(root, id string) (string, error)      { return root, nil }
func (None) Output(root, id string) string               { return root }
func (None) Gate(root, id, title string) (string, error) { return "", nil }
func (None) Cleanup(root, id string) []string            { return nil }

// checkID rejects ids that cannot name a directory or a branch.
func checkID(id string) error {
	if id == "" || strings.ContainsAny(id, "/\\. \t\n") {
		return fmt.Errorf("invalid workflow ticket ID %q", id)
	}
	return nil
}

// Create keeps all jobs for a ticket in one retained checkout and returns its path.
func (Git) Create(root, id string) (string, error) {
	if err := checkID(id); err != nil {
		return "", err
	}
	_, _, path, err := Add(root, "workflow-"+id)
	return path, err
}

// Output is the ticket's checkout path, whether or not it exists.
func (Git) Output(root, id string) string {
	return filepath.Join(root, Dir, "workflow-"+id)
}

// Gate commits the ticket checkout's work on agent/workflow-<id>, rebases it onto
// the main checkout's HEAD if needed, and fast-forwards the main checkout to it.
// Nothing is forced or reset; any refusal is returned and leaves both checkouts
// as they were. A missing checkout has nothing to merge. note is the merge
// summary ("merged <sha> into <branch>"), or "" when there was nothing to merge.
func (g Git) Gate(root, id, title string) (note string, err error) {
	wt := g.Output(root, id)
	if _, statErr := os.Stat(wt); statErr != nil {
		return "", nil
	}
	branch := "agent/workflow-" + id
	if !Usable(wt) { // git would resolve a bare dir to the main checkout and commit there
		if why := mergeBlocker(root, branch); why != "" {
			return "", fmt.Errorf("%s is not a usable git worktree and its branch is %s", wt, why)
		}
		return "", nil // nothing to merge; Cleanup removes the directory
	}
	dirty, err := Run(wt, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if dirty != "" {
		if _, err := Run(wt, "add", "-A"); err != nil {
			return "", err
		}
		if _, err := Run(wt, "commit", "-qm", title+" (#"+id+")"); err != nil {
			return "", err
		}
	}
	head, err := Run(root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	ahead, err := Run(root, "rev-list", "--count", head+".."+branch)
	if err != nil {
		return "", err
	}
	if ahead == "0" {
		return "", nil
	}
	if _, err := Run(root, "merge-base", "--is-ancestor", head, branch); err != nil {
		if _, err := Run(wt, "rebase", head); err != nil {
			Run(wt, "rebase", "--abort")
			return "", err
		}
	}
	if _, err := Run(root, "merge", "--ff-only", branch); err != nil {
		return "", err
	}
	short, _ := Run(root, "rev-parse", "--short", "HEAD")
	cur, _ := Run(root, "rev-parse", "--abbrev-ref", "HEAD")
	return "merged " + short + " into " + cur, nil
}

// Cleanup removes the ticket checkout when checkoutBlocker allows it, and its
// agent/workflow-<id> branch only if merged into the main checkout's HEAD.
// It returns one note per thing it left in place.
func (g Git) Cleanup(root, id string) (notes []string) {
	if g.Keep {
		return nil
	}
	name := "workflow-" + id
	path := filepath.Join(root, Dir, name)
	branch := "agent/" + name
	if _, err := os.Stat(path); err == nil {
		if why := checkoutBlocker(root, path, branch); why != "" {
			return []string{"cleanup: kept worktree " + path + " (" + why + ")"}
		}
		if err := removeCheckout(root, path); err != nil {
			return []string{"cleanup: kept worktree " + path + " (" + err.Error() + ")"}
		}
	}
	if why := mergeBlocker(root, branch); why != "" {
		return append(notes, "cleanup: kept branch "+branch+" ("+why+")")
	}
	if _, err := Run(root, "rev-parse", "--verify", "-q", "refs/heads/"+branch); err != nil {
		return nil
	}
	if _, err := Run(root, "branch", "-d", branch); err != nil {
		notes = append(notes, "cleanup: kept branch "+branch+" ("+err.Error()+")")
	}
	return notes
}

// checkoutBlocker returns why the checkout at path must stay, or "" when removing
// it loses nothing. A checkout that is not a usable git worktree (a removal that
// stopped partway) is kept while its branch is unmerged, since git cannot tell
// what else is in it. A usable one must be on branch, with no untracked or
// modified files. Deleted tracked files are allowed only once the branch is
// merged, since their content is then in HEAD. A detached HEAD is kept: its
// commits would be unreachable once the worktree is removed.
func checkoutBlocker(root, path, branch string) string {
	if !Usable(path) {
		return mergeBlocker(root, branch)
	}
	if head, err := Run(path, "symbolic-ref", "-q", "HEAD"); err != nil || head != "refs/heads/"+branch {
		return "HEAD is not " + branch
	}
	untracked, err := Run(path, "ls-files", "--others", "--exclude-standard")
	if err != nil || untracked != "" {
		return "uncommitted changes"
	}
	changes, err := Run(path, "diff", "--name-status", "HEAD")
	if err != nil {
		return "uncommitted changes"
	}
	deleted := false
	for _, line := range strings.Split(changes, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "D\t") {
			return "uncommitted changes"
		}
		deleted = true
	}
	if deleted && mergeBlocker(root, branch) != "" {
		return "uncommitted changes"
	}
	return ""
}

// mergeBlocker returns "not merged" when branch exists and is not merged into the
// main checkout's HEAD, else "" (no branch counts as merged).
func mergeBlocker(root, branch string) string {
	if _, err := Run(root, "rev-parse", "--verify", "-q", "refs/heads/"+branch); err != nil {
		return ""
	}
	if _, err := Run(root, "merge-base", "--is-ancestor", branch, "HEAD"); err != nil {
		return "not merged"
	}
	return ""
}

// Usable reports whether path is a git checkout of its own. It needs its own .git:
// a bare directory would resolve to the enclosing repository.
func Usable(path string) bool {
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		return false
	}
	_, err := Run(path, "rev-parse", "--git-dir")
	return err == nil
}

// removeCheckout deletes the checkout and its admin entry. A usable worktree goes
// through git (the blocker already ruled out unsaved work, so the force only drops
// deleted files and ignored output). A half-removed directory is deleted directly,
// since git no longer recognises it, then pruned from the admin list.
func removeCheckout(root, path string) error {
	if Usable(path) {
		_, err := Run(root, "worktree", "remove", "--force", path)
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	_, err := Run(root, "worktree", "prune")
	return err
}

// PruneBlocker returns why ticket id's work must stay, or "" when it can go: its
// checkout passes checkoutBlocker, and its branch (if any) is merged.
func (Git) PruneBlocker(root, id string) string {
	name := "workflow-" + id
	path := filepath.Join(root, Dir, name)
	branch := "agent/" + name
	if _, err := os.Stat(path); err == nil {
		if why := checkoutBlocker(root, path, branch); why != "" {
			return why
		}
	}
	return mergeBlocker(root, branch)
}

// Tickets lists the ids that have a checkout directory or an agent/workflow-<id>
// branch, sorted, so a worktree without a branch and a branch without a worktree
// are both seen.
func Tickets(root string) []string {
	set := map[string]bool{}
	dirs, _ := filepath.Glob(filepath.Join(root, Dir, "workflow-*"))
	for _, d := range dirs {
		set[strings.TrimPrefix(filepath.Base(d), "workflow-")] = true
	}
	refs, _ := Run(root, "for-each-ref", "--format=%(refname:short)", "refs/heads/agent/workflow-*")
	for _, r := range strings.Fields(refs) {
		set[strings.TrimPrefix(r, "agent/workflow-")] = true
	}
	return slices.Sorted(maps.Keys(set))
}

// Paths returns the repo root of dir, and the branch and path an agent's
// worktree uses.
func Paths(dir, name string) (repo, branch, path string, err error) {
	if repo, err = Run(dir, "rev-parse", "--show-toplevel"); err != nil {
		return "", "", "", fmt.Errorf("%s is not in a git repo", dir)
	}
	return repo, "agent/" + name, filepath.Join(repo, Dir, name), nil
}

// Add creates (or reuses) Dir/<name> on branch agent/<name>, keeping Dir out of
// git status.
func Add(dir, name string) (repo, branch, path string, err error) {
	if repo, branch, path, err = Paths(dir, name); err != nil {
		return
	}
	validate := func() error {
		listing, e := Run(repo, "worktree", "list", "--porcelain")
		if e != nil {
			return e
		}
		root, e := Run(path, "rev-parse", "--show-toplevel")
		if e != nil {
			return e
		}
		common, e := Run(path, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if e != nil {
			return e
		}
		expected, e := Run(repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if e != nil {
			return e
		}
		common, e = filepath.EvalSymlinks(common)
		if e != nil {
			return e
		}
		expected, e = filepath.EvalSymlinks(expected)
		if e != nil {
			return e
		}
		if common != expected {
			return fmt.Errorf("foreign repository at %s", path)
		}
		if _, e = Run(path, "rev-parse", "--verify", "HEAD^{commit}"); e != nil {
			return e
		}
		current, e := Run(path, "symbolic-ref", "HEAD")
		if e != nil {
			return e
		}
		if root != path || current != "refs/heads/"+branch || !strings.Contains(listing, "worktree "+path+"\n") {
			return fmt.Errorf("invalid worktree at %s", path)
		}
		return nil
	}
	if _, statErr := os.Lstat(path); statErr == nil {
		err = validate()
		return
	} else if !os.IsNotExist(statErr) {
		err = statErr
		return
	}
	if _, err = Run(repo, "worktree", "add", "-q", "-b", branch, path); err != nil {
		if validate() == nil {
			err = nil
		}
		return
	}
	excl, e := Run(repo, "rev-parse", "--git-path", "info/exclude")
	if e != nil {
		err = e
		return
	}
	if !filepath.IsAbs(excl) {
		excl = filepath.Join(repo, excl)
	}
	if b, _ := os.ReadFile(excl); !strings.Contains(string(b), Dir+"/") {
		if f, ferr := os.OpenFile(excl, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); ferr == nil {
			fmt.Fprintln(f, Dir+"/")
			f.Close()
		}
	}
	return
}

// Run runs git in dir and returns its trimmed output. On failure it returns ""
// and an error carrying git's output.
func Run(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

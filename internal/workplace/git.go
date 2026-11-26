// Package workplace owns where a ticket's or an agent's work lives on disk: a
// git worktree under Dir, the main root for `workplace: none`, a plain folder
// for `workplace: folder`, or a Chrome profile and captures for `workplace:
// browser`. It is a leaf (stdlib only) so tasks, cmd and worktree can all use it.
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

// webDist is the build output generated from web/src by `make web`. Rebases may
// conflict on it, and Gate rebuilds it instead of blocking.
const webDist = "internal/web/dist"

// Workplace is where a ticket's work is kept, merged and removed.
type Workplace interface {
	Create(root, id string) (string, error)
	Output(root, id string) string
	Gate(root, id, title string, checks func() ([]string, error)) (note string, err error)
	Cleanup(root, id string) []string
}

// Git keeps each ticket in a git worktree on branch agent/workflow-<id>. Keep
// (`cleanup: keep`) leaves the worktree and branch in place after done.
type Git struct{ Keep bool }

var _ Workplace = Git{}

// None runs the ticket in the main root: no worktree, no branch, nothing to merge.
type None struct{}

var _ Workplace = None{}

func (None) Create(root, id string) (string, error) { return root, nil }
func (None) Output(root, id string) string          { return root }
func (None) Gate(root, id, title string, checks func() ([]string, error)) (string, error) {
	return "", nil
}
func (None) Cleanup(root, id string) []string { return nil }

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
// A rebase conflicting only on webDist is resolved, and after a rebase webDist is
// rebuilt and committed when the repo tracks it. After a rebase, checks (called only
// then) gives the check commands, each run in the rebased checkout in order; a
// failing one refuses the merge. Nothing is forced
// or reset; any refusal is returned and leaves both checkouts as they were. A
// missing checkout has nothing to merge. note is the merge summary ("merged <sha>
// into <branch>"), or "" when there was nothing to merge.
func (g Git) Gate(root, id, title string, checks func() ([]string, error)) (note string, err error) {
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
		var list []string
		if checks != nil {
			var cerr error
			if list, cerr = checks(); cerr != nil {
				return "", cerr
			}
		}
		if err := rebaseOnto(wt, head); err != nil {
			Run(wt, "rebase", "--abort")
			return "", err
		}
		if err := rebuildDist(wt); err != nil {
			return "", err
		}
		for _, c := range list {
			if err := runCheck(wt, id, c); err != nil {
				return "", err
			}
		}
	}
	if _, err := Run(root, "merge", "--ff-only", branch); err != nil {
		return "", err
	}
	short, _ := Run(root, "rev-parse", "--short", "HEAD")
	cur, _ := Run(root, "rev-parse", "--abbrev-ref", "HEAD")
	return "merged " + short + " into " + cur, nil
}

// rebaseOnto rebases wt onto head. A rebase that stops only on conflicts under
// webDist takes the branch's side of those paths and continues; any other conflict
// is returned, and the caller aborts.
func rebaseOnto(wt, head string) error {
	_, err := Run(wt, "rebase", head)
	for err != nil {
		paths, perr := Run(wt, "diff", "--name-only", "--diff-filter=U")
		if perr != nil || paths == "" {
			return err
		}
		list := strings.Split(paths, "\n")
		for _, p := range list {
			if !strings.HasPrefix(p, webDist+"/") {
				return err
			}
		}
		if _, cerr := Run(wt, append([]string{"checkout", "--ours", "--"}, list...)...); cerr != nil {
			return err
		}
		if _, aerr := Run(wt, append([]string{"add", "--"}, list...)...); aerr != nil {
			return err
		}
		_, err = Run(wt, "-c", "core.editor=true", "rebase", "--continue")
		if err != nil {
			// A commit that only rebuilt dist can become empty; skip it when nothing is staged.
			if _, serr := Run(wt, "diff", "--cached", "--quiet"); serr == nil {
				_, err = Run(wt, "rebase", "--skip")
			}
		}
	}
	return nil
}

// rebuildDist regenerates webDist with `make web` in wt, when the repo tracks it,
// and commits the result if it changed. A failed build is returned, which blocks
// the merge, so the merged dist always matches a fresh build of web/src.
func rebuildDist(wt string) error {
	tracked, err := Run(wt, "ls-files", "--", webDist)
	if err != nil || tracked == "" {
		return err
	}
	if out, err := exec.Command("make", "-C", wt, "web").CombinedOutput(); err != nil {
		return fmt.Errorf("make web: %s", strings.TrimSpace(string(out)))
	}
	if _, err := Run(wt, "add", "-A", "--", webDist); err != nil {
		return err
	}
	if _, err := Run(wt, "diff", "--cached", "--quiet"); err == nil {
		return nil // dist already matches
	}
	_, err = Run(wt, "commit", "-qm", "rebuild web dist")
	return err
}

// runCheck runs one check command with sh -c in dir, as a workflow run job does,
// and returns an error carrying the tail of its output when it fails.
func runCheck(dir, id, command string) error {
	output, err := os.CreateTemp("", "tagent-output-*")
	if err != nil {
		return err
	}
	output.Close()
	defer os.Remove(output.Name()) // outputs are not recorded here; the file only has to exist
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TMUX_AGENT_TICKET="+id, "TMUX_AGENT_OUTPUT="+output.Name())
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	log := strings.TrimSpace(string(out))
	if len(log) > 2000 {
		log = log[len(log)-2000:]
	}
	return fmt.Errorf("check %q failed (%v):\n%s", command, err, log)
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

// Discard removes ticket id's checkout and its branch whatever they hold. Purge
// uses it; Cleanup keeps dirty or unmerged work instead.
func (Git) Discard(root, id string) error {
	if err := checkID(id); err != nil {
		return err
	}
	if _, err := Run(root, "rev-parse", "--git-dir"); err != nil { // not a repo: no checkout or branch exists
		return nil
	}
	name := "workflow-" + id
	path := filepath.Join(root, Dir, name)
	branch := "agent/" + name
	if _, err := os.Stat(path); err == nil {
		if err := removeCheckout(root, path); err != nil {
			return err
		}
	}
	// A worktree whose directory is already gone is still registered: prune it, or git refuses to delete its branch.
	if _, err := Run(root, "worktree", "prune"); err != nil {
		return err
	}
	if _, err := Run(root, "rev-parse", "--verify", "-q", "refs/heads/"+branch); err != nil {
		return nil
	}
	_, err := Run(root, "branch", "-D", branch)
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

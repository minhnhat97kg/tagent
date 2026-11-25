package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const folderFlow = "name: notes\nworkplace: folder\njobs:\n  write:\n    agent: planner\n    prompt: Write\n"

// newFolderTicket returns a repo with a notes workflow, a ticket on it, and the
// ticket's folder (created, as dispatch does).
func newFolderTicket(t *testing.T, yml string) (root, scope string, task Task, dir string) {
	t.Helper()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	root = t.TempDir()
	scope = Scope(root)
	wf := filepath.Join(root, ".tasks", "workflows")
	os.MkdirAll(wf, 0o755)
	os.WriteFile(filepath.Join(wf, "notes.yml"), []byte(yml), 0o644)
	task, err := Add(scope, "t", nil, MutationOptions{Workflow: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	wp, err := TicketWorkplace(scope, task)
	if err != nil {
		t.Fatal(err)
	}
	if dir, err = wp.Create(root, task.ID); err != nil {
		t.Fatal(err)
	}
	return root, scope, task, dir
}

// attached returns the names of the comment attachments that read without error.
func attached(t *testing.T, scope, root, id string) map[string]bool {
	t.Helper()
	list, err := AttachmentList(scope, root, id)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, a := range list {
		if a.Source == "comment" && a.Error == "" {
			names[a.Name] = true
		}
	}
	return names
}

func TestFolderDoneAttachesEveryFileAndRemovesDir(t *testing.T) {
	root, scope, task, dir := newFolderTicket(t, folderFlow)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "b.md"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("h"), 0o644)
	os.Symlink(filepath.Join(root, "missing"), filepath.Join(dir, "link"))
	cleanupDone(scope, task.ID, "")
	names := attached(t, scope, root, task.ID)
	for _, want := range []string{"a.md", "b.md", ".hidden"} {
		if !names[want] {
			t.Fatalf("%s not attached: %v", want, names)
		}
	}
	if names["link"] {
		t.Fatalf("symlink attached: %v", names)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("folder kept: %v", err)
	}
}

func TestFolderKeepAttachesAndKeepsDir(t *testing.T) {
	root, scope, task, dir := newFolderTicket(t, strings.Replace(folderFlow, "workplace: folder\n", "workplace: folder\ncleanup: keep\n", 1))
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0o644)
	cleanupDone(scope, task.ID, "")
	if !attached(t, scope, root, task.ID)["a.md"] {
		t.Fatal("a.md not attached")
	}
	if _, err := os.Stat(filepath.Join(dir, "a.md")); err != nil {
		t.Fatalf("keep removed the folder: %v", err)
	}
}

func TestFolderMissingDirIsCleanDone(t *testing.T) {
	_, scope, task, dir := newFolderTicket(t, folderFlow)
	os.RemoveAll(dir)
	cleanupDone(scope, task.ID, "")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("folder reappeared: %v", err)
	}
}

func TestFolderAttachFailureKeepsDirAndNotes(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable files")
	}
	root, scope, task, dir := newFolderTicket(t, folderFlow)
	f := filepath.Join(dir, "secret.md")
	os.WriteFile(f, []byte("s"), 0o644)
	os.Chmod(f, 0)
	t.Cleanup(func() { os.Chmod(f, 0o644) })
	cleanupDone(scope, task.ID, "")
	if _, err := os.Stat(f); err != nil {
		t.Fatalf("folder removed after failed attach: %v", err)
	}
	if attached(t, scope, root, task.ID)["secret.md"] {
		t.Fatal("unreadable file attached")
	}
	events, _ := ReadEvents(scope, 0)
	for _, e := range events {
		if strings.HasPrefix(e.Message, "cleanup: kept folder "+dir) {
			return
		}
	}
	t.Fatalf("no kept-folder note: %+v", events)
}

// A running ticket's output refs, relative or absolute, resolve inside its folder.
func TestFolderOutputsResolveWhileRunning(t *testing.T) {
	root, scope, task, dir := newFolderTicket(t, folderFlow)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	abs := filepath.Join(dir, "sub", "b.md")
	os.WriteFile(abs, []byte("b"), 0o644)
	task.Outputs = map[string]map[string]string{"write": {"rel": "sub/b.md", "abs": abs}}
	if err := write(scope, task); err != nil {
		t.Fatal(err)
	}
	list, err := AttachmentList(scope, root, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := 0
	for _, a := range list {
		if a.Source == "output" && a.Error == "" {
			got++
		}
	}
	if got != 2 {
		t.Fatalf("want 2 resolved outputs, got %d: %+v", got, list)
	}
}

// A folder's directory, and any path inside it, belong to the repo's scope.
func TestFolderMapsToRepoScope(t *testing.T) {
	root, scope, _, dir := newFolderTicket(t, folderFlow)
	if got := Scope(dir); got != scope {
		t.Fatalf("Scope(folder) = %q, want %q", got, scope)
	}
	if got := Scope(filepath.Join(dir, "sub")); got != scope {
		t.Fatalf("Scope(folder/sub) = %q, want %q", got, scope)
	}
	if got := MainRoot(dir); got != MainRoot(root) {
		t.Fatalf("MainRoot(folder) = %q, want %q", got, MainRoot(root))
	}
	other := t.TempDir()
	if got := ownerDir(other); got != other {
		t.Fatalf("ownerDir(unrelated) = %q", got)
	}
}

// A folder reached through a symlinked context dir (as an agent may report it)
// still maps to the repo's scope and main root.
func TestFolderMapsToRepoScopeViaSymlinkedContext(t *testing.T) {
	root, scope, _, dir := newFolderTicket(t, folderFlow)
	ctx := os.Getenv("TMUX_AGENT_CONTEXT_DIR")
	link := filepath.Join(t.TempDir(), "ctx")
	if err := os.Symlink(ctx, link); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(realpath(ctx), dir)
	if err != nil {
		t.Fatal(err)
	}
	via := filepath.Join(link, rel)
	if got := Scope(via); got != scope {
		t.Fatalf("Scope(via symlink) = %q, want %q", got, scope)
	}
	if got := MainRoot(via); got != MainRoot(root) {
		t.Fatalf("MainRoot(via symlink) = %q, want %q", got, MainRoot(root))
	}
}

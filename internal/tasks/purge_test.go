package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func closeTicket(t *testing.T, scope, id string) {
	t.Helper()
	if err := Move(scope, id, "closed", ""); err != nil {
		t.Fatal(err)
	}
}

func TestPurgeDryRunChangesNothing(t *testing.T) {
	repo, scope, id, wt := newMergeRepo(t)
	closeTicket(t, scope, id)
	os.WriteFile(filepath.Join(wt, "dirty.txt"), []byte("dirty\n"), 0o644)
	items, err := Purge(scope, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Removed || items[0].Warning == "" {
		t.Fatalf("dry run items: %+v", items)
	}
	if _, err := Get(scope, id); err != nil {
		t.Fatal("dry run deleted the ticket")
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatal("dry run removed the worktree")
	}
	if gitRun(t, repo, "branch", "--list", "agent/workflow-"+id) == "" {
		t.Fatal("dry run removed the branch")
	}
}

func TestPurgeYesDiscardsWorktreeBranchAndAttachments(t *testing.T) {
	repo, scope, id, wt := newMergeRepo(t)
	closeTicket(t, scope, id)
	os.WriteFile(filepath.Join(wt, "dirty.txt"), []byte("dirty\n"), 0o644)
	gitRun(t, wt, "add", "dirty.txt")
	gitRun(t, wt, "commit", "-qm", "unmerged")
	os.WriteFile(filepath.Join(wt, "more.txt"), []byte("more\n"), 0o644)
	att := filepath.Join(boardDir(scope), id)
	os.MkdirAll(att, 0o700)
	os.WriteFile(filepath.Join(att, "note.txt"), []byte("x"), 0o600)
	keep, _ := Add(scope, "Keep me", nil)
	items, err := Purge(scope, true)
	if err != nil {
		t.Fatal(err)
	}
	var removed bool
	for _, it := range items {
		if it.ID == id {
			removed = it.Removed
		}
	}
	if !removed {
		t.Fatalf("not removed: %+v", items)
	}
	if _, err := Get(scope, id); err == nil {
		t.Fatal("ticket file kept")
	}
	if _, err := os.Stat(att); err == nil {
		t.Fatal("attachments kept")
	}
	if _, err := os.Stat(wt); err == nil {
		t.Fatal("worktree kept")
	}
	if gitRun(t, repo, "branch", "--list", "agent/workflow-"+id) != "" {
		t.Fatal("unmerged branch kept")
	}
	if _, err := Get(scope, keep.ID); err != nil {
		t.Fatal("open ticket purged")
	}
}

func TestPurgeKeepsClosedTicketsStillNeeded(t *testing.T) {
	_, scope := testBoard(t)
	a, _ := Add(scope, "A", nil)
	b, _ := Add(scope, "B", nil, MutationOptions{Needs: []string{a.ID}})
	c, _ := Add(scope, "C", nil, MutationOptions{Needs: []string{b.ID}})
	closeTicket(t, scope, a.ID)
	closeTicket(t, scope, b.ID)
	items, err := Purge(scope, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items %+v", items)
	}
	for _, it := range items {
		if it.Removed || it.Kept == "" {
			t.Fatalf("needed ticket removed: %+v", it)
		}
	}
	if _, err := Get(scope, a.ID); err != nil {
		t.Fatal("A deleted under a live chain")
	}
	if _, err := Get(scope, c.ID); err != nil {
		t.Fatal("open ticket C deleted")
	}
}

func TestPurgeFolderWorkplaceRemovesDirectory(t *testing.T) {
	dir, scope := testBoard(t)
	if err := os.MkdirAll(filepath.Join(dir, ".tasks", "workflows"), 0o700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".tasks", "workflows", "fold.yml"), []byte("name: fold\nworkplace: folder\njobs:\n  build: {status: building, agent: implementer}\n"), 0o600)
	if err := SaveBoardConfig(dir, []byte("roles: {implementer: {agent: pi}}\n")); err != nil {
		t.Fatal(err)
	}
	ticket, err := Add(scope, "Folder work", nil, MutationOptions{Workflow: "fold"})
	if err != nil {
		t.Fatal(err)
	}
	closeTicket(t, scope, ticket.ID)
	wp, err := TicketWorkplace(scope, ticket)
	if err != nil {
		t.Fatal(err)
	}
	out := wp.Output(dir, ticket.ID)
	os.MkdirAll(out, 0o700)
	if _, err := Purge(scope, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("folder workplace kept")
	}
}

// A discard that fails keeps the ticket, and a retry after the cause is gone removes it.
func TestPurgeFailedDiscardKeepsTicketThenRetries(t *testing.T) {
	repo, scope, id, _ := newMergeRepo(t)
	closeTicket(t, scope, id)
	lock := filepath.Join(repo, ".git", "refs", "heads", "agent", "workflow-"+id+".lock")
	os.WriteFile(lock, nil, 0o600) // a held ref lock makes branch -D fail
	items, err := Purge(scope, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Removed || !strings.HasPrefix(items[0].Kept, "discard failed") {
		t.Fatalf("failed discard: %+v", items)
	}
	if _, err := Get(scope, id); err != nil {
		t.Fatal("ticket removed after a failed discard")
	}
	os.Remove(lock)
	if items, err = Purge(scope, true); err != nil || len(items) != 1 || !items[0].Removed {
		t.Fatalf("retry: %+v %v", items, err)
	}
	if _, err := Get(scope, id); err == nil {
		t.Fatal("ticket kept after retry")
	}
}

// A workplace whose directory is already gone is still purged (stale checkout).
func TestPurgeStaleWorktreeDirectory(t *testing.T) {
	repo, scope, id, wt := newMergeRepo(t)
	closeTicket(t, scope, id)
	os.RemoveAll(wt)
	if items, err := Purge(scope, true); err != nil || len(items) != 1 || !items[0].Removed {
		t.Fatalf("stale: %+v %v", items, err)
	}
	if gitRun(t, repo, "branch", "--list", "agent/workflow-"+id) != "" {
		t.Fatal("branch kept")
	}
}

package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTicketAttachmentConfinement(t *testing.T) {
	dir, scope := testBoard(t)
	ticket, err := Add(scope, "Files", nil)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "same.log")
	os.WriteFile(p, []byte("log"), 0600)
	if err = Comment(scope, ticket.ID, "user", "", []string{p, p}); err != nil {
		t.Fatal(err)
	}
	list, err := AttachmentList(scope, dir, ticket.ID)
	if err != nil || len(list) != 2 || list[0].Ref == list[1].Ref {
		t.Fatalf("%+v %v", list, err)
	}
	os.MkdirAll(filepath.Join(dir, "plans"), 0700)
	os.WriteFile(filepath.Join(dir, "plans", "plan.md"), []byte("plan"), 0600)
	ticket, _ = Get(scope, ticket.ID)
	ticket.Outputs = map[string]map[string]string{"build": {"plan": "plans/plan.md", "missing": "plans/missing.md", "scalar": "ok", "escape": "../outside.txt"}}
	if err = write(scope, ticket); err != nil {
		t.Fatal(err)
	}
	list, err = AttachmentList(scope, dir, ticket.ID)
	if err != nil || len(list) != 5 {
		t.Fatalf("%+v %v", list, err)
	}
	for _, a := range list {
		f, _, e := OpenTicketAttachment(scope, dir, ticket.ID, a.Ref)
		if a.Error != "" {
			if e == nil {
				f.Close()
				t.Fatal("opened unavailable")
			}
		} else {
			if e != nil {
				t.Fatal(e)
			}
			f.Close()
		}
	}
	if _, _, err = OpenTicketAttachment(scope, dir, ticket.ID, "../plans/plan.md"); err == nil {
		t.Fatal("unauthorized")
	}
	// Replacing either the file or an ancestor with a symlink must fail.
	os.Rename(filepath.Join(dir, "plans"), filepath.Join(dir, "real-plans"))
	os.Symlink("real-plans", filepath.Join(dir, "plans"))
	list, _ = AttachmentList(scope, dir, ticket.ID)
	for _, a := range list {
		if a.Name == "plan.md" && a.Error == "" {
			t.Fatal("followed output ancestor")
		}
	}
	c := ticket.Comments[0].Attachments[0]
	stored := filepath.Join(boardDir(scope), ticket.ID, c.File)
	os.Remove(stored)
	os.Symlink(p, stored)
	if _, err = OpenAttachment(scope, ticket.ID, c.File); err == nil {
		t.Fatal("followed stored symlink")
	}
	if err = Comment(scope, ticket.ID, "user", "", []string{p, filepath.Join(dir, "absent")}); err == nil {
		t.Fatal("expected rollback")
	}
	entries, _ := os.ReadDir(filepath.Join(boardDir(scope), ticket.ID))
	if len(entries) != 2 {
		t.Fatalf("rollback left %d files", len(entries))
	}
	if _, err = AttachmentList(scope, dir, "../bad"); err == nil {
		t.Fatal("invalid id")
	}
	if err = Comment(scope, ticket.ID, "user", strings.Repeat("long ", 150), nil); err != nil {
		t.Fatal(err)
	}
	list, _ = AttachmentList(scope, dir, ticket.ID)
	found := false
	for _, a := range list {
		if a.Name == "comment.md" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing compact comment")
	}
}

// A symlinked ancestor above the trusted root must not break storage or --attach.
func TestSymlinkedAncestorAboveBoardAllowed(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", filepath.Join(link, "ctx"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	proj := filepath.Join(link, "proj")
	os.MkdirAll(proj, 0700)
	scope := Scope(proj)
	ticket, err := Add(scope, "Long", nil)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(proj, "note.txt")
	os.WriteFile(src, []byte("note"), 0600)
	if err = Comment(scope, ticket.ID, "user", strings.Repeat("x", 700), []string{src}); err != nil {
		t.Fatal(err)
	}
	list, err := AttachmentList(scope, proj, ticket.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("%+v %v", list, err)
	}
	for _, a := range list {
		if a.Error != "" {
			t.Fatalf("unavailable through symlinked ancestor: %+v", a)
		}
	}
}

// A sentence output is text, not a file; a plan still in the ticket's worktree is found there.
func TestOutputAttachmentsSkipTextAndFindWorktreeFiles(t *testing.T) {
	dir, scope := testBoard(t)
	ticket, err := Add(scope, "Plan", nil)
	if err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(dir, ".worktrees", "workflow-"+ticket.ID, ".tasks", "plans")
	os.MkdirAll(wt, 0700)
	os.WriteFile(filepath.Join(wt, ticket.ID+".md"), []byte("plan"), 0600)
	ticket, _ = Get(scope, ticket.ID)
	ticket.Outputs = map[string]map[string]string{"plan": {"plan": ".tasks/plans/" + ticket.ID + ".md"}, "reproduce": {"cause": "server.go newAgent", "test": "worktree field never reaches it"}}
	if err = write(scope, ticket); err != nil {
		t.Fatal(err)
	}
	list, err := AttachmentList(scope, dir, ticket.ID)
	if err != nil || len(list) != 1 || list[0].Error != "" || list[0].Size != 4 {
		t.Fatalf("%+v %v", list, err)
	}
}

package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupKeepsOutputsAsAttachments(t *testing.T) {
	root := t.TempDir()
	scope := Scope(root)
	task, err := Add(scope, "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(root, ".worktrees", "workflow-"+task.ID, ".tasks", "plans")
	os.MkdirAll(plan, 0o755)
	os.WriteFile(filepath.Join(plan, task.ID+".md"), []byte("plan"), 0o644)
	task.Outputs = map[string]map[string]string{"plan": {"plan": ".tasks/plans/" + task.ID + ".md"}}
	if err := write(scope, task); err != nil {
		t.Fatal(err)
	}
	keepOutputs(scope, task.ID, filepath.Join(root, ".worktrees", "workflow-"+task.ID))
	os.RemoveAll(filepath.Join(root, ".worktrees"))
	list, err := AttachmentList(scope, root, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range list {
		if a.Source == "comment" && a.Name == task.ID+".md" && a.Error == "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("plan not attached: %+v", list)
	}
}

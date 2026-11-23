package tasks

import "testing"

func TestSetNoteSurvivesUpdates(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	task, err := Add("repo-1", "do it", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetNote("repo-1", task.ID, "  added retry  "); err != nil {
		t.Fatal(err)
	}
	if _, err := Update("repo-1", task.ID, "done", nil); err != nil {
		t.Fatal(err)
	}
	got, _ := Get("repo-1", task.ID)
	if got.Note != "added retry" || got.Status != "done" {
		t.Fatalf("note must be trimmed and kept after a status update: %+v", got)
	}
}

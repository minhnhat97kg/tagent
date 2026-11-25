package board

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tagent/internal/tasks"
)

func TestScoutSkip(t *testing.T) {
	cases := []struct {
		name string
		task tasks.Task
		skip bool
	}{
		{"no names", tasks.Task{Title: "make search faster", Text: "users wait too long"}, false},
		{"file named", tasks.Task{Title: "fix", Text: "bug in internal/tasks/move.go"}, true},
		{"function named", tasks.Task{Title: "x", Text: "Call Expand() twice"}, true},
		{"input names file", tasks.Task{Title: "x", Inputs: map[string]string{"goal": "see web/app.tsx"}}, true},
		{"output exists", tasks.Task{Title: "x", Outputs: map[string]map[string]string{"scout": {"scout": "done"}}}, true},
	}
	for _, c := range cases {
		if got := scoutSkip(c.task, "scout") != ""; got != c.skip {
			t.Errorf("%s: skip=%v want %v", c.name, got, c.skip)
		}
	}
}

func TestScoutActions(t *testing.T) {
	cfg := tasks.BoardConfig{Statuses: []tasks.Status{
		{ID: "scouting", Role: "ba", Uses: "scout", Job: "scout", Outputs: []string{"scout"}, Next: []string{"plan"}},
		{ID: "plan", Role: "planner", Job: "plan", Next: []string{"done"}},
	}}
	ev := tasks.Event{Kind: "status", From: "todo", To: "scouting", Task: "1", Seq: 7, Actor: "user"}
	dir := t.TempDir()
	named, _ := Actions(dir, cfg, ev, tasks.Task{ID: "1", Status: "scouting", Title: "fix", Text: "bug in internal/tasks/move.go"})
	if len(named) != 1 || named[0].Advance != "plan" || named[0].Note == "" {
		t.Fatalf("named file should skip scout: %+v", named)
	}
	open, _ := Actions(dir, cfg, ev, tasks.Task{ID: "1", Status: "scouting", Title: "make it faster", Text: "users wait"})
	if len(open) != 1 || open[0].Advance != "" || open[0].Preset != "scout" {
		t.Fatalf("unnamed ticket should run scout: %+v", open)
	}
}

func TestCIBounceReusesPaneWithLog(t *testing.T) {
	cfg := tasks.BoardConfig{Statuses: []tasks.Status{
		{ID: "fixing", Role: "dev", Job: "fix", Next: []string{"done"}},
	}}
	ci := tasks.TaskComment{ID: "c1", Author: "ci", Body: "CI exit code 1\n--- FAIL: TestX", Attachments: []tasks.Attachment{{Name: "ci.log", File: "ci.log"}}}
	task := tasks.Task{ID: "1", Status: "fixing", Title: "fix", Comments: []tasks.TaskComment{ci}}
	dir := t.TempDir()
	bounce, _ := Actions(dir, cfg, tasks.Event{Kind: "status", From: "reproducing", To: "fixing", Task: "1", Seq: 3, Actor: "ci", CommentID: "c1"}, task)
	if len(bounce) != 1 || bounce[0].FreshAfter != 0 || !strings.Contains(bounce[0].Prompt, "Full CI log: ") || !strings.Contains(bounce[0].ResumePrompt(0), "Full CI log: ") {
		t.Fatalf("CI bounce should reuse the pane with the log: %+v", bounce)
	}
	plain, _ := Actions(dir, cfg, tasks.Event{Kind: "status", From: "todo", To: "fixing", Task: "1", Seq: 4, Actor: "user"}, tasks.Task{ID: "1", Status: "fixing", Title: "fix"})
	if len(plain) != 1 || plain[0].FreshAfter != 3 || strings.Contains(plain[0].Prompt, "Full CI log") {
		t.Fatalf("ordinary entry changed: %+v", plain)
	}
}

// A skipped scout advances the ticket in the dispatcher and the next job fires.
func TestSkippedScoutAdvancesToPlan(t *testing.T) {
	dir, scope := setup(t, "")
	wf := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(wf, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wf, "scouted.yml"), []byte("name: scouted\ninputs:\n  goal: {required: true}\njobs:\n  scout: {uses: scout}\n  plan: {needs: scout, agent: planner}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	task, err := tasks.Add(scope, "fix\nbug in internal/tasks/move.go", nil, tasks.MutationOptions{Workflow: "scouted", Inputs: map[string]string{"goal": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tasks.Move(scope, task.ID, "scouting", ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var fired []string
	Run(ctx, dir, func(a Action) error {
		fired = append(fired, a.Agent+"@"+a.Advance)
		if a.Agent == "planner" {
			cancel()
		}
		return nil
	})
	got, _ := tasks.Get(scope, task.ID)
	if got.Status != "plan" || len(fired) != 1 || fired[0] != "planner@" {
		t.Fatalf("status=%s fired=%v (scout must not fire)", got.Status, fired)
	}
}

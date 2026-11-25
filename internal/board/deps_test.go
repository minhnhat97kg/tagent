package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/tasks"
)

const gatedConfig = `statuses:
  - {id: todo}
  - {id: doing, role: implementer}
  - {id: blocked}
  - {id: done}
`

func TestActionsWaitForDependencies(t *testing.T) {
	dir, scope := setup(t, gatedConfig)
	cfg, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := tasks.Add(scope, "First", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tasks.Add(scope, "Second", nil, tasks.MutationOptions{Needs: []string{first.ID}})
	if err != nil {
		t.Fatal(err)
	}
	entry := tasks.Event{Kind: "status", From: "todo", To: "doing", Actor: "user"}
	actions, err := Actions(dir, cfg, entry, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 0 {
		t.Fatalf("dispatched a waiting ticket: %+v", actions)
	}
	if _, err = tasks.Update(scope, first.ID, "done", nil); err != nil {
		t.Fatal(err)
	}
	actions, err = Actions(dir, cfg, entry, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Role != "implementer" {
		t.Fatalf("ready ticket not dispatched: %+v", actions)
	}
}

func TestAutoStartDependents(t *testing.T) {
	dir, scope := setup(t, "max_fires_per_hour: 20\n")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, ".tasks", "workflows"), 0700))
	must(os.WriteFile(filepath.Join(dir, ".tasks", "workflows", "build.yml"), []byte("name: build\njobs:\n  build: {run: echo ok}\n"), 0600))
	a, err := tasks.Add(scope, "A", nil)
	must(err)
	c, err := tasks.Add(scope, "C", nil)
	must(err)
	b, err := tasks.Add(scope, "B", nil, tasks.MutationOptions{Needs: []string{a.ID}, AutoStart: true})
	must(err)
	x, err := tasks.Add(scope, "X", nil, tasks.MutationOptions{Needs: []string{a.ID, c.ID}, AutoStart: true})
	must(err)
	n, err := tasks.Add(scope, "N", nil, tasks.MutationOptions{Needs: []string{a.ID}, AutoStart: true})
	must(err)
	// M has a workflow but auto_start unset: it must stay todo.
	m, err := tasks.Add(scope, "M", nil, tasks.MutationOptions{Needs: []string{a.ID}})
	must(err)
	must(tasks.SetWorkflow(scope, b.ID, "build"))
	must(tasks.SetWorkflow(scope, x.ID, "build"))
	must(tasks.SetWorkflow(scope, m.ID, "build"))
	_, err = tasks.Update(scope, a.ID, "done", nil)
	must(err)
	must(autoStart(dir, a.ID))

	// Only dependents whose last dependency is done start; the one without a workflow stays.
	if got, _ := tasks.Get(scope, b.ID); got.Status == "todo" || !hasComment(got, "#"+a.ID) {
		t.Fatalf("B did not start with a comment naming #%s: %q %+v", a.ID, got.Status, got.Comments)
	}
	if got, _ := tasks.Get(scope, x.ID); got.Status != "todo" {
		t.Fatalf("X started while C is open: %q", got.Status)
	}
	if got, _ := tasks.Get(scope, n.ID); got.Status != "todo" {
		t.Fatalf("ticket without workflow started: %q", got.Status)
	}
	if got, _ := tasks.Get(scope, m.ID); got.Status != "todo" {
		t.Fatalf("ticket with auto_start unset started: %q", got.Status)
	}
	// The status event Start wrote must dispatch B's first job.
	events, err := tasks.ReadEvents(scope, 0)
	must(err)
	cfg, err := tasks.LoadBoardConfig(dir)
	must(err)
	b, _ = tasks.Get(scope, b.ID)
	fired := 0
	for _, ev := range events {
		if ev.Task == b.ID && ev.Kind == "status" && ev.From == "todo" {
			actions, err := Actions(dir, cfg, ev, b)
			must(err)
			fired += len(actions)
		}
	}
	if fired != 1 {
		t.Fatalf("B's first job fired %d times, want 1", fired)
	}

	// The last dependency of X is done now, so X starts and its comment names C.
	_, err = tasks.Update(scope, c.ID, "done", nil)
	must(err)
	must(autoStart(dir, c.ID))
	if got, _ := tasks.Get(scope, x.ID); got.Status == "todo" || !hasComment(got, "#"+c.ID) {
		t.Fatalf("X did not start after its last dependency: %q %+v", got.Status, got.Comments)
	}
}

func hasComment(t tasks.Task, text string) bool {
	for _, c := range t.Comments {
		if strings.Contains(c.Body, text) {
			return true
		}
	}
	return false
}

func TestClosedDependencyNeverAutoStartsDependent(t *testing.T) {
	dir, scope := setup(t, "max_fires_per_hour: 20\n")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, ".tasks", "workflows"), 0700))
	must(os.WriteFile(filepath.Join(dir, ".tasks", "workflows", "build.yml"), []byte("name: build\njobs:\n  build: {run: echo ok}\n"), 0600))
	a, err := tasks.Add(scope, "A", nil)
	must(err)
	b, err := tasks.Add(scope, "B", nil, tasks.MutationOptions{Needs: []string{a.ID}, AutoStart: true})
	must(err)
	must(tasks.SetWorkflow(scope, b.ID, "build"))
	must(tasks.Move(scope, a.ID, "closed", "dropped"))
	must(autoStart(dir, a.ID))
	if got, _ := tasks.Get(scope, b.ID); got.Status != "todo" {
		t.Fatalf("B started on a closed dependency: %q", got.Status)
	}
}

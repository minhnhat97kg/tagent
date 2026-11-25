package board

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tagent/internal/tasks"
)

func setup(t *testing.T, cfg string) (string, string) {
	t.Helper()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	if err := tasks.SaveBoardConfig(dir, []byte(cfg)); err != nil {
		t.Fatal(err)
	}
	return dir, tasks.Scope(dir)
}
func TestActions(t *testing.T) {
	cfg, err := tasks.ParseBoardConfig([]byte(`triggers:
  - on: mention
    when: {to: '*', actor: user}
    agent: '{{event.name}}'
    prompt: '{{comment.body}} / {{task.assignees}}'
  - on: status
    when: {from: doing, to: review}
    agent: check
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Roles = map[string]tasks.Role{"a": {}, "b": {}}
	task := tasks.Task{ID: "abc", Title: "Title", Assignees: []string{"a", "b"}, Comments: []tasks.TaskComment{{ID: "c", Body: "raw {{task.id}}", Author: "user"}}}
	actions, err := Actions("dir", cfg, tasks.Event{Kind: "mention", Actor: "user", CommentID: "c", Names: []string{"a", "b"}}, task)
	if err != nil || len(actions) != 2 || actions[1].Agent != "b" || !strings.HasSuffix(actions[0].Prompt, "raw {{task.id}} / a,b\n") {
		t.Fatal(actions, err)
	}
	// A fresh pane gets the brief; a reused one gets only the comments after its cursor.
	task.Comments = append(task.Comments, tasks.TaskComment{ID: "d", Body: "second", Author: "user"}, tasks.TaskComment{ID: "e", Body: "third", Author: "system"})
	actions, _ = Actions("dir", cfg, tasks.Event{Kind: "mention", Actor: "user", CommentID: "d", Names: []string{"a"}}, task)
	if len(actions) != 1 || actions[0].CommentCount != 3 || !strings.HasPrefix(actions[0].Prompt, "abc: Title") {
		t.Fatal(actions)
	}
	resume := actions[0].ResumePrompt(1)
	if strings.Contains(resume, "raw") || !strings.Contains(resume, "second") || strings.Contains(resume, "third") || strings.Contains(resume, "Status:") {
		t.Fatal(resume)
	}
	actions, _ = Actions("dir", cfg, tasks.Event{Kind: "mention", Actor: "system", Names: []string{"a"}}, task)
	if len(actions) != 0 {
		t.Fatal("system loop")
	}
	cfg.Triggers[0].When.Actor = "*"
	actions, _ = Actions("dir", cfg, tasks.Event{Kind: "mention", Actor: "a", Names: []string{"a", "b"}}, task)
	if len(actions) != 1 || actions[0].Agent != "b" {
		t.Fatal("self loop", actions)
	}
	actions, _ = Actions("dir", cfg, tasks.Event{Kind: "status", From: "doing", To: "review", Actor: "user"}, task)
	if len(actions) != 1 || !strings.Contains(actions[0].Prompt, "task show abc") {
		t.Fatal(actions)
	}
	actions, _ = Actions("dir", cfg, tasks.Event{Kind: "status", From: "todo", To: "review", Actor: "user"}, task)
	if len(actions) != 0 {
		t.Fatal(actions)
	}
}
func TestReservationLockAndRestart(t *testing.T) {
	dir, scope := setup(t, `triggers:
  - on: create
    agent: planner
  - on: create
    agent: implementer
`)
	task, err := tasks.Add(scope, "request", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err = Run(ctx, dir, func(a Action) error {
		calls++
		// The reservation must already be readable before the external side effect.
		b, e := os.ReadFile(filepath.Join(tasks.BoardDir(scope), "dispatch.state"))
		var s state
		json.Unmarshal(b, &s)
		if e != nil || s.LastSeq != a.Event.Seq || len(s.Fires[task.ID]) != 1 {
			t.Fatal("missing reservation", string(b), e)
		}
		if e = Run(ctx, dir, func(Action) error { return nil }); !errors.Is(e, ErrLocked) {
			t.Fatal("duplicate dispatcher", e)
		}
		cancel()
		return errors.New("simulated interrupted send")
	})
	if err != nil || calls != 1 {
		t.Fatal(calls, err)
	}
	ctx2, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	if err = Run(ctx2, dir, func(Action) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("replayed reserved event", calls)
	}
	events, _ := tasks.ReadEvents(scope, 0)
	reserved, failed := false, false
	for _, ev := range events {
		if ev.Kind != "dispatch" || ev.Task != task.ID {
			continue
		}
		reserved = reserved || strings.Contains(ev.Message, "reserved; outcome uncertain")
		failed = failed || strings.Contains(ev.Message, "failure: simulated interrupted send")
	}
	if !reserved || !failed {
		t.Fatal("missing reservation/failure audits", events)
	}
	b, _ := os.ReadFile(filepath.Join(tasks.BoardDir(scope), "dispatch.state"))
	if !strings.Contains(string(b), "simulated interrupted send") {
		t.Fatal(string(b))
	}
}
func TestRateLimitAndSystemComments(t *testing.T) {
	dir, scope := setup(t, `max_fires_per_hour: 1
triggers:
  - on: create
    agent: planner
  - on: comment
    agent: planner
`)
	task, _ := tasks.Add(scope, "request", nil)
	ctx, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	calls := 0
	if err := Run(ctx, dir, func(Action) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	got, _ := tasks.Get(scope, task.ID)
	events, _ := tasks.ReadEvents(scope, 0)
	audits := 0
	for _, ev := range events {
		if ev.Kind == "dispatch" && strings.Contains(ev.Message, "triggered planner") {
			audits++
		}
	}
	if calls != 1 || len(got.Comments) != 0 || audits != 1 {
		t.Fatal(calls, got)
	}
	tasks.Comment(scope, task.ID, "user", "another event", nil)
	ctx2, stop2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop2()
	if err := Run(ctx2, dir, func(Action) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("cap not persisted", calls)
	}
	events, _ = tasks.ReadEvents(scope, 0)
	found := false
	for _, ev := range events {
		if ev.Task == task.ID && strings.Contains(ev.Message, "suppressed: hourly rate limit") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing suppression audit")
	}
}
func TestInvalidConfigDoesNotConsume(t *testing.T) {
	dir, scope := setup(t, "triggers: [{on: create, agent: planner}]")
	tasks.Add(scope, "request", nil)
	os.WriteFile(tasks.BoardConfigPath(dir), []byte("columns: [oops]"), 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := Run(ctx, dir, func(Action) error { t.Fatal("invalid config fired"); return nil }); err != nil {
		t.Fatal(err)
	}
	tasks.SaveBoardConfig(dir, []byte("triggers: [{on: create, agent: planner}]"))
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	calls := 0
	if err := Run(ctx2, dir, func(Action) error { calls++; cancel2(); return nil }); err != nil || calls != 1 {
		t.Fatal(calls, err)
	}
}

func TestMentionIgnoresNonAgents(t *testing.T) {
	cfg, err := tasks.ParseBoardConfig([]byte("triggers:\n  - on: mention\n    when: {to: '*'}\n    agent: '{{event.name}}'\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Roles = map[string]tasks.Role{"qa": {}}
	task := tasks.Task{ID: "1", Comments: []tasks.TaskComment{{ID: "c"}}}
	a, err := Actions(t.TempDir(), cfg, tasks.Event{Kind: "mention", Actor: "dev", CommentID: "c", Names: []string{"pane_role", "qa"}}, task)
	if err != nil || len(a) != 1 || a[0].Agent != "qa" {
		t.Fatal(a, err)
	}
}

func TestFreshBoardSkipsHistory(t *testing.T) {
	dir, scope := setup(t, `triggers:
  - on: create
    agent: planner
`)
	_, err := tasks.Add(scope, "old", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Backdate the event: it was copied in from another scope.
	path := filepath.Join(tasks.BoardDir(scope), "events.jsonl")
	b, _ := os.ReadFile(path)
	var ev tasks.Event
	json.Unmarshal(b, &ev)
	ev.Time = time.Now().Add(-time.Hour)
	nb, _ := json.Marshal(ev)
	os.WriteFile(path, append(nb, '\n'), 0600)
	ctx, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	if err = Run(ctx, dir, func(Action) error { t.Fatal("fired history"); return nil }); err != nil {
		t.Fatal(err)
	}
}

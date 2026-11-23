package tasks

import "testing"

func TestBlockedReplyRouting(t *testing.T) {
	dir, scope := testBoard(t)
	if err := SaveBoardConfig(dir, []byte("statuses:\n  - {id: todo}\n  - {id: build, role: implementer}\n  - {id: verify, role: reviewer}\n  - {id: blocked}\n  - {id: done}\n")); err != nil {
		t.Fatal(err)
	}
	task, err := Add(scope, "Reply test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = Move(scope, task.ID, "build", ""); err != nil {
		t.Fatal(err)
	}
	if err = Move(scope, task.ID, "blocked", "Which token?", MutationOptions{Actor: "implementer"}); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"answer", "@unknown answer", "@implementer @reviewer answer"} {
		err = Comment(scope, task.ID, "user", body, nil)
		if body != "answer" {
			if _, ok := err.(*CommentNotice); !ok {
				t.Fatalf("notice: %v", err)
			}
		}
		got, _ := Get(scope, task.ID)
		if got.Status != "blocked" {
			t.Fatal(got.Status)
		}
	}
	got, _ := Get(scope, task.ID)
	cfg, _ := ConfigForScopeTask(scope, got)
	history, _ := ReadEvents(scope, 0)
	_, role, reason := BlockContext(got, cfg, history)
	if role != "implementer" || reason != "Which token?" {
		t.Fatal(role, reason)
	}
	if err = Comment(scope, task.ID, "user", "@implementer token in env", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = Get(scope, task.ID)
	if got.Status != "build" {
		t.Fatal(got.Status)
	}
	history, _ = ReadEvents(scope, 0)
	last := history[len(history)-1]
	if last.Kind != "status" || last.From != "blocked" || last.CommentID == "" {
		t.Fatal(last)
	}
	if err = Move(scope, task.ID, "blocked", "Which reviewer?"); err != nil {
		t.Fatal(err)
	}
	if err = Comment(scope, task.ID, "user", "@reviewer please check", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = Get(scope, task.ID)
	if got.Status != "verify" {
		t.Fatal(got.Status)
	}
}

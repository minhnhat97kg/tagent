package board

import (
	"fmt"
	"testing"

	"tagent/internal/tasks"
)

const ciLoopConfig = `roles: {implementer: {agent: pi}}
statuses:
 - {id: todo}
 - {id: build, role: implementer}
 - {id: test, run: "go test ./...", pass: done, fail: build}
 - {id: blocked}
 - {id: done}
`

func ciFail(id string, secs int) tasks.TaskComment {
	return tasks.TaskComment{ID: id, Author: "ci", Body: fmt.Sprintf("CI exit code 1\n--- FAIL: TestA (%ds)\n", secs)}
}

// The failed run's on-failure move back to dev must block when its output is
// identical to the previous failed run's (timings differ).
func TestIdenticalRunFailureBlocks(t *testing.T) {
	dir, _ := setup(t, ciLoopConfig)
	cfg, _ := tasks.LoadBoardConfig(dir)
	task := tasks.Task{ID: "abc", Status: "build", Comments: []tasks.TaskComment{ciFail("c1", 1), ciFail("c2", 2)}}
	ev := tasks.Event{Kind: "status", From: "test", To: "build", Actor: "ci", CommentID: "c2", Seq: 5}
	actions, err := Actions(dir, cfg, ev, task)
	if err != nil || len(actions) != 1 {
		t.Fatal(actions, err)
	}
	if actions[0].BlockReason == "" {
		t.Fatal("identical run failure was re-dispatched to dev instead of blocked")
	}
	// Different output is a new failure: dev gets it.
	task.Comments[1].Body = "CI exit code 1\n--- FAIL: TestB (2s)\n"
	actions, _ = Actions(dir, cfg, ev, task)
	if len(actions) != 1 || actions[0].BlockReason != "" {
		t.Fatal("different failure blocked", actions)
	}
}

// A user's "skip test" lets the next run job pass once; an agent's comment is ignored.
func TestSkipOnlyFromUserAndOnce(t *testing.T) {
	dir, _ := setup(t, ciLoopConfig)
	cfg, _ := tasks.LoadBoardConfig(dir)
	ev := tasks.Event{Kind: "status", From: "build", To: "test", Actor: "implementer", Seq: 7}
	run := func(comments ...tasks.TaskComment) Action {
		t.Helper()
		actions, err := Actions(dir, cfg, ev, tasks.Task{ID: "abc", Status: "test", Comments: comments})
		if err != nil || len(actions) != 1 {
			t.Fatal(actions, err)
		}
		return actions[0]
	}
	user := tasks.TaskComment{ID: "u", Author: "user", Body: "@lead skip test"}
	if !run(ciFail("c1", 1), user).Skip {
		t.Fatal("user skip ignored")
	}
	agent := tasks.TaskComment{ID: "a", Author: "dev", Body: "I will skip test for now"}
	if run(ciFail("c1", 1), agent).Skip {
		t.Fatal("agent skip honoured")
	}
	// The skipped run's own CI result comes after the user comment: the next run is not skipped.
	passed := tasks.TaskComment{ID: "c2", Author: "ci", Body: "CI exit code 0\n"}
	if run(ciFail("c1", 1), user, passed).Skip {
		t.Fatal("skip honoured twice")
	}
	// A skipped run writes "CI skipped" instead of an exit code; it must also end the skip.
	skipped := tasks.TaskComment{ID: "c3", Author: "ci", Body: "CI skipped: a user comment asked to skip this run"}
	if run(ciFail("c1", 1), user, skipped).Skip {
		t.Fatal("skip honoured again after a skipped run")
	}
}

// A test that fails on the base commit too is not the ticket's fault: the
// decision is forward, not dev.
func TestPreExistingOnlyWhenAllFailOnBase(t *testing.T) {
	if !PreExisting([]string{"TestA", "TestB"}, []string{"TestB", "TestA", "TestC"}) {
		t.Fatal("all failures on base not pre-existing")
	}
	if PreExisting([]string{"TestA", "TestD"}, []string{"TestA"}) {
		t.Fatal("new failure treated as pre-existing")
	}
	if PreExisting(nil, []string{"TestA"}) {
		t.Fatal("no failures treated as pre-existing")
	}
	if got := FailedTests("--- FAIL: TestA (0.00s)\n--- FAIL: TestA/sub (0.00s)\n--- FAIL: TestB (1s)\n"); len(got) != 2 || got[0] != "TestA" || got[1] != "TestB" {
		t.Fatal(got)
	}
}

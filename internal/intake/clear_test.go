package intake

import (
	"errors"
	"testing"
)

func TestClearConversation(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	scope := t.TempDir()
	s, err := ClearConversation(scope, 0, false)
	if err != nil || s.Revision != 1 || s.Generation != 1 {
		t.Fatalf("empty clear: %+v %v", s, err)
	}
	old, err := ReserveSubmission(scope, "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	s, err = ClearConversation(scope, old.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Messages) != 0 || s.Generation <= old.Generation || s.Status != "idle" {
		t.Fatalf("reset: %+v", s)
	}
	if _, err = CompleteTypedSubmission(scope, old.Generation, Result{Answer: "late"}, "late", nil, nil); !errors.Is(err, ErrStale) {
		t.Fatalf("late completion: %v", err)
	}
	if _, err = ClearConversation(scope, old.Revision, false); !errors.Is(err, ErrStale) {
		t.Fatalf("stale clear: %v", err)
	}
	loaded, err := LoadConversation(scope)
	if err != nil || loaded.Revision != s.Revision || len(loaded.Messages) != 0 {
		t.Fatalf("visibility: %+v %v", loaded, err)
	}
}

func TestClearApproval(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	scope := t.TempDir()
	s, err := ReserveSubmission(scope, "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	s, err = CompleteTypedSubmission(scope, s.Generation, Result{Calls: []ToolCall{{Name: "board_summary"}}}, "proposal", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err = ClaimApproval(scope, s.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ClearConversation(scope, s.Revision, true); !errors.Is(err, ErrBusy) {
		t.Fatalf("active approval clear: %v", err)
	}
	s, err = conversationUpdate(scope, func(s *Conversation) error { s.Status = "uncertain"; s.Owner = 0; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ClearConversation(scope, s.Revision, false); err == nil {
		t.Fatal("uncertain clear needs acknowledgement")
	}
	if _, err = ClearConversation(scope, s.Revision, true); err != nil {
		t.Fatal(err)
	}
}

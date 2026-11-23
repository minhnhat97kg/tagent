package intake

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"tagent/internal/sharedctx"
	"testing"
	"time"
)

func TestConversationSharedTransitions(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	s, err := LoadConversation("project")
	if err != nil || s.Revision != 0 {
		t.Fatal(s, err)
	}
	s, err = ReserveSubmission("project", "build a thing", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ReserveSubmission("project", "other", ""); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	if _, err = CompleteSubmission("project", s.Generation+1, "Q: which thing?", nil, nil); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	s, err = CompleteSubmission("project", s.Generation, "Q: which thing?", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err = ReserveSubmission("project", "a small thing", "")
	if err != nil || len(s.Turns) != 1 {
		t.Fatal(s, err)
	}
	s, err = CompleteSubmission("project", s.Generation, "CMD: tagent reload", nil, nil)
	if err != nil || s.Status != "proposal" {
		t.Fatal(s, err)
	}
	revision := s.Revision
	var wg sync.WaitGroup
	var mu sync.Mutex
	claims := 0
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := ClaimApproval("project", revision); e == nil {
				mu.Lock()
				claims++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if claims != 1 {
		t.Fatal(claims)
	}
	s, err = FinishApproval("project", s.Generation, "done")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ReserveSubmission("project", "next request", ""); err != nil {
		t.Fatal(err)
	}
	other, err := LoadConversation("other")
	if err != nil || len(other.Messages) != 0 {
		t.Fatal(other, err)
	}
}

func TestConversationRecoveryAndCorruption(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	s, err := ReserveSubmission("p", "question", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = conversationUpdate("p", func(s *Conversation) error { s.Deadline = time.Now().Add(-time.Second); return nil })
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := LoadConversation("p")
	if err != nil || recovered.Status != "failed" {
		t.Fatal(recovered, err)
	}
	if _, err = CompleteSubmission("p", s.Generation, "ANSWER: late", nil, nil); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	s, err = ReserveSubmission("p", "retry", "")
	if err != nil {
		t.Fatal(err)
	}
	s, err = CompleteSubmission("p", s.Generation, "", errors.New("model unavailable"), nil)
	if err != nil || s.Status != "failed" {
		t.Fatal(s, err)
	}
	path := filepath.Join(sharedctx.Dir("p"), "conversation", "state.json")
	if err = os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadConversation("p"); err == nil {
		t.Fatal("corrupt state accepted")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "broken" {
		t.Fatal("corruption overwritten")
	}
}

func TestConversationBoundedAndUncertain(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	for range 40 {
		s, e := ReserveSubmission("p", "question", "")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = CompleteSubmission("p", s.Generation, "ANSWER: fact", nil, nil); e != nil {
			t.Fatal(e)
		}
	}
	s, e := LoadConversation("p")
	if e != nil || len(s.Messages) != historyLimit {
		t.Fatal(len(s.Messages), e)
	}
	s, e = ReserveSubmission("p", "!reload", "")
	if e != nil {
		t.Fatal(e)
	}
	s, e = CompleteSubmission("p", s.Generation, "CMD: tagent reload", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ClaimApproval("p", s.Revision); e != nil {
		t.Fatal(e)
	}
	_, e = conversationUpdate("p", func(s *Conversation) error { s.Deadline = time.Now().Add(-time.Second); return nil })
	if e != nil {
		t.Fatal(e)
	}
	s, e = LoadConversation("p")
	if e != nil || s.Status != "uncertain" {
		t.Fatal(s, e)
	}
	if _, e = ReserveSubmission("p", "!reload", ""); e == nil {
		t.Fatal("uncertain mutation replayed")
	}
}

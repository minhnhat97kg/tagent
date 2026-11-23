package tmux

import (
	"testing"
	"time"
)

func TestPiEditorContains(t *testing.T) {
	for _, tc := range []struct {
		name, screen, text string
		want               bool
	}{
		{"pending wrapped report", "history\n─────\n hello\n report /tmp/result\n─────\nmodel", "hello\nreport /tmp/result", true},
		{"already submitted", "hello\n─────\n\n─────\nmodel", "hello", false},
		{"different input", "─────\nhello user\n─────", "hello", false},
		{"no editor", "hello", "hello", false},
		{"empty", "─────\n\n─────", "", false},
		{"scrolled long prompt", "─────\n──── ↑ 3 more ────\nthen rename it to the final report file.\n─────\nmodel", "intro line\nmore text\nthen rename it to the final report file.", true},
		{"scrolled other text", "─────\n──── ↑ 3 more ────\nsomething typed by the user here\n─────", "intro\nthen rename it to the final report file.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := piEditorContains(tc.screen, tc.text); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

type submitRunner struct {
	*Fake
	enters int
}

func (r *submitRunner) Run(args ...string) (string, error) {
	if len(args) > 0 && args[0] == "capture-pane" {
		if r.enters == 0 {
			return "─────\nhello\n─────", nil
		}
		return "hello\n─────\n\n─────", nil
	}
	if len(args) > 0 && args[0] == "send-keys" {
		r.enters++
		return "", nil
	}
	return r.Fake.Run(args...)
}

func TestRetryPiSubmit(t *testing.T) {
	for _, status := range []string{"idle", "running", "waiting"} {
		t.Run(status, func(t *testing.T) {
			r := &submitRunner{Fake: NewFake()}
			r.Set("pi", "show", "-t", "%1", "-pv", PaneAgent)
			r.Set(status, "show", "-t", "%1", "-pv", PaneStatusOpt)
			defer SetRunner(r)()
			retryPiSubmit("%1", "hello", time.Second, time.Millisecond)
			want := 0
			if status == "idle" {
				want = 1
			}
			if r.enters != want {
				t.Fatalf("Enter count = %d, want %d", r.enters, want)
			}
		})
	}
}

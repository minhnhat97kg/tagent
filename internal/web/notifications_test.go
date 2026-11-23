package web

import (
	"net/http/httptest"
	"strings"
	"tagent/internal/tmux"
	"testing"
)

func TestDismissFinishedAgent(t *testing.T) {
	fake := tmux.NewFake()
	restore := tmux.SetRunner(fake)
	defer restore()
	fake.Set("claude", "show", "-t", "%1", "-pv", tmux.PaneAgent)
	fake.Set("", "set", "-t", "%1", "-p", "-u", tmux.PaneUnseen)
	s := server{}
	w := httptest.NewRecorder()
	s.agentAction(w, httptest.NewRequest("POST", "/api/pane/action", strings.NewReader(`{"pane":"%1","data":"dismiss"}`)))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	found := false
	for _, c := range fake.Calls() {
		if strings.Join(c, " ") == "set -t %1 -p -u "+tmux.PaneUnseen {
			found = true
		}
	}
	if !found {
		t.Fatal("dismiss did not clear the TUI unseen flag")
	}
}

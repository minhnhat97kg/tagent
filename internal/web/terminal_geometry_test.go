package web

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"tagent/internal/tmux"
)

// Ownership is resent only when it changes: an idle viewer gets the first tick and nothing after it.
func TestTerminalOwnershipSentOnChangeOnly(t *testing.T) {
	_, r, _, _ := startBench(t, "geo-idle", "stty raw -echo; echo READY; exec cat")
	time.Sleep(2500 * time.Millisecond)
	count := 0
	for drained := false; !drained; {
		select {
		case m := <-r.in:
			if m.kind == websocket.TextMessage && strings.Contains(string(m.data), `"type":"ownership"`) {
				count++
			}
		default:
			drained = true
		}
	}
	if count != 1 {
		t.Fatalf("idle viewer got %d ownership messages in 2.5s, want 1 (the first tick)", count)
	}
}

// Handoff is always answered, and a release sent right before the socket closes (pagehide) still
// gives the pane back.
func TestTerminalHandoffAnsweredAndReleasedOnClose(t *testing.T) {
	_, r, conn, pane := startBench(t, "geo-handoff", "stty raw -echo; echo READY; exec cat")
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"handoff","cols":100,"rows":30}`)); err != nil {
		t.Fatal(err)
	}
	waitOwnership(t, r, true)
	if !strings.HasPrefix(tmux.GetPaneOptionValue(pane, "@agent_size_owner"), "web:") {
		t.Fatal("handoff did not claim the pane")
	}
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"release"}`))
	_ = conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	for tmux.GetPaneOptionValue(pane, "@agent_size_owner") != "" {
		if time.Now().After(deadline) {
			t.Fatal("release before close did not give the pane back")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitOwnership reads until an ownership message with the wanted owner arrives.
func waitOwnership(t *testing.T, r *wsReader, owner bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case m, open := <-r.in:
			if !open {
				t.Fatal("socket closed")
			}
			if m.kind != websocket.TextMessage || !strings.Contains(string(m.data), `"type":"ownership"`) {
				continue
			}
			var f struct {
				Owner bool `json:"owner"`
			}
			if json.Unmarshal(m.data, &f) == nil && f.Owner == owner {
				return
			}
		case <-deadline:
			t.Fatalf("no ownership=%v message", owner)
		}
	}
}

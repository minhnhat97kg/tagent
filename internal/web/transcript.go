package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"tagent/internal/queue"
	"tagent/internal/tmux"
	"tagent/internal/transcript"
)

type transcriptJSON struct {
	Harness     string            `json:"harness"`
	Status      string            `json:"status"`
	Items       []transcript.Item `json:"items"`
	Cursor      string            `json:"cursor"`
	Reset       bool              `json:"reset"`
	More        bool              `json:"more"`
	Unsupported bool              `json:"unsupported,omitempty"`
	Unreadable  bool              `json:"unreadable,omitempty"`
	Queued      []string          `json:"queued"`
	Screen      string            `json:"screen,omitempty"`
}

// transcript returns an agent's conversation (active branch) after ?after=<cursor>.
func (s *server) transcript(w http.ResponseWriter, r *http.Request) {
	pane := r.PathValue("pane")
	if err := agentPane(pane); err != nil {
		fail(w, err)
		return
	}
	status := string(tmux.ParsePaneStatus(tmux.GetPaneOptionValue(pane, tmux.PaneStatusOpt)))
	out := transcriptJSON{Harness: tmux.GetPaneOptionValue(pane, tmux.PaneAgent), Status: status, Items: []transcript.Item{}, Queued: append([]string{}, queue.List(pane)...)}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	page, err := transcript.Read(ctx, out.Harness, tmux.GetPaneOptionValue(pane, tmux.PaneSessionID), r.URL.Query().Get("after"))
	switch {
	case errors.Is(err, transcript.ErrUnsupported):
		out.Unsupported = true
	case err != nil:
		out.Unreadable = true
	default:
		out.Items, out.Cursor, out.Reset, out.More = page.Items, page.Cursor, page.Reset, page.More
	}
	if status == string(tmux.StatusWaiting) {
		// The approval itself is answered in Terminal; Chat only shows what is being asked.
		screen, _ := tmux.CaptureTail(pane, 40)
		lines := strings.Split(strings.TrimRight(screen, "\n"), "\n")
		if len(lines) > 30 {
			lines = lines[len(lines)-30:]
		}
		out.Screen = strings.Join(lines, "\n")
	}
	writeJSON(w, out)
}

// send types a message into the agent, or queues it while the agent is busy or waiting
// (the same rule as `tagent send` without --now).
func (s *server) send(w http.ResponseWriter, r *http.Request) {
	p, ok := s.paneBody(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(p.Data) == "" {
		fail(w, fmt.Errorf("empty message"))
		return
	}
	unlock := lockPaneInput(p.Pane)
	defer unlock()
	status := tmux.ParsePaneStatus(tmux.GetPaneOptionValue(p.Pane, tmux.PaneStatusOpt))
	if queue.Busy(status) {
		if _, _, err := queue.Push(p.Pane, p.Data); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"queued": true})
		return
	}
	if _, alive := tmux.PaneSessionName(p.Pane); !alive {
		fail(w, fmt.Errorf("no pane %q", p.Pane))
		return
	}
	tmux.TypeLine(p.Pane, p.Data)
	writeJSON(w, map[string]bool{"queued": false})
}

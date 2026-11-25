package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"tagent/internal/tmux"
)

var viewerID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

func terminalOrigin(r *http.Request) bool {
	u, err := url.Parse(r.Header.Get("Origin"))
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return err == nil && u.Scheme == scheme && u.Host == r.Host && u.User == nil
}

// tmux escapes control bytes and backslashes as three octal digits.
func decodeOutput(s string) ([]byte, error) {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			out = append(out, s[i])
			continue
		}
		if i+3 >= len(s) {
			return nil, fmt.Errorf("short output escape")
		}
		for _, c := range []byte(s[i+1 : i+4]) {
			if c < '0' || c > '7' {
				return nil, fmt.Errorf("invalid output escape")
			}
		}
		n, err := strconv.ParseUint(s[i+1:i+4], 8, 8)
		if err != nil {
			return nil, err
		}
		out = append(out, byte(n))
		i += 3
	}
	return out, nil
}

// geoRequest is a client action for the geometry worker, applied in arrival order.
type geoRequest struct {
	action string // release, handoff, resize, ready
	size   terminalSize
	shot   paneShot
}

// geoEvent is one message for the socket, made by the geometry worker. gone ends the connection.
type geoEvent struct {
	msg  any
	gone bool
}

// ownershipState is what the client shows for the size vote; the tick resends only when it changes.
type ownershipState struct {
	owner, automatic bool
	cols, rows       int
}

func ownershipOf(f paneShot) ownershipState {
	return ownershipState{f.Resizable, f.Automatic, f.Cols, f.Rows}
}

func (s *server) paneWS(w http.ResponseWriter, r *http.Request) {
	if !terminalOrigin(r) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	pane := r.URL.Query().Get("pane")
	if err := agentPane(pane); err != nil {
		fail(w, err)
		return
	}
	session, ok := tmux.RunTmux("display-message", "-p", "-t", pane, "#{session_id}")
	if !ok {
		fail(w, fmt.Errorf("pane unavailable"))
		return
	}
	cmd := exec.CommandContext(r.Context(), "tmux", "-C", "attach-session", "-f", "ignore-size,no-output", "-t", strings.TrimSpace(session))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fail(w, err)
		return
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fail(w, err)
		return
	}
	if err = cmd.Start(); err != nil {
		fail(w, err)
		return
	}
	defer func() { stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	conn, err := (&websocket.Upgrader{CheckOrigin: terminalOrigin, EnableCompression: true}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	// The browser keeps one viewer id per device, so a reload or reconnect keeps its Handoff: the latest
	// Handoff wins and lasts until another device takes over, Release, or the TUI/CLI hands it back.
	viewer := r.URL.Query().Get("viewer")
	if !viewerID.MatchString(viewer) {
		viewer = NewToken()
	}
	released := false
	hadOwnership := false
	conn.SetReadLimit(65536)
	// Only this handler writes to the socket. Keys go to the input goroutine through a bounded
	// queue, text messages to this loop. Reader results are backpressured.
	type clientMessage struct {
		kind int
		data []byte
	}
	keys := make(chan []byte, 16)
	messages := make(chan clientMessage)
	done := make(chan struct{})
	stop := make(chan struct{})
	inputDone := make(chan struct{})
	inputStarted := false
	acks := make(chan bool, 32)
	geoReqs := make(chan geoRequest, 64)
	geoEvents := make(chan geoEvent, 64)
	geoDone := make(chan struct{})
	geoStarted := false
	defer func() {
		close(done)
		_ = conn.Close()
		// Keys already read still run, for at most 2s, before tmux stdin is closed.
		if inputStarted {
			select {
			case <-inputDone:
			case <-time.After(2 * time.Second):
			}
		}
		close(stop)
		// Queued geometry requests (a release from pagehide) still apply, for at most 2s.
		if geoStarted {
			close(geoReqs)
			select {
			case <-geoDone:
			case <-time.After(2 * time.Second):
			}
		}
	}()
	go func() {
		defer close(messages)
		defer close(keys)
		for {
			kind, data, e := conn.ReadMessage()
			if e != nil {
				return
			}
			if kind == websocket.BinaryMessage {
				select {
				case keys <- data:
				case <-done:
					return
				}
				continue
			}
			select {
			case messages <- clientMessage{kind, data}:
			case <-done:
				return
			}
		}
	}()
	reader := &controlReader{pane: pane}
	events := make(chan controlEvent, 64)
	go reader.run(stdout, events, done)
	// Pane output waits in out for at most outputDelay. Every other write flushes it first, so frames
	// keep the order of snapshot, output and gone.
	var out outputBuffer
	flushTimer := time.NewTimer(time.Hour)
	flushTimer.Stop()
	var flushC <-chan time.Time
	flushOut := func() error {
		if flushC != nil {
			if !flushTimer.Stop() {
				select {
				case <-flushTimer.C:
				default:
				}
			}
			flushC = nil
		}
		data := out.take()
		if len(data) == 0 {
			return nil
		}
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		conn.EnableWriteCompression(len(data) >= compressFrom)
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}
	write := func(v any) error {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if err := flushOut(); err != nil {
			return err
		}
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		conn.EnableWriteCompression(len(data) >= compressFrom)
		return conn.WriteMessage(websocket.TextMessage, data)
	}
	// sendGeo queues a geometry request. While it waits it still writes geometry events, so the worker
	// never blocks on a full geoEvents while this loop blocks on a full geoReqs. False ends the connection.
	sendGeo := func(req geoRequest) bool {
		for {
			select {
			case geoReqs <- req:
				return true
			case e := <-geoEvents:
				if write(e.msg) != nil || e.gone {
					return false
				}
			case <-geoDone:
				// The worker has ended; its last events are still buffered.
				for len(geoEvents) > 0 {
					if write((<-geoEvents).msg) != nil {
						break
					}
				}
				return false
			}
		}
	}
	geometry := func() (paneShot, bool) {
		info, ok := tmux.RunTmux("display-message", "-p", "-t", pane, "#{pane_width} #{pane_height} #{cursor_x} #{cursor_y} #{cursor_flag} #{alternate_on} #{mouse_any_flag} #{mouse_sgr_flag}")
		var f paneShot
		f.Automatic = sessionResizable(strings.TrimSpace(session))
		f.Resizable = s.sizeOwners.owns(pane, viewer)
		if f.Resizable {
			hadOwnership = true
		} else if hadOwnership {
			released = true
		}
		var cursor, alternate, mouse, sgr int
		_, err := fmt.Sscan(info, &f.Cols, &f.Rows, &f.CX, &f.CY, &cursor, &alternate, &mouse, &sgr)
		f.Alternate = alternate == 1
		// The app (Claude, pi...) reads the mouse itself: the client forwards wheel/swipe as SGR mouse events.
		f.Mouse = mouse == 1 && sgr == 1
		f.Cursor = cursor == 1
		return f, ok && err == nil
	}
	// sendKeys writes the keys as send-keys blocks on the control client's stdin, in order, and
	// returns once each block has ended. The pane lock covers the whole message.
	sendKeys := func(data []byte) bool {
		unlock := lockPaneInput(pane)
		defer unlock()
		for start := 0; start < len(data); start += 1024 {
			var b strings.Builder
			b.WriteString("send-keys -t " + pane + " -H")
			for _, c := range data[start:min(start+1024, len(data))] {
				fmt.Fprintf(&b, " %02x", c)
			}
			b.WriteByte('\n')
			c := newControlCmd()
			reader.expect(c)
			if _, err := io.WriteString(stdin, b.String()); err != nil {
				return false
			}
			select {
			case <-c.done:
			case <-stop:
				return false
			}
			if !c.ok {
				return false
			}
		}
		return true
	}
	// One goroutine is the only stdin writer after setup. A failed block ends it: nothing runs after a failure.
	inputLoop := func() {
		defer close(inputDone)
		for data := range keys {
			ok := sendKeys(data)
			select {
			case acks <- ok:
			case <-stop:
				return
			}
			if !ok {
				return
			}
		}
	}
	shot, ok := geometry()
	if !ok {
		_ = write(map[string]string{"type": "gone"})
		return
	}
	// Geometry and ownership run in one worker, so tmux queries never hold up output. It owns the
	// release, handoff and resize state from here on; this loop never calls geometry itself.
	geoStarted = true
	go func() {
		defer close(geoDone)
		emit := func(e geoEvent) bool {
			select {
			case geoEvents <- e:
				return true
			case <-done:
				return false
			}
		}
		errorEvent := func(err error) geoEvent {
			return geoEvent{msg: map[string]string{"type": "error", "message": err.Error()}}
		}
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		var pending terminalSize
		resizeTimer := time.NewTimer(time.Hour)
		resizeTimer.Stop()
		var resizeC <-chan time.Time
		ready := false
		var last ownershipState
		sentOwner := false
		var geoShot paneShot
		owner := func(f paneShot) bool {
			last, sentOwner = ownershipOf(f), true
			return emit(geoEvent{msg: map[string]any{"type": "ownership", "owner": f.Resizable, "automatic": f.Automatic, "cols": f.Cols, "rows": f.Rows}})
		}
		for {
			select {
			case req, open := <-geoReqs:
				if !open {
					return
				}
				switch req.action {
				case "ready":
					ready, geoShot = true, req.shot
				case "release":
					released = true
					resizeC = nil
					if e := s.sizeOwners.release(pane, viewer); e != nil && !emit(errorEvent(e)) {
						return
					}
					f, _ := geometry() // Ownership is always answered, even when unchanged.
					if !owner(f) {
						return
					}
				case "handoff":
					released = false
					resizeC = nil
					if e := s.sizeOwners.resize(pane, viewer, req.size, true, false); e != nil && !emit(errorEvent(e)) {
						return
					}
					f, _ := geometry()
					if !owner(f) {
						return
					}
				case "resize":
					pending = req.size
					if !resizeTimer.Stop() {
						select {
						case <-resizeTimer.C:
						default:
						}
					}
					resizeTimer.Reset(150 * time.Millisecond)
					resizeC = resizeTimer.C
				}
			case <-resizeC:
				resizeC = nil
				_, _ = geometry() // Notice external CLI/TUI ownership changes before considering auto-claim.
				if ready {
					if e := s.sizeOwners.resize(pane, viewer, pending, pending.Action == "handoff", !released && sessionResizable(strings.TrimSpace(session))); e != nil && !emit(errorEvent(e)) {
						return
					}
					f, _ := geometry()
					if !owner(f) {
						return
					}
				}
				pending.Action = "resize"
			case <-tick.C:
				f, ok := geometry()
				if !ok {
					emit(geoEvent{msg: map[string]string{"type": "gone"}, gone: true})
					return
				}
				if ready {
					if f.Resizable {
						s.sizeOwners.keepZoomed(pane, viewer)
					}
					if !sentOwner || ownershipOf(f) != last {
						if !owner(f) {
							return
						}
					}
				}
				if ready && (f.Cols != geoShot.Cols || f.Rows != geoShot.Rows || f.Resizable != geoShot.Resizable) {
					geoShot = f
					if !emit(geoEvent{msg: map[string]any{"type": "resize", "cols": f.Cols, "rows": f.Rows, "resizable": f.Resizable}}) {
						return
					}
				}
			}
		}
	}()
	// Subscription and capture are sent after the attach block; the capture reply is an
	// ordered snapshot barrier relative to %output.
	ready := false
	var refreshCmd, snapCmd *controlCmd
	for {
		select {
		case ev := <-events:
			switch ev.kind {
			case controlEnd:
				_ = write(map[string]string{"type": "gone"})
				return
			case controlFail:
				return
			case controlAttached:
				refreshCmd, snapCmd = newControlCmd(), newControlCmd()
				reader.expect(refreshCmd)
				if _, err = fmt.Fprintf(stdin, "refresh-client -f ignore-size,!no-output -A '%s:on'\n", pane); err != nil {
					return
				}
				reader.expect(snapCmd)
				if _, err = fmt.Fprintf(stdin, "capture-pane -e -p -S -2000 -t %s\n", pane); err != nil {
					return
				}
				inputStarted = true
				go inputLoop()
			case controlCmdDone:
				// Input replies are judged by inputLoop, which acks them; only setup failures close here.
				if (ev.cmd == refreshCmd || ev.cmd == snapCmd) && !ev.cmd.ok {
					return
				}
				if ev.cmd == snapCmd {
					shot.Screen = strings.Join(ev.cmd.out, "\n")
					if write(struct {
						Type string `json:"type"`
						paneShot
					}{"snapshot", shot}) != nil {
						return
					}
					ready = true
					if !sendGeo(geoRequest{action: "ready", shot: shot}) {
						return
					}
				}
			case controlOutput:
				if !ready {
					break
				}
				if out.add(ev.output, time.Now(), len(events)) {
					if flushOut() != nil {
						return
					}
				} else if flushC == nil {
					flushTimer.Reset(outputDelay)
					flushC = flushTimer.C
				}
			}
		case <-flushC:
			flushC = nil
			if flushOut() != nil {
				return
			}
		case ok := <-acks:
			if write(map[string]any{"type": "ack", "ok": ok}) != nil || !ok {
				return
			}
		case message, open := <-messages:
			if !open {
				return
			}
			size, e := parseTerminalResize(message.data)
			if e != nil {
				continue
			}
			if !sendGeo(geoRequest{action: size.Action, size: size}) {
				return
			}
		case g := <-geoEvents:
			if write(g.msg) != nil || g.gone {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

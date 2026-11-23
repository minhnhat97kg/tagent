package web

import (
	"bufio"
	"fmt"
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
	conn, err := (&websocket.Upgrader{CheckOrigin: terminalOrigin}).Upgrade(w, r, nil)
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
	// Only this handler writes to the socket. Reader results are backpressured.
	type clientMessage struct {
		kind int
		data []byte
	}
	inputs := make(chan clientMessage)
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer close(inputs)
		for {
			kind, data, e := conn.ReadMessage()
			if e != nil {
				return
			}
			select {
			case inputs <- clientMessage{kind, data}:
			case <-done:
				return
			}
		}
	}()
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 4<<20)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-done:
				return
			}
		}
	}()
	write := func(v any) error {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(v)
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
	writeOwner := func(f paneShot) error {
		return write(map[string]any{"type": "ownership", "owner": f.Resizable, "automatic": f.Automatic, "cols": f.Cols, "rows": f.Rows})
	}
	shot, ok := geometry()
	if !ok {
		_ = write(map[string]string{"type": "gone"})
		return
	}
	// Attach, subscription and capture each have a control response block. The
	// capture response is an ordered snapshot barrier relative to %output.
	block := 0
	capturing := false
	ready := false
	var screen []string
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var pending terminalSize
	resizeTimer := time.NewTimer(time.Hour)
	if !resizeTimer.Stop() {
		<-resizeTimer.C
	}
	defer resizeTimer.Stop()
	var resizeC <-chan time.Time
	for {
		select {
		case line, open := <-lines:
			if !open {
				_ = write(map[string]string{"type": "gone"})
				return
			}
			if strings.HasPrefix(line, "%begin ") {
				block++
				capturing = block == 3
				continue
			}
			if strings.HasPrefix(line, "%error ") {
				return
			}
			if strings.HasPrefix(line, "%end ") {
				if block == 1 {
					if _, err = fmt.Fprintf(stdin, "refresh-client -f ignore-size,!no-output -A '%s:on'\ncapture-pane -e -p -S -2000 -t %s\n", pane, pane); err != nil {
						return
					}
				}
				if capturing {
					shot.Screen = strings.Join(screen, "\n")
					if write(struct {
						Type string `json:"type"`
						paneShot
					}{"snapshot", shot}) != nil {
						return
					}
					ready = true
					capturing = false
				}
				continue
			}
			if capturing {
				screen = append(screen, line)
				continue
			}
			if ready && strings.HasPrefix(line, "%output "+pane+" ") {
				data, e := decodeOutput(strings.TrimPrefix(line, "%output "+pane+" "))
				if e != nil {
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if conn.WriteMessage(websocket.BinaryMessage, data) != nil {
					return
				}
			}
		case message, open := <-inputs:
			if !open {
				return
			}
			if message.kind == websocket.TextMessage {
				size, e := parseTerminalResize(message.data)
				if e != nil {
					continue
				}
				if size.Action == "release" {
					released = true
					resizeC = nil
					if e := s.sizeOwners.release(pane, viewer); e != nil {
						_ = write(map[string]string{"type": "error", "message": e.Error()})
					}
					f, _ := geometry()
					_ = writeOwner(f)
					continue
				}
				if size.Action == "handoff" {
					released = false
					resizeC = nil
					if e := s.sizeOwners.resize(pane, viewer, size, true, false); e != nil {
						_ = write(map[string]string{"type": "error", "message": e.Error()})
					}
					f, _ := geometry()
					_ = writeOwner(f)
					continue
				}
				pending = size
				if !resizeTimer.Stop() {
					select {
					case <-resizeTimer.C:
					default:
					}
				}
				resizeTimer.Reset(150 * time.Millisecond)
				resizeC = resizeTimer.C
				continue
			}
			data := message.data
			unlock := lockPaneInput(pane)
			args := []string{"send-keys", "-t", pane, "-H"}
			for _, b := range data {
				args = append(args, fmt.Sprintf("%02x", b))
			}
			_, ok := tmux.RunTmux(args...)
			unlock()
			if write(map[string]any{"type": "ack", "ok": ok}) != nil || !ok {
				return
			}
		case <-resizeC:
			resizeC = nil
			_, _ = geometry() // Notice external CLI/TUI ownership changes before considering auto-claim.
			if ready {
				if e := s.sizeOwners.resize(pane, viewer, pending, pending.Action == "handoff", !released && sessionResizable(strings.TrimSpace(session))); e != nil {
					if write(map[string]string{"type": "error", "message": e.Error()}) != nil {
						return
					}
				}
				f, _ := geometry()
				if writeOwner(f) != nil {
					return
				}
			}
			pending.Action = "resize"
		case <-tick.C:
			f, ok := geometry()
			if !ok {
				_ = write(map[string]string{"type": "gone"})
				return
			}
			if ready {
				if writeOwner(f) != nil {
					return
				}
			}
			if ready && (f.Cols != shot.Cols || f.Rows != shot.Rows || f.Resizable != shot.Resizable) {
				shot = f
				if write(map[string]any{"type": "resize", "cols": f.Cols, "rows": f.Rows, "resizable": f.Resizable}) != nil {
					return
				}
			}
		case <-r.Context().Done():
			return
		}
	}
}

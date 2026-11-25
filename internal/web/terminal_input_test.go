package web

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Keys typed back-to-back must reach the pane byte for byte: `cat > file` in raw mode writes exactly
// what the pane received.
func TestTerminalInputBackToBackByteExact(t *testing.T) {
	file := filepath.Join(t.TempDir(), "in.bin")
	_, r, conn, _ := startBench(t, "input-exact", "stty raw -echo; echo READY; exec cat > "+file)
	var want []byte
	var msgs [][]byte
	for i := 0; i < 200; i++ {
		msgs = append(msgs, []byte{byte('a' + i%26)})
	}
	msgs = append(msgs,
		[]byte("tiếng Việt: ệ ơ ư ắ ộ đ — Telex ✓\n"),
		bytes.Repeat([]byte("0123456789abcdef"), 300),                          // about 4.8KB paste
		bytes.Repeat([]byte{0x00, 0x1b, 0x7f, 0xff, '\\', '%'}, 11000)[:65000], // near the 64KB read limit
	)
	for _, m := range msgs {
		want = append(want, m...)
		if err := conn.WriteMessage(websocket.BinaryMessage, m); err != nil {
			t.Fatal(err)
		}
	}
	r.until(func() bool { return r.acks == len(msgs) })
	waitFile(t, file, want)
}

// Input typed while the pane floods output: no deadlock, and output keeps its order.
func TestTerminalInputInterleavedWithHeavyOutput(t *testing.T) {
	file := filepath.Join(t.TempDir(), "in.bin")
	const lines = 3000
	// The flood waits for a gate file created after the snapshot, so every line reaches the socket.
	gate := filepath.Join(t.TempDir(), "go")
	flood := fmt.Sprintf("(while [ ! -e '%s' ]; do sleep 0.01; done; i=0; while [ $i -lt %d ]; do printf '%%06d %s\\r\\n' $i; i=$((i+1)); done) & exec cat > %s",
		gate, lines, strings.Repeat("x", 60), file)
	_, r, conn, _ := startBench(t, "input-flood", "stty raw -echo; echo READY; "+flood)
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var want []byte
	for i := 0; i < 50; i++ {
		m := bytes.Repeat([]byte{byte('A' + i%26)}, 1000)
		want = append(want, m...)
		if err := conn.WriteMessage(websocket.BinaryMessage, m); err != nil {
			t.Fatal(err)
		}
	}
	r.until(func() bool { return r.acks == 50 && strings.Contains(string(r.echo), fmt.Sprintf("%06d ", lines-1)) })
	waitFile(t, file, want)
	// Output lines are consecutive: a reordered or dropped chunk would break the numbering.
	next := -1
	for _, line := range strings.Split(string(r.echo), "\r\n") {
		var n int
		if _, err := fmt.Sscanf(line, "%06d", &n); err != nil {
			continue
		}
		if next >= 0 && n != next {
			t.Fatalf("output reordered: line %d after %d", n, next-1)
		}
		next = n + 1
	}
	if next != lines {
		t.Fatalf("output ended at line %d", next)
	}
}

// A pane that disappears mid-input acks ok:false (or the socket closes), and nothing after that runs.
func TestTerminalInputFailureStopsAndCloses(t *testing.T) {
	file := filepath.Join(t.TempDir(), "in.bin")
	_, r, conn, pane := startBench(t, "input-fail", "stty raw -echo; echo READY; exec cat > "+file)
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("first")); err != nil {
		t.Fatal(err)
	}
	r.until(func() bool { return r.acks == 1 })
	if out, err := exec.Command("tmux", "kill-pane", "-t", pane).CombinedOutput(); err != nil {
		t.Fatalf("kill-pane: %v %s", err, out)
	}
	_ = conn.WriteMessage(websocket.BinaryMessage, []byte("second"))
	deadline := time.After(20 * time.Second)
	failed := false
	for !failed {
		select {
		case m, open := <-r.in:
			if !open {
				failed = true
				break
			}
			if m.kind != websocket.TextMessage {
				continue
			}
			s := string(m.data)
			if strings.Contains(s, `"ok":true`) {
				t.Fatalf("input acked ok after the pane was killed: %s", s)
			}
			if strings.Contains(s, `"ok":false`) || strings.Contains(s, `"type":"gone"`) {
				failed = true
			}
		case <-deadline:
			t.Fatal("no failure ack or close after the pane was killed")
		}
	}
	if got, _ := os.ReadFile(file); strings.Contains(string(got), "second") {
		t.Fatalf("input after the pane was killed was delivered: %q", got)
	}
}

// HTTP and two WebSocket writers to one pane never interleave inside a message.
func TestTerminalInputSerializedAcrossTransports(t *testing.T) {
	file := filepath.Join(t.TempDir(), "in.bin")
	_, r, conn, pane, addr := startBenchAddr(t, "input-mixed", "stty raw -echo; echo READY; exec cat > "+file)
	r2, conn2 := dialViewer(t, addr, pane, "benchviewer2")
	const size, httpSize, count = 3000, 900, 20
	h := (&server{token: "secret"}).handler()
	var wg sync.WaitGroup
	wg.Add(3)
	send := func(c *websocket.Conn, letter string) {
		defer wg.Done()
		for i := 0; i < count; i++ {
			if err := c.WriteMessage(websocket.BinaryMessage, []byte(strings.Repeat(letter, size))); err != nil {
				t.Errorf("ws write: %v", err)
				return
			}
		}
	}
	go send(conn, "A")
	go send(conn2, "B")
	go func() {
		defer wg.Done()
		for i := 0; i < count; i++ {
			body := fmt.Sprintf(`{"pane":%q,"data":%q}`, pane, strings.Repeat("C", httpSize))
			req := httptest.NewRequest("POST", "http://example.com/api/pane/input", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://example.com")
			req.AddCookie(&http.Cookie{Name: "tmux_agent", Value: "secret"})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("http input: %d %s", rec.Code, rec.Body.String())
			}
		}
	}()
	wg.Wait()
	r.until(func() bool { return r.acks == count })
	r2.until(func() bool { return r2.acks == count })
	got := waitLen(t, file, 2*size*count+httpSize*count)
	// Each message is one letter repeated; a split message would show a run that stops early.
	for off := 0; off < len(got); {
		n := size
		if got[off] == 'C' {
			n = httpSize
		}
		if off+n > len(got) || bytes.Count(got[off:off+n], got[off:off+1]) != n {
			t.Fatalf("messages interleaved at byte %d: %q", off, got[off:min(off+16, len(got))])
		}
		off += n
	}
}

func waitFile(t *testing.T, file string, want []byte) {
	t.Helper()
	if got := waitLen(t, file, len(want)); !bytes.Equal(got, want) {
		t.Fatalf("pane received %d bytes, not the %d sent in order", len(got), len(want))
	}
}

// waitLen polls the file the pane writes until it holds n bytes, then returns them.
func waitLen(t *testing.T, file string, n int) []byte {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		got, _ := os.ReadFile(file)
		if len(got) >= n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A burst of release messages fills the geometry request queue while the worker has events to send.
// Every release is answered with ownership, and the connection keeps working.
func TestTerminalGeometryFloodDoesNotDeadlock(t *testing.T) {
	skipUnlessFlaky(t)
	_, r, conn, _ := startBench(t, "geo-flood", "stty raw -echo; echo READY; exec cat")
	const releases = 300
	for i := 0; i < releases; i++ {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"release"}`)); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.After(30 * time.Second)
	got := 0
	for got < releases {
		select {
		case m, open := <-r.in:
			if !open {
				t.Fatalf("socket closed after %d of %d ownership replies", got, releases)
			}
			if m.kind == websocket.TextMessage && strings.Contains(string(m.data), `"type":"ownership"`) {
				got++
			}
		case <-deadline:
			t.Fatalf("deadlock: %d of %d ownership replies", got, releases)
		}
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("ok")); err != nil {
		t.Fatal(err)
	}
	r.until(func() bool { return r.acks == 1 })
}

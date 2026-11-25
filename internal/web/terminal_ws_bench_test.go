package web

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"tagent/internal/tmux"
)

// Env-gated measurement (TAGENT_WS_BENCH=1): the real handler on the private tmux server, the
// WebSocket behind a proxy that adds 50ms each way. Logs numbers; it asserts only correctness.
const benchKeys = 100

type wireCounter struct{ bytes atomic.Int64 }

type countListener struct {
	net.Listener
	c *wireCounter
}

func (l countListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return countConn{c, l.c}, nil
}

type countConn struct {
	net.Conn
	c *wireCounter
}

func (c countConn) Write(b []byte) (int, error) {
	c.c.bytes.Add(int64(len(b)))
	return c.Conn.Write(b)
}

func delayedProxy(t *testing.T, target string, oneWay time.Duration) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			up, err := net.Dial("tcp", target)
			if err != nil {
				c.Close()
				continue
			}
			go delayCopy(up, c, oneWay)
			go delayCopy(c, up, oneWay)
		}
	}()
	return l.Addr().String()
}

// delayCopy keeps chunk order and adds a fixed latency without limiting throughput.
func delayCopy(dst, src net.Conn, d time.Duration) {
	type chunk struct {
		at time.Time
		b  []byte
	}
	ch := make(chan chunk, 4096)
	go func() {
		defer close(ch)
		buf := make([]byte, 32<<10)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				ch <- chunk{time.Now().Add(d), append([]byte(nil), buf[:n]...)}
			}
			if err != nil {
				return
			}
		}
	}()
	for c := range ch {
		time.Sleep(time.Until(c.at))
		if _, err := dst.Write(c.b); err != nil {
			break
		}
	}
	dst.Close()
	src.Close()
}

type wsReader struct {
	t        *testing.T
	in       chan wsMsg
	snapshot bool
	acks     int
	echo     []byte
	msgs     int
}

type wsMsg struct {
	kind int
	data []byte
}

func (r *wsReader) until(done func() bool) {
	r.t.Helper()
	timeout := time.After(60 * time.Second)
	for !done() {
		select {
		case m, open := <-r.in:
			if !open {
				r.t.Fatal("socket closed")
			}
			if m.kind == websocket.BinaryMessage {
				r.msgs++
				r.echo = append(r.echo, m.data...)
				continue
			}
			s := string(m.data)
			switch {
			case strings.Contains(s, `"type":"snapshot"`):
				r.snapshot = true
			case strings.Contains(s, `"type":"ack"`):
				if strings.Contains(s, `"ok":false`) {
					r.t.Fatalf("input failed: %s", s)
				}
				r.acks++
			case strings.Contains(s, `"type":"error"`), strings.Contains(s, `"type":"gone"`):
				r.t.Fatalf("server: %s", s)
			}
		case <-timeout:
			_ = pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
			r.t.Fatalf("timeout: acks=%d echo=%d snapshot=%v", r.acks, len(r.echo), r.snapshot)
		}
	}
}

// startBench runs the real handler behind a counting listener and a delayed proxy, then attaches
// one viewer to a fresh pane running command, and waits for the snapshot barrier.
func startBench(t *testing.T, name, command string) (*wireCounter, *wsReader, *websocket.Conn, string) {
	counter, r, conn, pane, _ := startBenchAddr(t, name, command)
	return counter, r, conn, pane
}

func startBenchAddr(t *testing.T, name, command string) (*wireCounter, *wsReader, *websocket.Conn, string, string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux")
	}
	// A private tmux server: the user's sessions are never touched. The path stays short for the socket.
	dir, err := os.MkdirTemp("", "tmx")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	// Cleanups run in reverse: the server is killed with the directory still set, and only then removed.
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Cleanup(func() {
		cmd := exec.Command("tmux", "kill-server")
		cmd.Env = append(os.Environ(), "TMUX_TMPDIR="+dir, "TMUX=")
		_ = cmd.Run()
	})
	counter := &wireCounter{}
	ts := httptest.NewUnstartedServer((&server{token: "secret"}).handler())
	ts.Listener.Close()
	ts.Listener = countListener{mustListen(t), counter}
	ts.Start()
	t.Cleanup(ts.Close)

	out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-x", "120", "-y", "40", "-P", "-F", "#{pane_id}", command).Output()
	if err != nil {
		t.Fatalf("new-session: %v", err)
	}
	pane := strings.TrimSpace(string(out))
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", name).Run() })
	tmux.SetPaneOption(pane, tmux.PaneKeep, "1")
	// Keys sent before the pane program has set raw mode are lost or line-buffered: wait for READY.
	waitPaneText(t, pane, "READY")

	proxy := delayedProxy(t, ts.Listener.Addr().String(), 50*time.Millisecond)
	r, conn := dialViewer(t, proxy, pane, "benchviewer")
	return counter, r, conn, pane, proxy
}

// dialViewer attaches a WebSocket viewer to pane through addr and waits for its snapshot.
func dialViewer(t *testing.T, addr, pane, viewer string) (*wsReader, *websocket.Conn) {
	t.Helper()
	header := http.Header{"Origin": {"http://" + addr}, "Cookie": {"tmux_agent=secret"}}
	// Browsers offer permessage-deflate; the bench does too, so wire bytes match what a browser gets.
	conn, _, err := (&websocket.Dialer{EnableCompression: true}).Dial("ws://"+addr+"/api/pane/ws?pane="+url.QueryEscape(pane)+"&viewer="+viewer, header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	r := &wsReader{t: t, in: make(chan wsMsg, 4096)}
	go func() {
		defer close(r.in)
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			r.in <- wsMsg{kind, data}
		}
	}()
	r.until(func() bool { return r.snapshot })
	return r, conn
}

// waitPaneText polls the visible pane until text appears, so a test types only after its program started.
func waitPaneText(t *testing.T, pane, text string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		out, _ := exec.Command("tmux", "capture-pane", "-p", "-t", pane).Output()
		if strings.Contains(string(out), text) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane never printed %q: %q", text, out)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func mustListen(t *testing.T) net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestTerminalWSBench(t *testing.T) {
	if os.Getenv("TAGENT_WS_BENCH") == "" {
		t.Skip("set TAGENT_WS_BENCH=1 to measure the terminal WebSocket")
	}
	key := func(i int) []byte { return []byte{byte('a' + i%26)} }

	t.Run("echo-stop-and-wait", func(t *testing.T) {
		_, r, conn, _ := startBench(t, "bench-saw", "stty raw -echo; echo READY; exec cat")
		start := time.Now()
		for i := 0; i < benchKeys; i++ {
			if err := conn.WriteMessage(websocket.BinaryMessage, key(i)); err != nil {
				t.Fatal(err)
			}
			r.until(func() bool { return r.acks == i+1 })
		}
		acked := time.Since(start)
		r.until(func() bool { return len(r.echo) >= benchKeys })
		t.Logf("stop-and-wait: %d keys, all echoed %v, last ack %v, echo messages %d", benchKeys, time.Since(start), acked, r.msgs)
		assertEcho(t, r.echo)
	})

	t.Run("echo-pipelined", func(t *testing.T) {
		_, r, conn, _ := startBench(t, "bench-pipe", "stty raw -echo; echo READY; exec cat")
		start := time.Now()
		for i := 0; i < benchKeys; i++ {
			if err := conn.WriteMessage(websocket.BinaryMessage, key(i)); err != nil {
				t.Fatal(err)
			}
		}
		r.until(func() bool { return len(r.echo) >= benchKeys })
		echoed := time.Since(start)
		r.until(func() bool { return r.acks == benchKeys })
		t.Logf("pipelined: %d keys, all echoed %v, all acked %v, echo messages %d", benchKeys, echoed, time.Since(start), r.msgs)
		assertEcho(t, r.echo)
	})

	t.Run("output-burst", func(t *testing.T) {
		dir := t.TempDir()
		script, gate := filepath.Join(dir, "burst.sh"), filepath.Join(dir, "go")
		body := "stty raw -echo; echo READY; while [ ! -e '" + gate + "' ]; do sleep 0.01; done; i=0; while [ $i -lt 3000 ]; do printf '%06d abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ\\r\\n' $i; i=$((i+1)); done; printf 'END\\r\\n'; exec cat\n"
		if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		counter, r, _, pane := startBench(t, "bench-burst", "sh "+script)
		defer func() {
			if t.Failed() {
				out, _ := exec.Command("tmux", "capture-pane", "-p", "-S", "-20", "-t", pane).Output()
				tty, _ := exec.Command("tmux", "display", "-p", "-t", pane, "#{pane_tty}").Output()
				t.Logf("pane screen:\n%s\ntty %s", out, tty)
				ps, _ := exec.Command("sh", "-c", "ps -o pid,stat,command -t "+strings.TrimPrefix(strings.TrimSpace(string(tty)), "/dev/")).Output()
				t.Logf("processes on tty:\n%s", ps)
			}
		}()
		counter.bytes.Store(0)
		r.echo, r.msgs = nil, 0
		start := time.Now()
		// The burst starts after the snapshot: the test creates the gate file, not a key.
		if err := os.WriteFile(gate, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		r.until(func() bool { return strings.Contains(string(r.echo), "END\r\n") })
		t.Logf("burst: %v, %d output messages, %d decoded bytes, %d wire bytes from server", time.Since(start), r.msgs, len(r.echo), counter.bytes.Load())
	})
}

func assertEcho(t *testing.T, got []byte) {
	t.Helper()
	want := make([]byte, benchKeys)
	for i := range want {
		want[i] = byte('a' + i%26)
	}
	if string(got[:benchKeys]) != string(want) {
		t.Fatalf("echo not byte-exact: %q", got)
	}
}

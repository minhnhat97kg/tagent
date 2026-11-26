package web

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSingleInstanceLock(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	lock, _, err := Claim()
	if err != nil || lock == nil {
		t.Fatalf("first claim: %v", err)
	}
	go lock.Serve("127.0.0.1:0", "tok", t.TempDir())
	var held Instance
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, held, _ = Claim(); held.Addr != "" {
			break
		}
	}
	if held.Addr == "" || held.Token != "tok" || held.PID != os.Getpid() {
		t.Fatalf("second claim should report the holder, got %+v", held)
	}
}

func TestStaleLockTakenOver(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	os.WriteFile(filepath.Join(filepath.Dir(lockPath()), "web.lock"), []byte(`{"pid":999999,"addr":"127.0.0.1:1","token":"old"}`), 0600)
	lock, _, err := Claim()
	if err != nil || lock == nil {
		t.Fatalf("a dead holder's lock must be free: %v", err)
	}
	lock.f.Close()
}

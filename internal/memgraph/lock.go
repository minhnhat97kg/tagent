package memgraph

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockTimeout bounds how long a writer waits for the graph lock.
var lockTimeout = 5 * time.Second

// lock takes an exclusive flock on <dir>/.lock and returns the release func.
// It creates the graph dir (0700) only when a write needs it.
func (g *Graph) lock() (func(), error) {
	if err := os.MkdirAll(g.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("graph %s: %w", g.Dir, err)
	}
	f, err := os.OpenFile(filepath.Join(g.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("graph %s: %w", g.Dir, err)
	}
	deadline := time.Now().Add(lockTimeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
			}, nil
		}
		if err != syscall.EWOULDBLOCK || time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("graph %s: timed out waiting for lock", g.Dir)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

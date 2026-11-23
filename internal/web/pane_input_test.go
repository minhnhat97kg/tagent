package web

import (
	"testing"
	"time"
)

func TestPaneInputLock(t *testing.T) {
	unlock := lockPaneInput("test-one")
	entered := make(chan struct{})
	go func() { release := lockPaneInput("test-one"); close(entered); release() }()
	// Other panes remain independent while one pane is sending.
	other := lockPaneInput("test-two")
	other()
	select {
	case <-entered:
		t.Fatal("same-pane input overlapped")
	case <-time.After(20 * time.Millisecond):
	}
	unlock()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("queued input never released")
	}
}

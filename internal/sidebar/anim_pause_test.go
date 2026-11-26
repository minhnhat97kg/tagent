package sidebar

import (
	"testing"

	"tagent/internal/tmux"
)

// A sidebar in a hidden window stops its animation tick and resumes it once
// the window is the active one of an attached session again.
func TestAnimPausesWhileWindowHidden(t *testing.T) {
	sess := []tmux.SessionInfo{{Attached: true, Windows: []tmux.WindowInfo{{WindowID: "@1", WindowActive: false}}}}
	m := Model{windowID: "@1", sessions: sess}
	got, cmd := m.Update(AnimTickMsg{})
	m = got.(Model)
	if cmd == nil || !m.animPaused {
		t.Fatalf("hidden window did not pause: cmd=%v paused=%v", cmd != nil, m.animPaused)
	}
	m.sessions[0].Windows[0].WindowActive = true
	got, _ = m.Update(refreshRequestMsg{})
	m = got.(Model)
	if m.animPaused {
		t.Fatal("visible window did not resume the animation")
	}
}

// Sessions only refresh every hiddenRefreshEvery ticks, so a window that is
// shown again is found by the 1s poll, not by the refresh.
func TestHiddenPollResumesWithinSecond(t *testing.T) {
	defer func(f func(string) bool) { windowVisibleNow = f }(windowVisibleNow)
	hidden := []tmux.SessionInfo{{Attached: true, Windows: []tmux.WindowInfo{{WindowID: "@1", WindowActive: false}}}}
	m := Model{windowID: "@1", sessions: hidden, animPaused: true}

	windowVisibleNow = func(string) bool { return false }
	got, cmd := m.Update(hiddenPollMsg{})
	m = got.(Model)
	if cmd == nil || !m.animPaused {
		t.Fatal("poll on a hidden window must keep polling and stay paused")
	}

	windowVisibleNow = func(string) bool { return true }
	got, cmd = m.Update(hiddenPollMsg{})
	m = got.(Model)
	if m.animPaused || cmd == nil {
		t.Fatalf("poll on a visible window must resume: paused=%v cmd=%v", m.animPaused, cmd != nil)
	}
}

// A hidden window refreshes (forks tmux and ps) only on every
// hiddenRefreshEvery-th tick; a visible one refreshes on each tick.
func TestHiddenWindowSkipsRefreshTicks(t *testing.T) {
	hidden := []tmux.SessionInfo{{Attached: true, Windows: []tmux.WindowInfo{{WindowID: "@1", WindowActive: false}}}}
	m := Model{windowID: "@1", sessions: hidden}
	for i := 0; i < hiddenRefreshEvery-1; i++ {
		got, _ := m.Update(refreshTickMsg{})
		m = got.(Model)
	}
	if m.hiddenTicks != hiddenRefreshEvery-1 {
		t.Fatalf("hidden ticks = %d, want %d", m.hiddenTicks, hiddenRefreshEvery-1)
	}
	m.sessions[0].Windows[0].WindowActive = true
	got, _ := m.Update(refreshTickMsg{})
	if got.(Model).hiddenTicks != hiddenRefreshEvery-1 {
		t.Fatal("visible window counted a hidden tick")
	}
}

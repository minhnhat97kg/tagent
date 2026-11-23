// Package tmux wraps all communication with tmux via subprocess.
//
// All sidebar state lives in tmux (pane options + global options), so this
// package is the sole boundary between the program and tmux. Every command
// goes through a replaceable Runner, so tests don't need a real tmux.
package tmux

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner runs a tmux command and returns stdout.
type Runner interface {
	Run(args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return string(out), nil
}

var current Runner = execRunner{}

// SetRunner replaces the current Runner and returns a restore function. Only
// used in tests:
//
//	defer SetRunner(NewFake())()
//
// Runner is a global variable, so tests using it must not run in parallel.
func SetRunner(r Runner) (restore func()) {
	prev := current
	current = r
	resetRawCache() // cached raw option values belong to the previous runner
	return func() { current = prev; resetRawCache() }
}

// RunTmux runs a tmux command, returning raw stdout and false on error.
//
// Used for the background refresh path: an error here means "no data
// available", not an incident that needs to be reported to the user. A pane
// may have just closed in between two commands.
func RunTmux(args ...string) (string, bool) {
	out, err := current.Run(args...)
	if err != nil {
		return "", false
	}
	return out, true
}

// RunTmuxCapture runs a tmux command, returning trimmed stdout or an error
// with a message.
//
// Used for user-initiated actions (spawn/remove in part 5), where silently
// swallowing the error would leave the user pressing a key without
// understanding why nothing happened.
func RunTmuxCapture(args ...string) (string, error) {
	out, err := current.Run(args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DisplayMessage asks tmux for a format string about a target. Returns empty
// on error.
func DisplayMessage(target, format string) string {
	out, _ := RunTmux("display-message", "-t", target, "-p", format)
	return strings.TrimSpace(out)
}

// PaneSessionName returns the name of the session containing the pane. ok =
// false when tmux can't find the pane (e.g. it was just closed).
func PaneSessionName(paneID string) (string, bool) {
	name := DisplayMessage(paneID, "#{session_name}")
	return name, name != ""
}

// NewWindow creates a new window in the session, returning (paneID, windowID).
//
// windowID is needed by the spawn flow in part 5: the marker is set at the
// window level so that a pane split off from it later (e.g. a Claude Code
// subagent) inherits it.
func NewWindow(session, cwd, name string) (string, string, error) {
	out, err := RunTmuxCapture(
		"new-window", "-d",
		"-t", session+":",
		"-c", cwd,
		"-n", name,
		"-P", "-F", "#{pane_id} #{window_id}",
	)
	if err != nil {
		return "", "", err
	}
	paneID, windowID, ok := strings.Cut(out, " ")
	if !ok {
		return "", "", fmt.Errorf("new-window returned unexpected output: %q", out)
	}
	return paneID, windowID, nil
}

// SetWindowOption sets an option at the window level.
func SetWindowOption(windowID, key, value string) {
	RunTmux("set", "-t", windowID, "-w", key, value)
}

// SendCommand types a command line into a pane followed by Enter.
func SendCommand(target, command string) {
	RunTmux("send-keys", "-t", target, command, "Enter")
}

// KillWindow closes a window.
func KillWindow(windowID string) {
	RunTmux("kill-window", "-t", windowID)
}

// SelectPane switches focus to a pane, even when that pane belongs to a
// different session/window than the current client. The sidebar lists panes
// across the WHOLE tmux server (all sessions, all windows), so "select-pane"
// alone isn't enough: it selects a pane within the client's currently active
// window, but doesn't itself switch the client to the window/session
// containing the target pane. Three steps, in this exact order — ported from
// the reference implementation (tas/src/tmux/commands.rs, function
// select_pane):
// switch-client (session) → select-window (window) → select-pane (pane).
//
// fromPane is the pane the caller is acting FROM (the sidebar's own pane, or
// wherever the CLI command was invoked) — used to resolve WHICH client
// switch-client should move. Real bug, real user report: with several tmux
// clients each attached to a DIFFERENT session, `switch-client -t <session>`
// with no `-c <client>` lets tmux pick an implicit "current client" that can
// be a DIFFERENT client than the one actually looking at fromPane — the
// terminal the user is watching doesn't move, some other terminal jumps
// instead. ClientForPane(fromPane) pins the -c target explicitly so only the
// client that owns fromPane's session is ever moved.
func SelectPane(paneID, fromPane string) {
	if sessionID := DisplayMessage(paneID, "#{session_id}"); sessionID != "" {
		if client := ClientForPane(fromPane); client != "" {
			RunTmux("switch-client", "-c", client, "-t", sessionID)
		} else {
			RunTmux("switch-client", "-t", sessionID)
		}
	}
	if windowID := DisplayMessage(paneID, "#{window_id}"); windowID != "" {
		RunTmux("select-window", "-t", windowID)
	}
	RunTmux("select-pane", "-t", paneID)
}

// ClientForPane returns the tty of a client currently attached to pane's
// session, or "" if none is attached or the lookup fails. When several
// clients are attached to the SAME session (list-clients returns several
// lines), the first one is used — good enough to disambiguate the common
// case SelectPane cares about (several clients, each on a DIFFERENT
// session); picking among several clients on the very same session is a
// rarer setup this doesn't try to get exactly right.
func ClientForPane(pane string) string {
	out, ok := RunTmux("list-clients", "-t", pane, "-F", "#{client_tty}")
	if !ok {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	return line
}

// IsPaneViewed is true when someone is looking at pane right now: it is the
// active pane of the active window of a session with a client attached.
// Used to decide whether a finished run is "unseen" (herdr's done state).
func IsPaneViewed(pane string) bool {
	return DisplayMessage(pane, "#{&&:#{pane_active},#{&&:#{window_active},#{session_attached}}}") == "1"
}

// ResolvePane maps a target to a pane id: an agent name set with
// `tagent name` (@pane_name) wins, anything else is passed through as a
// tmux target (%12, sess:1.2, …).
func ResolvePane(target string) string {
	out, ok := RunTmux("list-panes", "-a", "-F", "#{pane_id} #{"+PaneName+"}")
	if !ok {
		return target
	}
	for _, line := range strings.Split(out, "\n") {
		id, name, _ := strings.Cut(line, " ")
		if name != "" && name == target {
			return id
		}
	}
	return target
}

// TypeLine pastes text and submits it separately: TUIs can swallow Enter
// arriving in the same burst as a paste. Pi can also accept the text during
// startup before submission is ready, so verify its editor before retrying.
func TypeLine(pane, text string) {
	buf := "tagent-" + strings.TrimPrefix(pane, "%")
	if _, ok := RunTmux("set-buffer", "-b", buf, "--", text); ok {
		RunTmux("paste-buffer", "-p", "-d", "-b", buf, "-t", pane)
	} else {
		RunTmux("send-keys", "-t", pane, "-l", text)
	}
	time.Sleep(300 * time.Millisecond)
	RunTmux("send-keys", "-t", pane, "Enter")
	retryPiSubmit(pane, text, 30*time.Second, 300*time.Millisecond)
}

// Retry only Enter, never the paste. Claude can also drop the Enter while it is
// still taking in a long paste. Lifecycle idle alone is not proof that a
// prompt was missed (a short turn may already have completed). Require the exact
// original text in Pi's editor as well, not a match in transcript/history.
func retryPiSubmit(pane, text string, timeout, poll time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(poll)
		agent := GetPaneOptionValue(pane, PaneAgent)
		if (agent != "pi" && agent != "claude") || GetPaneOptionValue(pane, PaneStatusOpt) != string(StatusIdle) {
			return
		}
		screen, ok := RunTmux("capture-pane", "-p", "-t", pane)
		contains := piEditorContains
		if agent == "claude" {
			contains = claudeEditorContains
		}
		if !ok || !contains(screen, text) {
			return
		}
		RunTmux("send-keys", "-t", pane, "Enter")
	}
}

func piEditorContains(screen, text string) bool {
	lines := strings.Split(screen, "\n")
	bottom := -1
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		// Pi's standard editor is bounded by horizontal rules. Fail closed for
		// custom editors, clipped prompts, or dialogs rather than guessing.
		if len([]rune(line)) < 3 || strings.Trim(line, "─") != "" {
			continue
		}
		if bottom < 0 {
			bottom = i
			continue
		}
		body := lines[i+1 : bottom]
		// A long prompt scrolls: pi shows "── ↑ N more ──" as the editor's first line
		// and only the tail of the text, so match a suffix then.
		scrolled := len(body) > 0 && strings.Contains(body[0], "↑") && strings.Contains(body[0], "more")
		if scrolled {
			body = body[1:]
		}
		got, want := normalizeScreen(strings.Join(body, "\n")), normalizeScreen(text)
		if got == "" || want == "" {
			return false
		}
		return got == want || (scrolled && len(got) >= 20 && strings.HasSuffix(want, got))
	}
	return false
}

func normalizeScreen(s string) string { return strings.Join(strings.Fields(s), "") }
func isRule(line string) bool {
	line = strings.TrimSpace(line)
	return len([]rune(line)) >= 3 && strings.Trim(line, "─") == ""
}

// claudeEditorContains reports an unsent prompt in Claude's input box: the text
// above its bottom rule. A long prompt can be taller than the pane, so only its
// tail (or Claude's "[Pasted text" placeholder) may be visible.
func claudeEditorContains(screen, text string) bool {
	lines := strings.Split(strings.TrimRight(screen, "\n"), "\n")
	bottom := len(lines) - 1
	for bottom >= 0 && !isRule(lines[bottom]) {
		bottom--
	}
	if bottom < 0 {
		return false
	}
	top := bottom - 1
	for top >= 0 && !isRule(lines[top]) {
		top--
	}
	body := strings.TrimLeft(strings.TrimSpace(strings.Join(lines[top+1:bottom], "\n")), "❯> ")
	if strings.HasPrefix(body, "[Pasted text") {
		return true
	}
	got, want := normalizeScreen(body), normalizeScreen(text)
	return got != "" && want != "" && (got == want || (len(got) >= 20 && strings.HasSuffix(want, got)))
}

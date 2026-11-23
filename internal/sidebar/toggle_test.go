package sidebar

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"tmux-agent/internal/tmux"
)

// ─── sidebarPositionFromSetting ────────────────────────────────────────────

func TestSidebarPositionFromSettingOnlyAcceptsRight(t *testing.T) {
	cases := map[string]sidebarPosition{
		"right":   positionRight,
		" RIGHT ": positionRight,
		"left":    positionLeft,
		"":        positionLeft,
		"invalid": positionLeft,
	}
	for setting, want := range cases {
		if got := sidebarPositionFromSetting(setting); got != want {
			t.Fatalf("sidebarPositionFromSetting(%q) = %v, want %v", setting, got, want)
		}
	}
}

// ─── parsePaneGeometry / targetPaneForPosition ─────────────────────────────

func TestParsePaneGeometryMalformedLine(t *testing.T) {
	if _, ok := parsePaneGeometry("bad-line"); ok {
		t.Fatal("ok = true for a broken line, want false")
	}
	if _, ok := parsePaneGeometry("0 nope %1"); ok {
		t.Fatal("ok = true for an unparseable width, want false")
	}
	geo, ok := parsePaneGeometry("12 30 %2")
	if !ok || geo != (paneGeometry{left: 12, width: 30, paneID: "%2"}) {
		t.Fatalf("parsePaneGeometry = %+v, %v", geo, ok)
	}
}

func TestTargetPaneForLeftPositionPicksLeftmostPane(t *testing.T) {
	output := "40 80 %3\n0 20 %1\n20 20 %2"

	got, ok := targetPaneForPosition(output, positionLeft)
	if !ok || got != "%1" {
		t.Fatalf("targetPaneForPosition = %q, %v; want %q, true", got, ok, "%1")
	}
}

func TestTargetPaneForRightPositionPicksRightmostEdge(t *testing.T) {
	output := "0 20 %1\n20 20 %2\n40 80 %3"

	got, ok := targetPaneForPosition(output, positionRight)
	if !ok || got != "%3" {
		t.Fatalf("targetPaneForPosition = %q, %v; want %q, true", got, ok, "%3")
	}
}

func TestTargetPaneForPositionSkipsMalformedLines(t *testing.T) {
	output := "bad-line\n0 nope %1\n12 30 %2"

	if got, ok := targetPaneForPosition(output, positionLeft); !ok || got != "%2" {
		t.Fatalf("targetPaneForPosition = %q, %v; want %q, true", got, ok, "%2")
	}
	if _, ok := targetPaneForPosition("", positionRight); ok {
		t.Fatal("ok = true for empty output, want false")
	}
}

func TestSplitWindowFlagsForMatchTmuxSemantics(t *testing.T) {
	if got := splitWindowFlagsFor(positionLeft); got != "-hfb" {
		t.Fatalf("splitWindowFlagsFor(left) = %q", got)
	}
	if got := splitWindowFlagsFor(positionRight); got != "-hf" {
		t.Fatalf("splitWindowFlagsFor(right) = %q", got)
	}
}

// ─── uniqueWindowPaths / anySidebarPane ────────────────────────────────────

func TestUniqueWindowPathsDedupesKeepsBlank(t *testing.T) {
	output := "%1|/Users/me/My Project\n%1|/Users/me/My Project\n%2|/tmp/another project"
	want := []windowPath{
		{windowID: "%1", path: "/Users/me/My Project"},
		{windowID: "%2", path: "/tmp/another project"},
	}
	if got := uniqueWindowPaths(output); !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueWindowPaths = %+v, want %+v", got, want)
	}
}

func TestUniqueWindowPathsSkipsMalformedLines(t *testing.T) {
	output := "bad-line\n%1|/tmp"
	want := []windowPath{{windowID: "%1", path: "/tmp"}}
	if got := uniqueWindowPaths(output); !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueWindowPaths = %+v, want %+v", got, want)
	}
}

func TestAnySidebarPaneFindsItAnywhere(t *testing.T) {
	if !anySidebarPane("%1|pane\n%2|sidebar\n%3|pane") {
		t.Fatal("want true, there is a sidebar line")
	}
	if anySidebarPane("%1|pane\n%2|main") {
		t.Fatal("want false, there is no sidebar line")
	}
}

// ─── splitLines ─────────────────────────────────────────────────────────────

func TestSplitLinesDropsTrailingEmptyLineFromNewline(t *testing.T) {
	if got := splitLines(""); got != nil {
		t.Fatalf("splitLines(\"\") = %v, want nil", got)
	}
	if got := splitLines("sidebar\n"); !reflect.DeepEqual(got, []string{"sidebar"}) {
		t.Fatalf("splitLines = %v, want [sidebar]", got)
	}
	if got := splitLines("\nsidebar\n"); !reflect.DeepEqual(got, []string{"", "sidebar"}) {
		t.Fatalf("splitLines = %v, want [\"\" sidebar]", got)
	}
}

// ─── the three commands, using tmux.Fake ───────────────────────────────────

func selfBin(t *testing.T) string {
	t.Helper()
	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable(): %v", err)
	}
	return bin
}

// hasCall reports true if fake.Calls() contains exactly one command with
// this argument list — it doesn't pin down the total number of commands,
// it only checks that the expected command is present.
func hasCall(fake *tmux.Fake, want ...string) bool {
	for _, call := range fake.Calls() {
		if reflect.DeepEqual(call, want) {
			return true
		}
	}
	return false
}

func TestToggleNoWindowIdExitsSilently(t *testing.T) {
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()

	if code := Toggle(nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if len(fake.Calls()) != 0 {
		t.Fatalf("without a window_id, tmux must not be called, got %v", fake.Calls())
	}
}

func TestToggleSidebarExistsKillsPane(t *testing.T) {
	fake := tmux.NewFake()
	fake.Set("", "display-message", "-t", "%50", "-p", "#{@sidebar_width}")
	fake.Set("", "display-message", "-t", "%50", "-p", "#{@sidebar_position}")
	fake.Set("%2|sidebar\n%1|\n", "list-panes", "-t", "%50", "-F", "#{pane_id}|#{@pane_role}")
	defer tmux.SetRunner(fake)()

	if code := Toggle([]string{"%50"}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !hasCall(fake, "kill-pane", "-t", "%2") {
		t.Fatalf("missing kill-pane -t %%2 command in %v", fake.Calls())
	}
}

func TestToggleSidebarExistsAndCreateOnlyDoesNothing(t *testing.T) {
	fake := tmux.NewFake()
	fake.Set("", "display-message", "-t", "%50", "-p", "#{@sidebar_width}")
	fake.Set("", "display-message", "-t", "%50", "-p", "#{@sidebar_position}")
	fake.Set("%2|sidebar\n", "list-panes", "-t", "%50", "-F", "#{pane_id}|#{@pane_role}")
	defer tmux.SetRunner(fake)()

	if code := Toggle([]string{"--create-only", "%50"}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, call := range fake.Calls() {
		if len(call) > 0 && call[0] == "kill-pane" {
			t.Fatalf("--create-only must not kill-pane, got %v", fake.Calls())
		}
	}
}

func TestToggleNoSidebarCreatesPaneAndRestoresFocus(t *testing.T) {
	bin := selfBin(t)
	fake := tmux.NewFake()
	fake.Set("", "display-message", "-t", "%50", "-p", "#{@sidebar_width}")
	fake.Set("right\n", "display-message", "-t", "%50", "-p", "#{@sidebar_position}")
	fake.Set("", "list-panes", "-t", "%50", "-F", "#{pane_id}|#{@pane_role}")
	fake.Set("0 20 %1\n20 20 %2\n", "list-panes", "-t", "%50", "-F", "#{pane_left} #{pane_width} #{pane_id}")
	fake.Set("%2\n", "display-message", "-t", "%50", "-p", "#{pane_id}")
	fake.Set("%9\n", "split-window", "-hf", "-l", "30", "-t", "%2", "-c", "~", "-P", "-F", "#{pane_id}", bin)
	defer tmux.SetRunner(fake)()

	if code := Toggle([]string{"%50"}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !hasCall(fake, "split-window", "-hf", "-l", "30", "-t", "%2", "-c", "~", "-P", "-F", "#{pane_id}", bin) {
		t.Fatalf("missing split-window command with the correct arguments in %v", fake.Calls())
	}
	if !hasCall(fake, "set", "-t", "%9", "-p", tmux.PaneRole, "sidebar") {
		t.Fatalf("missing set @pane_role=sidebar command for the new pane in %v", fake.Calls())
	}
	if !hasCall(fake, "select-pane", "-t", "%2") {
		t.Fatalf("missing command to return focus to the previously active pane in %v", fake.Calls())
	}
}

func TestToggleUsesPercentSidebarWidth(t *testing.T) {
	bin := selfBin(t)
	fake := tmux.NewFake()
	fake.Set("15%\n", "display-message", "-t", "%50", "-p", "#{@sidebar_width}")
	fake.Set("100\n", "display-message", "-t", "%50", "-p", "#{window_width}")
	fake.Set("", "display-message", "-t", "%50", "-p", "#{@sidebar_position}")
	fake.Set("", "list-panes", "-t", "%50", "-F", "#{pane_id}|#{@pane_role}")
	fake.Set("0 100 %1\n", "list-panes", "-t", "%50", "-F", "#{pane_left} #{pane_width} #{pane_id}")
	fake.Set("%1\n", "display-message", "-t", "%50", "-p", "#{pane_id}")
	fake.Set("%9\n", "split-window", "-hfb", "-l", "15", "-t", "%1", "-c", "~", "-P", "-F", "#{pane_id}", bin)
	defer tmux.SetRunner(fake)()

	if code := Toggle([]string{"%50"}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	// 100 * 15 / 100 = 15.
	if !hasCall(fake, "split-window", "-hfb", "-l", "15", "-t", "%1", "-c", "~", "-P", "-F", "#{pane_id}", bin) {
		t.Fatalf("missing split-window -l 15 (15%% of 100) in %v", fake.Calls())
	}
}

func TestToggleAllClosesAllWhenAtLeastOneSidebarExists(t *testing.T) {
	fake := tmux.NewFake()
	fake.Set("%1|pane\n%2|sidebar\n%3|sidebar\n", "list-panes", "-a", "-F", "#{pane_id}|#{@pane_role}")
	defer tmux.SetRunner(fake)()

	if code := ToggleAll(nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !hasCall(fake, "kill-pane", "-t", "%2") {
		t.Fatalf("missing kill-pane -t %%2 in %v", fake.Calls())
	}
	if !hasCall(fake, "kill-pane", "-t", "%3") {
		t.Fatalf("missing kill-pane -t %%3 in %v", fake.Calls())
	}
}

func TestToggleAllCreatesInEveryWindowWhenNoSidebarExists(t *testing.T) {
	bin := selfBin(t)
	fake := tmux.NewFake()
	fake.Set("%1|\n%2|\n", "list-panes", "-a", "-F", "#{pane_id}|#{@pane_role}")
	fake.Set("$1|/tmp/a\n$2|/tmp/b\n", "list-panes", "-a", "-F", "#{window_id}|#{pane_current_path}")
	// Toggle is called for each window with --create-only: declare enough
	// so it doesn't panic (scenario where there's no sidebar yet).
	for _, w := range []struct{ window, path string }{{"$1", "/tmp/a"}, {"$2", "/tmp/b"}} {
		fake.Set("", "display-message", "-t", w.window, "-p", "#{@sidebar_width}")
		fake.Set("", "display-message", "-t", w.window, "-p", "#{@sidebar_position}")
		fake.Set("", "list-panes", "-t", w.window, "-F", "#{pane_id}|#{@pane_role}")
		fake.Set("0 20 %1\n", "list-panes", "-t", w.window, "-F", "#{pane_left} #{pane_width} #{pane_id}")
		fake.Set("%1\n", "display-message", "-t", w.window, "-p", "#{pane_id}")
		fake.Set("%9\n", "split-window", "-hfb", "-l", "30", "-t", "%1", "-c", w.path, "-P", "-F", "#{pane_id}", bin)
	}
	defer tmux.SetRunner(fake)()

	if code := ToggleAll(nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !hasCall(fake, "split-window", "-hfb", "-l", "30", "-t", "%1", "-c", "/tmp/a", "-P", "-F", "#{pane_id}", bin) {
		t.Fatalf("missing split-window for $1 in %v", fake.Calls())
	}
	if !hasCall(fake, "split-window", "-hfb", "-l", "30", "-t", "%1", "-c", "/tmp/b", "-P", "-F", "#{pane_id}", bin) {
		t.Fatalf("missing split-window for $2 in %v", fake.Calls())
	}
}

func TestAutoCloseNoWindowIdExitsSilently(t *testing.T) {
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()

	if code := AutoClose(nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if len(fake.Calls()) != 0 {
		t.Fatalf("without a window_id, tmux must not be called, got %v", fake.Calls())
	}
}

func TestAutoCloseClosesWindowWhenOnlySidebarRemains(t *testing.T) {
	fake := tmux.NewFake()
	fake.Set("sidebar\n", "list-panes", "-t", "@3", "-F", "#{@pane_role}")
	fake.Set("", "kill-window", "-t", "@3")
	defer tmux.SetRunner(fake)()

	if code := AutoClose([]string{"@3"}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !hasCall(fake, "kill-window", "-t", "@3") {
		t.Fatalf("missing kill-window for the last sidebar pane, got %v", fake.Calls())
	}
}

func TestAutoCloseReportsKillWindowFailure(t *testing.T) {
	fake := tmux.NewFake()
	fake.Set("sidebar\n", "list-panes", "-t", "@3", "-F", "#{@pane_role}")
	fake.SetErr(errors.New("busy"), "kill-window", "-t", "@3")
	defer tmux.SetRunner(fake)()

	if code := AutoClose([]string{"@3"}); code == 0 {
		t.Fatal("exit code = 0, want failure when kill-window fails")
	}
}

func TestAutoCloseKeepsWindowWhenOtherPaneRemains(t *testing.T) {
	fake := tmux.NewFake()
	fake.Set("sidebar\n\n", "list-panes", "-t", "@3", "-F", "#{@pane_role}")
	defer tmux.SetRunner(fake)()

	if code := AutoClose([]string{"@3"}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, call := range fake.Calls() {
		if len(call) > 0 && call[0] == "kill-window" {
			t.Fatalf("must not kill-window while another pane remains, got %v", fake.Calls())
		}
	}
}

func TestAutoCloseKeepsWindowWhenListPanesErrors(t *testing.T) {
	fake := tmux.NewFake()
	fake.SetErr(errors.New("busy"), "list-panes", "-t", "@3", "-F", "#{@pane_role}")
	defer tmux.SetRunner(fake)()

	if code := AutoClose([]string{"@3"}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, call := range fake.Calls() {
		if len(call) > 0 && call[0] == "kill-window" {
			t.Fatalf("must not kill-window when list-panes fails, got %v", fake.Calls())
		}
	}
}

package assets

import (
	"os"
	"strings"
	"testing"
)

// Asset is a file copied from upstream. This test does not check their
// content, only guards against two things that break easily: the file being
// gone, and any leftover Rust paths.
func TestAssetsExist(t *testing.T) {
	for _, p := range []string{
		"agent-sidebar.conf",
		"tagent.tmux",
		"hook.sh",
		"hooks/hooks.json",
		".opencode/plugins/tagent-sidebar.js",
		"fixtures/scenarios/common/_lib.sh",
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing asset %s: %v", p, err)
		}
	}
}

// TestNoRustPathsLeftInScripts guards against two easily-missed traces of the
// original Rust version: the target/release build path, and the binary name
// "tmux-agent-sidebar" (from the Rust version) being invoked as a COMMAND
// instead of "tagent" (the real Go binary). It does not ban the string
// "tmux-agent-sidebar" outright — it is still valid as a file name
// (tagent-sidebar.js, the OpenCode contract) or as part of the upstream
// repo name in attribution ("hiroppy/tmux-agent-sidebar").
func TestNoRustPathsLeftInScripts(t *testing.T) {
	for _, p := range []string{
		"tagent.tmux",
		"hook.sh",
		"fixtures/scenarios/common/_lib.sh",
		"agent-sidebar.conf",
		".opencode/plugins/tagent-sidebar.js",
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		content := string(data)

		if strings.Contains(content, "target/release") {
			t.Errorf("%s still points to the Rust version's target/release", p)
		}
		if used := rustSidebarBinaryUsedAsCommand(content); used {
			t.Errorf("%s invokes \"tmux-agent-sidebar\" as a command — that is the Rust version's binary name, the real Go command is \"tagent\"", p)
		}
	}
}

// rustSidebarBinaryUsedAsCommand reports true if the string
// "tmux-agent-sidebar" appears NOT as a file name (not followed by "." — a
// file extension like .tmux/.js/.sh) and NOT as part of
// "hiroppy/tmux-agent-sidebar" (preceded by "/" — the upstream repo name in
// attribution).
func rustSidebarBinaryUsedAsCommand(content string) bool {
	const needle = "tmux-agent-sidebar"
	for i := 0; ; {
		idx := strings.Index(content[i:], needle)
		if idx < 0 {
			return false
		}
		start := i + idx
		end := start + len(needle)
		i = end

		if start > 0 && content[start-1] == '/' {
			continue
		}
		if end < len(content) && content[end] == '.' {
			continue
		}
		return true
	}
}

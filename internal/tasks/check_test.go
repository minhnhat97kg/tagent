package tasks

import (
	"strings"
	"testing"
)

func TestCheckBoardRoleAgent(t *testing.T) {
	cfg := BoardConfig{Roles: map[string]Role{"dev": {Agent: "nope"}, "qa": {Agent: "claude"}}}
	got := strings.Join(CheckBoard(t.TempDir(), cfg), "\n")
	if !strings.Contains(got, `role dev: unsupported agent "nope"`) || strings.Contains(got, "role qa") {
		t.Fatalf("problems: %q", got)
	}
}

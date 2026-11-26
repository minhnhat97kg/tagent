package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/tasks"
)

func TestSaveRoleContextRoundTrip(t *testing.T) {
	dir := setup(t)
	none, mcp := []string{}, []string{"tagent"}
	a := Agent{Name: "reviewer", Harness: "claude", Source: "board", ContextSet: true,
		RoleContext: tasks.RoleContext{Skills: &none, MCP: &mcp, Tools: []string{"Read"}}}
	if err := Save(dir, "", a); err != nil {
		t.Fatal(err)
	}
	b := board(t, dir)
	for _, want := range []string{"# project board", "# the planner picks the plan", "fresh_after: 3", "skills: []", "mcp: [tagent]", "tools: [Read]"} {
		if !strings.Contains(b, want) {
			t.Fatalf("board lost %q:\n%s", want, b)
		}
	}
	got := byName(List(dir, "", nil))["reviewer"]
	if got.Skills == nil || len(*got.Skills) != 0 || got.MCP == nil || (*got.MCP)[0] != "tagent" || len(got.Tools) != 1 {
		t.Fatalf("listed context %+v", got.RoleContext)
	}
	// Clearing a field removes its key; an unsupported combination is refused and writes nothing.
	if err := Save(dir, "", Agent{Name: "reviewer", Harness: "claude", Source: "board", ContextSet: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(board(t, dir), "skills:") {
		t.Fatal("unset skills kept its key")
	}
	bad := Agent{Name: "reviewer", Harness: "pi", Source: "board", ContextSet: true, RoleContext: tasks.RoleContext{MCP: &mcp}}
	if err := Save(dir, "", bad); err == nil {
		t.Fatal("pi with an MCP list was saved")
	}
}

func TestSaveWithoutContextKeysKeepsRestrictions(t *testing.T) {
	dir := setup(t)
	mcp := []string{"tagent"}
	if err := Save(dir, "", Agent{Name: "reviewer", Harness: "claude", Source: "board", ContextSet: true, RoleContext: tasks.RoleContext{MCP: &mcp}}); err != nil {
		t.Fatal(err)
	}
	// A save that sent no context keys (e.g. a model-only edit) leaves the restriction alone.
	if err := Save(dir, "", Agent{Name: "reviewer", Harness: "claude", Source: "board", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if got := byName(List(dir, "", nil))["reviewer"]; got.MCP == nil || (*got.MCP)[0] != "tagent" {
		t.Fatalf("context deleted by a save without context keys: %+v", got.RoleContext)
	}
}

// A configured harness name is saved, with the effort levels of its kind; a name outside harnesses.yml is refused.
func TestSaveConfiguredHarness(t *testing.T) {
	dir := setup(t)
	config := filepath.Join(dir, "xdg")
	os.MkdirAll(filepath.Join(config, "tagent"), 0700)
	os.WriteFile(filepath.Join(config, "tagent", "harnesses.yml"), []byte("harnesses:\n  - name: Claude API\n    command: claude-api\n"), 0600)
	if err := Save(dir, "", Agent{Name: "reviewer", Harness: "Claude API", Model: "haiku", Effort: "xhigh", Source: "board"}); err != nil {
		t.Fatal(err)
	}
	if got := byName(List(dir, "", nil))["reviewer"]; got.Harness != "Claude API" || got.Effort != "xhigh" {
		t.Fatalf("listed %+v", got)
	}
	if err := Save(dir, "", Agent{Name: "reviewer", Harness: "Nope", Source: "board"}); err == nil || !strings.Contains(err.Error(), "Claude API") {
		t.Fatalf("unknown harness: %v", err)
	}
}

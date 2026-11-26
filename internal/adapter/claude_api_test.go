package adapter

import "testing"

func TestClaudeAPIAdapterLabelsEventsClaudeAPI(t *testing.T) {
	a, ok := Resolve("claude-api")
	if !ok || a.Name() != "claude-api" {
		t.Fatalf("Resolve(claude-api) = %v, %v", a, ok)
	}
	evt, ok := a.Parse("session-start", map[string]any{"session_id": "s1"})
	if !ok || evt.Agent != "claude-api" {
		t.Fatalf("Parse = %+v, %v; want Agent claude-api", evt, ok)
	}
	if len(a.Registrations()) != len((ClaudeAdapter{}).Registrations()) {
		t.Error("claude-api must register the same hooks as claude")
	}
}

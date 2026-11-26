package main

import (
	"testing"

	"tagent/internal/board"
)

func TestTierPaneReuseNeedsSameRoute(t *testing.T) {
	tier := board.Action{Agent: "dev", Role: "pi", Model: "openai-codex/gpt-6-luna", Tier: "fast"}
	if !reusable(tier, "pi:openai-codex/gpt-6-luna") {
		t.Fatal("same route should reuse")
	}
	if reusable(tier, "pi:openai-codex/gpt-6.1-sol") || reusable(tier, "") {
		t.Fatal("different model or a pane without a route must not be reused")
	}
	if !reusable(board.Action{Agent: "dev", Role: "claude", Model: "opus"}, "") {
		t.Fatal("fixed roles reuse as before")
	}
}

// A configured harness is its own pane route: "Claude API" never shares a pane with "claude".
func TestTierPaneReuseKeepsConfiguredName(t *testing.T) {
	api := board.Action{Agent: "dev", Role: "Claude API", Model: "haiku", Tier: "fast"}
	if !reusable(api, "Claude API:haiku") {
		t.Fatal("same configured name and model should reuse")
	}
	if reusable(api, "claude:haiku") {
		t.Fatal("the plain claude harness must not reuse a Claude API pane")
	}
}

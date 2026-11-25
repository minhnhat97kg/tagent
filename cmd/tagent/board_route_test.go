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

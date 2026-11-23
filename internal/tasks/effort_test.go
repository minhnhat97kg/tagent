package tasks

import (
	"fmt"
	"testing"
)

func TestRoleEffort(t *testing.T) {
	for _, tc := range []struct {
		agent, effort string
		valid         bool
	}{
		{"pi", "", true}, {"pi", "off", true}, {"pi", "max", true}, {"claude", "high", true}, {"claude", "off", false}, {"codex", "none", true}, {"codex", "off", false}, {"opencode", "", true}, {"opencode", "high", false}, {"reviewer", "high", true}, {"pi", "typo", false},
	} {
		cfg, err := ParseBoardConfig([]byte(fmt.Sprintf("roles: {planner: {agent: %s, effort: %q}}", tc.agent, tc.effort)))
		if (err == nil) != tc.valid {
			t.Fatalf("%+v: %v", tc, err)
		}
		if err == nil && cfg.Roles["planner"].Effort != tc.effort {
			t.Fatal(cfg)
		}
	}
	for _, yaml := range []string{"roles: {planner: {agent: pi, efforts: high}}", "statuses: [{id: todo, effort: high}, {id: done}]"} {
		if _, err := ParseBoardConfig([]byte(yaml)); err == nil {
			t.Fatal("accepted unknown field", yaml)
		}
	}
	cfg, err := ParseBoardConfig([]byte("roles: {planner: {agent: pi}}"))
	if err != nil || cfg.Roles["planner"].Effort != "" {
		t.Fatal(cfg, err)
	}
}

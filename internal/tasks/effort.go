package tasks

import (
	"fmt"
	"slices"
)

// ValidateEffort checks known spellings and builtin harness compatibility.
// Named repository agents are checked again once their harness is resolved.
func ValidateEffort(harness, effort string) error {
	if effort == "" {
		return nil
	}
	levels := []string{"off", "none", "minimal", "low", "medium", "high", "xhigh", "max"}
	switch harness {
	case "pi":
		levels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}
	case "claude":
		levels = []string{"low", "medium", "high", "xhigh", "max"}
	case "codex":
		levels = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}
	case "opencode", "gemini", "copilot", "amp":
		return fmt.Errorf("effort is unsupported for %s", harness)
	}
	if !slices.Contains(levels, effort) {
		return fmt.Errorf("invalid effort %q for %s", effort, harness)
	}
	return nil
}

package tasks

import (
	"fmt"
	"slices"

	"tagent/internal/roles"
)

// ValidateEffort checks effort against the levels harness accepts (roles.Efforts).
// An empty or unlisted harness is a tier role or a named repository agent, whose
// harness is only resolved later: its effort need only be a level some harness has.
func ValidateEffort(harness, effort string) error {
	if effort == "" {
		return nil
	}
	if harness == "" || !roles.KnownHarness(harness) {
		if roles.KnownEffort(effort) {
			return nil
		}
		return fmt.Errorf("invalid effort %q", effort)
	}
	levels := roles.Efforts(harness)
	if len(levels) == 0 {
		return fmt.Errorf("effort is unsupported for %s", harness)
	}
	if !slices.Contains(levels, effort) {
		return fmt.Errorf("invalid effort %q for %s", effort, harness)
	}
	return nil
}

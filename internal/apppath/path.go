// Package apppath selects the canonical application directory without moving user data.
package apppath

import (
	"os"
	"path/filepath"
)

// Dir prefers tagent, falling back only when it is absent and the legacy root exists.
func Dir(base string) string {
	current := filepath.Join(base, "tagent")
	if _, err := os.Stat(current); os.IsNotExist(err) {
		legacy := filepath.Join(base, "tmux-agent")
		if _, err := os.Stat(legacy); err == nil {
			return legacy
		}
	}
	return current
}

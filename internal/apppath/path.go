// Package apppath selects the application directory under a config or data base.
package apppath

import (
	"os"
	"path/filepath"
)

// Dir is the tagent directory under base.
func Dir(base string) string { return filepath.Join(base, "tagent") }

// ConfigDir is the tagent config directory: $XDG_CONFIG_HOME/tagent, else ~/.config/tagent.
func ConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		h, _ := os.UserHomeDir()
		base = filepath.Join(h, ".config")
	}
	return Dir(base)
}

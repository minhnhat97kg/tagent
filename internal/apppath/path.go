// Package apppath selects the application directory under a config or data base.
package apppath

import "path/filepath"

// Dir is the tagent directory under base.
func Dir(base string) string { return filepath.Join(base, "tagent") }

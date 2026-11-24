package apppath

import (
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	if got := Dir("/x"); got != filepath.Join("/x", "tagent") {
		t.Fatalf("got %s", got)
	}
}

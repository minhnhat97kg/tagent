package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

// Every tracked example workflow must parse, so a bad edit cannot ship.
func TestExampleWorkflowsParse(t *testing.T) {
	files, _ := filepath.Glob("../../examples/workflows/*.yml")
	if len(files) == 0 {
		t.Fatal("no example workflows found")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if w, err := Parse(b); err != nil || w.Jobs[0].ID != "scout" {
			t.Fatalf("%s: %v", f, err)
		}
	}
}

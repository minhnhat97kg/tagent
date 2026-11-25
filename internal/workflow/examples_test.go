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
		w, err := Parse(b)
		if err != nil || (w.Workplace != "none" && w.Workplace != "folder" && w.Jobs[0].ID != "scout") {
			t.Fatalf("%s: %v", f, err)
		}
		if name := filepath.Base(f); (name == "research.yml" || name == "docs.yml") && w.Workplace != "none" {
			t.Fatalf("%s: want workplace none", f)
		}
		if name := filepath.Base(f); name == "notes.yml" && w.Workplace != "folder" {
			t.Fatalf("%s: want workplace folder", f)
		}
	}
}

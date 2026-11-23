package skill

import (
	"strings"
	"testing"
)

func TestRenderFillsBinary(t *testing.T) {
	got := Render("/opt/bin/tagent")
	if strings.Contains(got, "{{BIN}}") || !strings.HasPrefix(got, "---\nname: tagent") || !strings.Contains(got, "/opt/bin/tagent new") {
		t.Fatalf("bad skill:\n%s", got)
	}
}

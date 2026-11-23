package intake

import (
	"strings"
	"testing"
)

func TestRunner(t *testing.T) {
	out, e := Run("cat", "whole transcript")
	if e != nil || out != "whole transcript" {
		t.Fatal(out, e)
	}
	_, e = Run("echo bad >&2; exit 1", "")
	if e == nil || !strings.Contains(e.Error(), "bad") {
		t.Fatal(e)
	}
}

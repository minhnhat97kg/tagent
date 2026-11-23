package workflow

import (
	"strings"
	"testing"
)

func TestDescription(t *testing.T) {
	for _, description := range []string{"", "Used for building tickets"} {
		src := Template("example")
		if description == "" {
			src = strings.Replace(src, "description: Describe when to use this workflow\n", "", 1)
		} else {
			src = strings.Replace(src, "Describe when to use this workflow", description, 1)
		}
		w, e := Parse([]byte(src))
		if e != nil || w.Description != description {
			t.Fatal(w.Description, e)
		}
	}
}

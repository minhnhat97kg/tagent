package scrape

import (
	"reflect"
	"testing"
)

func TestOrphanWindows(t *testing.T) {
	out := "@1|2|sidebar\n@1|2|\n@2|1|sidebar\n@3|1|\n@4|1|other\n"
	if got, want := orphanWindows(out), map[string]bool{"@2": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("orphanWindows = %v, want %v", got, want)
	}
}

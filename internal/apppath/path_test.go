package apppath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	for _, tc := range []struct {
		name         string
		old, current bool
		want         string
	}{
		{"fresh", false, false, "tagent"}, {"legacy", true, false, "tmux-agent"},
		{"canonical", false, true, "tagent"}, {"both", true, true, "tagent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			for name, exists := range map[string]bool{"tagent": tc.current, "tmux-agent": tc.old} {
				if exists {
					if err := os.Mkdir(filepath.Join(base, name), 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			if got := Dir(base); got != filepath.Join(base, tc.want) {
				t.Fatalf("got %s", got)
			}
		})
	}
}

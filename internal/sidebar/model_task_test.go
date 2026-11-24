package sidebar

import (
	"testing"

	"tagent/internal/tmux"
)

func TestByTask(t *testing.T) {
	w := func(name string) tmux.WindowInfo { return tmux.WindowInfo{Panes: []tmux.PaneInfo{{Name: name}}} }
	got := byTask([]tmux.WindowInfo{w("dev-12"), w("plan#7"), w("bonsai-test"), w("review#12")})
	var names []string
	for _, x := range got {
		names = append(names, x.Panes[0].Name)
	}
	want := []string{"bonsai-test", "plan#7", "dev-12", "review#12"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v want %v", names, want)
		}
	}
}

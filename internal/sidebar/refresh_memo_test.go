package sidebar

import "testing"

// Panes share one refresh result for refreshMemoTTL, but each edits its own
// copy of the maps: a write in one pane must not reach another.
func TestRefreshMapsArePerPane(t *testing.T) {
	a := refreshCmd()().(RefreshMsg)
	a.SizeOwners["leak"] = true
	b := refreshCmd()().(RefreshMsg)
	if b.SizeOwners["leak"] {
		t.Fatal("pane write leaked into the shared refresh result")
	}
}

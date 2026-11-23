package queue

import "testing"

func TestPushRankDeliversPriorityFirst(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, m := range []struct {
		text string
		rank int
	}{{"normal", 1}, {"low", 2}, {"high", 0}, {"normal-2", 1}} {
		if _, _, err := PushRank("%9", m.text, m.rank); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"high", "normal", "normal-2", "low"}
	for i, w := range want {
		got, ok := Pop("%9")
		if !ok || got != w {
			t.Fatalf("pop %d = %q %v, want %q", i, got, ok, w)
		}
	}
}

package settings

import "testing"

func TestParseRoundTrip(t *testing.T) {
	got := parse("# c\nset -g @a 'it'\\''s'\nset -g @b 25%\nset-option -g @c \"x y\"\nbind x y\n")
	if got["@a"] != "it's" || got["@b"] != "25%" || got["@c"] != "x y" || len(got) != 3 {
		t.Fatalf("%v", got)
	}
}

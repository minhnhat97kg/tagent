package tmux

import (
	"strings"
	"testing"
)

func TestSessionName(t *testing.T) {
	a, b := SessionName("/x/api"), SessionName("/y/api")
	if a == b || a != SessionName("/x/api") {
		t.Fatalf("same folder, different path must differ and be stable: %s %s", a, b)
	}
	if got := SessionName("/x/my.app:v1"); !strings.HasPrefix(got, "tagent:my_app_v1-") || len(got) != len("tagent:my_app_v1-")+4 {
		t.Fatalf("not sanitized: %s", got)
	}
}

func TestProjectSession(t *testing.T) {
	f := NewFake()
	defer SetRunner(f)()
	name := SessionName("/x/api")
	f.Set("$7 @8", "new-session", "-d", "-s", name, "-c", "/x/api", "-P", "-F", "#{session_id} #{window_id}")
	f.Set("", "set-option", "-t", "$7", OwnerOption, "/x/api")
	if id, spare, err := ProjectSession("/x/api"); err != nil || id != "$7" || spare != "@8" {
		t.Fatal(id, spare, err)
	}
	f.Set("$7 "+name, "list-sessions", "-F", "#{session_id} #{session_name}")
	if id, spare, err := ProjectSession("/x/api"); err != nil || id != "$7" || spare != "" {
		t.Fatal(id, spare, err)
	}
}

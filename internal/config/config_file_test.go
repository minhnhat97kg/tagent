package config

import (
	"testing"

	"tagent/internal/settings"
)

func TestSetWritesTheConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, _ := Find("notice_seconds")
	if _, err := Set(s, "25"); err != nil {
		t.Fatal(err)
	}
	if v, ok := settings.Get("@agent_notice_seconds"); !ok || v != "25" {
		t.Fatalf("file has %q %v", v, ok)
	}
}

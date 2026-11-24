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

func TestSeedWritesDefaultsOnce(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if ok, err := Seed(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if v, _ := settings.Get("@agent_notice_seconds"); v != "10" {
		t.Fatalf("default not seeded: %q", v)
	}
	s, _ := Find("notice_seconds")
	Set(s, "30")
	if ok, _ := Seed(); ok {
		t.Fatal("seed overwrote an existing file")
	}
	if v, _ := settings.Get("@agent_notice_seconds"); v != "30" {
		t.Fatalf("existing value lost: %q", v)
	}
}

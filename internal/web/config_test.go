package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"tagent/internal/config"
	"tagent/internal/tmux"
)

func TestSettingsHideSecretsAndRefuseReadOnly(t *testing.T) {
	f := tmux.NewFake()
	f.Set("https://ntfy.example/topic?auth=SECRET", "show", "-gv", "@agent_notify_url")
	defer tmux.SetRunner(f)()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := &server{}
	w := httptest.NewRecorder()
	s.settings(w, httptest.NewRequest("GET", "/api/config", nil))
	if b := w.Body.String(); strings.Contains(b, "SECRET") || !strings.Contains(b, `"value":"set"`) || !strings.Contains(b, `"@agent_stuck_minutes"`) || strings.Contains(b, `"@agent_theme"`) {
		t.Fatalf("settings leaked or incomplete: %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	s.settings(w, httptest.NewRequest("POST", "/api/config", strings.NewReader(`{"Key":"@agent_web_token","Value":"x"}`)))
	if w.Code != 400 {
		t.Fatalf("read-only setting accepted: %d", w.Code)
	}
}

func TestStateReportsCompact(t *testing.T) {
	defer tmux.SetRunner(tmux.NewFake())()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := &server{}
	get := func() string {
		w := httptest.NewRecorder()
		s.state(w, httptest.NewRequest("GET", "/api/state", nil))
		return w.Body.String()
	}
	if b := get(); !strings.Contains(b, `"compact":false`) {
		t.Fatalf("compact should default off: %s", b)
	}
	for _, st := range config.Settings {
		if st.Key == "@agent_compact" {
			if _, err := config.Set(st, "on"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if b := get(); !strings.Contains(b, `"compact":true`) {
		t.Fatalf("compact not reported after set: %s", b)
	}
}

package web

import (
	"net/http/httptest"
	"strings"
	"testing"

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

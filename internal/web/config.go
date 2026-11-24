package web

import (
	"fmt"
	"net/http"
	"slices"

	"tagent/internal/config"
)

// webReadOnly settings are shown but not editable from the browser: secrets never leave the
// machine, and changing the web address or token from the web would cut the session off.
var webReadOnly = []string{"@agent_notify_url", "@agent_web_token", "@agent_web_addr", "@agent_web"}
var webSecret = []string{"@agent_notify_url", "@agent_web_token"}

var kinds = map[config.Kind]string{config.Bool: "bool", config.Choice: "choice", config.Preset: "preset", config.Int: "int", config.Text: "text"}

// settings serves the same registry as `tagent config` and the TUI settings screen.
func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var o struct {
			Key, Value string
			Reset      bool
		}
		if err := readBody(r, &o); err != nil {
			fail(w, err)
			return
		}
		st, ok := config.Find(o.Key)
		if !ok || slices.Contains(webReadOnly, o.Key) {
			fail(w, fmt.Errorf("setting %q cannot be changed from the web", o.Key))
			return
		}
		old := config.Get(st)
		var err error
		if o.Reset {
			err = config.Reset(st)
		} else {
			_, err = config.Set(st, o.Value)
		}
		if err != nil {
			fail(w, err)
			return
		}
		if st.Reload {
			config.ReloadKeys(old)
		}
	}
	out := []map[string]any{}
	for _, st := range config.Settings {
		if st.TUIOnly {
			continue
		}
		v := config.Get(st)
		secret := slices.Contains(webSecret, st.Key)
		if secret {
			v = map[bool]string{true: "set", false: ""}[v != ""]
		}
		out = append(out, map[string]any{"key": st.Key, "group": st.Group, "label": st.Label, "help": st.Help, "kind": kinds[st.Kind],
			"value": v, "default": st.Default, "choices": st.Choices, "min": st.Min, "max": st.Max,
			"readOnly": slices.Contains(webReadOnly, st.Key), "secret": secret, "reload": st.Reload})
	}
	writeJSON(w, out)
}

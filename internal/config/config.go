// Package config is tagent's central configuration: one registry of
// every user setting (group, kind, choices, default, help), shared by
// `tagent config list|get|set|reset` and the settings screen (ui.go).
//
// Settings are tmux global options. A change is applied to the running
// server at once (`set -g`) and persisted to ConfigFile — tmux syntax,
// sourced by assets/tagent.tmux before the plugin seeds its defaults —
// so it survives a tmux restart.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"tagent/internal/roles"
	"tagent/internal/settings"

	"tagent/internal/tmux"
)

// Kind is how a setting is edited.
type Kind int

const (
	Bool         Kind = iota // on / off
	Choice                   // one of Choices
	Preset                   // one of Choices, or any text
	Int                      // a number in [Min, Max]
	Text                     // free text
	Workspace                // a workspace row of the settings screen; Key is its path
	AddWorkspace             // the row that adds one
)

// Setting describes one option.
type Setting struct {
	Key, Group, Label, Help string
	Kind                    Kind
	Default                 string
	Choices                 []string
	Min, Max                int
	// Reload: only takes effect after the plugin reloads (key bindings).
	Reload bool
	// TUIOnly: tmux sidebar options (look, avatar, placement) that mean nothing in
	// the browser; the web settings screen leaves them out.
	TUIOnly bool
}

// Settings is what the settings screen shows: the few options worth
// changing by hand, in display order.
var Settings = []Setting{
	{Key: "@agent_theme", Group: "Look", Label: "Theme", TUIOnly: true, Kind: Choice, Default: "light", Choices: []string{"dark", "light"}},
	{Key: "@agent_avatar", Group: "Look", Label: "Character", TUIOnly: true, Kind: Choice, Default: "off",
		Choices: []string{"slime", "cat", "sprout", "ghost", "robot", "off"}, Help: "the sidebar avatar; off hides it"},
	{Key: "@agent_avatar_lang", Group: "Look", Label: "Language", TUIOnly: true, Kind: Choice, Default: "en", Choices: []string{"vi", "en"}},
	{Key: "@agent_compact", Group: "Look", Label: "Compact panes", Kind: Bool, Default: "off",
		Help: "collapse the dashboard panes; the saved layout is kept and editing is off"},
	{Key: "@sidebar_position", Group: "Sidebar", Label: "Position", TUIOnly: true, Kind: Choice, Default: "left", Choices: []string{"left", "right"}},
	{Key: "@sidebar_width", Group: "Sidebar", Label: "Initial width", TUIOnly: true, Kind: Text, Default: "15%", Help: "starting size only; resize freely with tmux"},
	{Key: "@agent_notice_seconds", Group: "Sidebar", Label: "Notice shown for (seconds)", TUIOnly: true, Kind: Int, Default: "10", Min: 1, Max: 600, Help: "how long a result or error line stays at the bottom of the board"},
	{Key: "@sidebar_auto_create", Group: "Sidebar", Label: "Auto-create in new windows", TUIOnly: true, Kind: Bool, Default: "on"},
	{Key: "@agent-sidebar-default-agent", Group: "Agents", Label: "Default agent for `new`", Kind: Preset, Default: "claude",
		Choices: roles.HarnessNames()},
	{Key: "@agent_model", Group: "Agents", Label: "Model for `new`", Kind: Text, Help: "passed as --model to the selected harness; empty uses harness default"},
	{Key: "@agent_claude_flags", Group: "Agents", Label: "Extra flags for claude agents", Kind: Text, Help: "appended to every claude launch, e.g. --strict-mcp-config (no MCP servers); empty = none"},
	{Key: "@agent_stuck_minutes", Group: "Agents", Label: "Stuck after (minutes)", Kind: Int, Default: "10", Min: 1, Max: 240},
	{Key: "@agent_blocked_hours", Group: "Agents", Label: "Blocked alert after (hours)", Kind: Int, Default: "24", Min: 1, Max: 720, Help: "notify once when a ticket waits on a human this long"},
	{Key: "@sidebar_notifications", Group: "Notifications", Label: "Desktop notifications", Kind: Bool, Default: "on"},
	{Key: "@agent_notify_url", Group: "Notifications", Label: "Phone push URL", Kind: Text,
		Help: "Phone alerts and task-status updates: ntfy or plain-text POST webhook URL; empty = off"},
	{Key: "@agent_web", Group: "Web", Label: "Web UI", Kind: Bool, Default: "off",
		Help: "serve the browser UI from the background process; takes effect after `tagent reload`; URL in `tmux show -gv @agent_web_url`"},
	{Key: "@agent_web_addr", Group: "Web", Label: "Web address", Kind: Text, Default: "127.0.0.1:7777",
		Help: "0.0.0.0:7777 opens it to the network (plain HTTP: use a tunnel or VPN)"},
	{Key: "@agent_web_token", Group: "Web", Label: "Web token", Kind: Text,
		Help: "access token; empty = random, saved on first start"},
}

// Advanced options are not in the screen; `tagent config set|get|list`
// still handles them.
var Advanced = []Setting{
	{Key: "@agent_digest_llm", Group: "Avatar", Label: "LLM for its lines", Kind: Preset, Default: "ollama run --nowordwrap --think=false qwen3:0.6b",
		Choices: []string{"ollama run --nowordwrap --think=false qwen3:0.6b", "ollama run --nowordwrap --think=false --keepalive 30m gemma4:e2b", "claude -p --model haiku", "off"},
		Help:    "rewrites the avatar's line on every agent event (prompt = last argument); a small local model is enough; off = built-in phrases"},
	{Key: "@agent_avatar_personality", Group: "Avatar", Label: "Personality", Kind: Preset, Default: "cheerful, cute and encouraging",
		Choices: []string{"cheerful, cute and encouraging", "anime tsundere: grumbles but secretly cares", "lazy office cat, always sleepy",
			"witty and lightly sarcastic", "serious and brief, like a professional assistant", "adventurous pirate", "reliable senpai"},
		Help: "how the avatar talks; pick one or type your own description"},
	{Key: "@agent_avatar_llm", Group: "Avatar", Label: "LLM for dashboard summaries", Kind: Preset, Default: "claude -p --model haiku",
		Choices: []string{"claude -p --model haiku", "claude -p --model sonnet", "codex exec", "gemini -p", "off"},
		Help:    "one-line task summaries in the dashboard (prompt = last argument); off = none"},
	{Key: "@agent_avatar_lines", Group: "Avatar", Label: "Speech lines", Kind: Int, Default: "1", Min: 1, Max: 8},
	{Key: "@agent_done_prompt", Group: "Avatar", Label: "Prompt for a finished agent", Kind: Text,
		Help: "quick action (space / prefix+a) sends this to an agent that finished; empty = jump to it"},
	{Key: "@agent_scrape", Group: "Agents", Label: "Screen detection", Kind: Bool, Default: "on", Help: "status for agents without hooks"},
	{Key: "@agent_checkpoints", Group: "Agents", Label: "Checkpoint before each turn", Kind: Bool, Default: "on",
		Help: "snapshot the repo before every agent turn so `tagent undo <agent>` can roll it back"},
	{Key: "@sidebar_key", Group: "Keys", Label: "Toggle sidebar (prefix +)", Kind: Text, Default: "e", Reload: true},
	{Key: "@sidebar_key_all", Group: "Keys", Label: "Toggle all sidebars (prefix +)", Kind: Text, Default: "E", Reload: true},
	{Key: "@agent_watch_key", Group: "Keys", Label: "Dashboard (prefix +)", Kind: Text, Default: "A", Reload: true},
	{Key: "@agent_action_key", Group: "Keys", Label: "Quick action (prefix +)", Kind: Text, Default: "a", Reload: true},
}

// all is every known setting, shown first.
func all() []Setting { return append(append([]Setting{}, Settings...), Advanced...) }

// Find returns the setting for key: the full option (@agent_avatar), or
// without the "@", "@agent_" or "@sidebar_" prefix (agent_avatar, avatar).
func Find(key string) (Setting, bool) {
	key = strings.TrimPrefix(key, "@")
	if key == "default_agent" {
		key = "agent-sidebar-default-agent"
	}
	for _, s := range all() {
		k := strings.TrimPrefix(s.Key, "@")
		if k == key || "agent_"+key == k || "sidebar_"+key == k {
			return s, true
		}
	}
	return Setting{}, false
}

// Get returns a setting's current value (its default when unset).
func Get(s Setting) string {
	if v, ok := tmux.GetOption(s.Key); ok {
		return v
	}
	return s.Default
}

// Validate normalizes value for s, or says why it's not allowed.
func Validate(s Setting, value string) (string, error) {
	value = strings.TrimSpace(value)
	switch s.Kind {
	case Bool:
		switch strings.ToLower(value) {
		case "on", "true", "yes", "1":
			return "on", nil
		case "off", "false", "no", "0":
			return "off", nil
		}
		return "", fmt.Errorf("%s: want on or off", s.Key)
	case Choice:
		for _, c := range s.Choices {
			if c == value {
				return value, nil
			}
		}
		return "", fmt.Errorf("%s: want one of %s", s.Key, strings.Join(s.Choices, ", "))
	case Int:
		n, err := strconv.Atoi(value)
		if err != nil || n < s.Min || n > s.Max {
			return "", fmt.Errorf("%s: want a number %d–%d", s.Key, s.Min, s.Max)
		}
		return strconv.Itoa(n), nil
	}
	return value, nil
}

// Set validates, saves to the config file (the source of truth) and mirrors
// the value into a running tmux, which may be absent.
func Set(s Setting, value string) (string, error) {
	v, err := Validate(s, value)
	if err != nil {
		return "", err
	}
	if err := persist(s.Key, &v); err != nil {
		return "", err
	}
	if v == "" && s.Kind == Text {
		tmux.RunTmux("set", "-gu", s.Key)
	} else {
		tmux.RunTmux("set", "-g", s.Key, v)
	}
	return v, nil
}

// Reset drops the setting back to its default, everywhere.
func Reset(s Setting) error {
	if err := persist(s.Key, nil); err != nil {
		return err
	}
	if s.Default != "" {
		tmux.RunTmux("set", "-g", s.Key, s.Default)
	} else {
		tmux.RunTmux("set", "-gu", s.Key)
	}
	return nil
}

// ConfigFile is where settings persist.
func ConfigFile() string { return settings.Path() }

const fileHeader = "# tagent settings — written by `tagent config` (tmux syntax).\n# Loaded by the plugin before its defaults; edit freely.\n"

// persist rewrites ConfigFile with key set to *value (nil = removed),
// keeping every other line.
func persist(key string, value *string) error {
	path := ConfigFile()
	var lines []string
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			l := sc.Text()
			if f := strings.Fields(l); len(f) >= 3 && (f[0] == "set" || f[0] == "set-option") && f[2] == key {
				continue
			}
			lines = append(lines, l)
		}
		f.Close()
	}
	if len(lines) == 0 {
		lines = strings.Split(strings.TrimRight(fileHeader, "\n"), "\n")
	}
	if value != nil {
		lines = append(lines, fmt.Sprintf("set -g %s %s", key, quote(*value)))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// quote makes value one tmux word.
func quote(v string) string { return settings.Quote(v) }

// Seed writes the config file with every setting's default when there is no
// file yet (a first install); an existing file is never touched. It reports
// whether it wrote one.
func Seed() (bool, error) {
	path := ConfigFile()
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	lines := strings.Split(strings.TrimRight(fileHeader, "\n"), "\n")
	for _, s := range all() {
		if s.Default != "" {
			lines = append(lines, fmt.Sprintf("set -g %s %s", s.Key, quote(s.Default)))
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

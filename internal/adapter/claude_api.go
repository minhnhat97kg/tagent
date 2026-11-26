package adapter

import "tagent/internal/event"

// ClaudeAPIAdapter is the claude adapter for a second Claude Code install (the
// "Claude API" harness, CLAUDE_CONFIG_DIR=~/.claude-api). It parses exactly like
// claude but labels its events "claude-api", so the two installs can be told
// apart while every other part of tagent treats both as claude (IsClaude).
type ClaudeAPIAdapter struct{ ClaudeAdapter }

// ClaudeAPIAgentName is the agent label of the second Claude Code install.
const ClaudeAPIAgentName = "claude-api"

func init() { register(ClaudeAPIAdapter{}) }

func (ClaudeAPIAdapter) Name() string { return ClaudeAPIAgentName }

func (a ClaudeAPIAdapter) Parse(externalName string, payload map[string]any) (event.Event, bool) {
	evt, ok := a.ClaudeAdapter.Parse(externalName, payload)
	if ok && evt.Agent == claudeAgentName {
		evt.Agent = ClaudeAPIAgentName
	}
	return evt, ok
}

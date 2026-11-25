package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tagent/internal/quota"
	"tagent/internal/tasks"
)

// routeCase stubs the quota and the routing config for one dispatch.
func routeCase(t *testing.T, routing string, snap quota.Snapshot) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "tagent"), 0700)
	if err := os.WriteFile(filepath.Join(dir, "tagent", "routing.yaml"), []byte(routing), 0600); err != nil {
		t.Fatal(err)
	}
	old := quotaGet
	quotaGet = func() quota.Snapshot { return snap }
	t.Cleanup(func() { quotaGet = old })
}

const tierRouting = `tiers:
  fast: [claude:haiku, pi:openai-codex/gpt-6-luna]
`

func claudeFull(used float64) quota.Snapshot {
	reset := []byte(`"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"`)
	return quota.Snapshot{Claude: quota.Provider{Five: quota.Window{Used: &used, Reset: reset}}}
}

func tierAction(t *testing.T, roles string) Action {
	t.Helper()
	cfg, err := tasks.ParseBoardConfig([]byte(strings.Replace(jiraConfig, "roles: {implementer: {agent: pi, model: chosen}}", roles, 1)))
	if err != nil {
		t.Fatal(err)
	}
	a, err := Actions(t.TempDir(), cfg, tasks.Event{Kind: "status", From: "todo", To: "build", Actor: "human"}, tasks.Task{ID: "1"})
	if err != nil || len(a) != 1 {
		t.Fatal(a, err)
	}
	return a[0]
}

func TestTierRoleFollowsQuota(t *testing.T) {
	routeCase(t, tierRouting, claudeFull(93))
	a := tierAction(t, "roles: {implementer: {tier: fast, effort: minimal}}")
	if a.Role != "pi" || a.Model != "openai-codex/gpt-6-luna" || a.Tier != "fast" {
		t.Fatalf("%+v", a)
	}
	if !strings.Contains(a.Route, "tier fast (Claude 5h 93%)") || a.Effort != "minimal" {
		t.Fatalf("route %q effort %q", a.Route, a.Effort)
	}
}

func TestTierUnknownQuotaIsAvailable(t *testing.T) {
	routeCase(t, tierRouting, quota.Snapshot{})
	a := tierAction(t, "roles: {implementer: {tier: fast, effort: minimal}}")
	if a.Role != "claude" || a.Model != "haiku" || a.Effort != "" || a.BlockReason != "" {
		t.Fatalf("%+v", a)
	}
}

func TestStepModelWinsOverQuota(t *testing.T) {
	routeCase(t, tierRouting, claudeFull(99))
	cfg, _ := tasks.ParseBoardConfig([]byte(strings.Replace(jiraConfig, "roles: {implementer: {agent: pi, model: chosen}}", "roles: {implementer: {tier: fast}}", 1)))
	cfg.Statuses[1].Model = "override"
	a, err := Actions(t.TempDir(), cfg, tasks.Event{Kind: "status", From: "todo", To: "build", Actor: "human"}, tasks.Task{ID: "1"})
	if err != nil || len(a) != 1 || a[0].Model != "override" || a[0].Role != "claude" || a[0].Route != "step model" || a[0].BlockReason != "" {
		t.Fatal(a, err)
	}
}

func TestTierBlocksWhenAllExhausted(t *testing.T) {
	snap := claudeFull(95)
	snap.Codex = quota.Provider{Five: quota.Window{Used: snap.Claude.Five.Used}}
	routeCase(t, tierRouting, snap)
	a := tierAction(t, "roles: {implementer: {tier: fast}}")
	if !strings.Contains(a.BlockReason, "Claude 5h 95%") || a.Trigger != -1 {
		t.Fatalf("%+v", a)
	}
}

func TestMentionOnExhaustedTierIsSuppressed(t *testing.T) {
	snap := claudeFull(99)
	snap.Codex = quota.Provider{Five: quota.Window{Used: snap.Claude.Five.Used, Reset: snap.Claude.Five.Reset}}
	routeCase(t, tierRouting, snap)
	cfg, _ := tasks.ParseBoardConfig([]byte(`roles: {implementer: {tier: fast}}
triggers:
  - on: mention
    when: {to: '*'}
    agent: '{{event.name}}'
    prompt: hi
`))
	a, err := Actions(t.TempDir(), cfg, tasks.Event{Kind: "mention", Actor: "human", Names: []string{"implementer"}}, tasks.Task{ID: "1"})
	if err != nil || len(a) != 1 || a[0].Trigger != 0 || a[0].BlockReason == "" {
		t.Fatal(a, err)
	}
}

func TestCommentTriggerOnExhaustedTierBlocks(t *testing.T) {
	snap := claudeFull(99)
	snap.Codex = quota.Provider{Five: quota.Window{Used: snap.Claude.Five.Used, Reset: snap.Claude.Five.Reset}}
	routeCase(t, tierRouting, snap)
	cfg, _ := tasks.ParseBoardConfig([]byte(`roles: {implementer: {tier: fast}}
triggers:
  - on: comment
    when: {to: '*'}
    agent: implementer
    prompt: hi
`))
	a, err := Actions(t.TempDir(), cfg, tasks.Event{Kind: "comment", Actor: "human"}, tasks.Task{ID: "1"})
	if err != nil || len(a) != 1 || a[0].Event.Kind != "comment" || a[0].BlockReason == "" {
		t.Fatal(a, err)
	}
}

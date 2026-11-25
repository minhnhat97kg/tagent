package tasks

import (
	"strings"
	"testing"
)

func TestRoleContextArgs(t *testing.T) {
	none, one := []string{}, []string{"tagent"}
	cases := []struct {
		name, harness string
		rc            RoleContext
		want          string // joined with spaces; "ERR" when Args must fail
	}{
		{"claude empty", "claude", RoleContext{}, ""},
		{"claude no skills no mcp", "claude", RoleContext{Skills: &none, MCP: &none}, "--disable-slash-commands --strict-mcp-config"},
		{"claude mcp list", "claude", RoleContext{MCP: &one}, "--strict-mcp-config --mcp-config=/f.json"},
		{"claude tools allow deny", "claude", RoleContext{Tools: []string{"Read", "Grep"}, Allow: []string{"a"}, Deny: []string{"b"}}, "--tools=Read,Grep --allowedTools=a --disallowedTools=b"},
		{"claude skill list", "claude", RoleContext{Skills: &one}, "ERR"},
		{"pi none", "pi", RoleContext{Skills: &none, MCP: &none, Tools: []string{"read"}, Deny: []string{"bash"}}, "--no-skills --no-mcp --tools=read --exclude-tools=bash"},
		{"pi mcp list", "pi", RoleContext{MCP: &one}, "ERR"},
		{"pi allow", "pi", RoleContext{Allow: []string{"x"}}, "ERR"},
		{"codex tools", "codex", RoleContext{Tools: []string{"read"}}, "ERR"},
		{"codex empty", "codex", RoleContext{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args, err := c.rc.Args(c.harness, "/f.json")
			if c.want == "ERR" {
				if err == nil {
					t.Fatalf("want error, got %v", args)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(args, " "); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestScoutContextIsReadOnlyTicketAccess(t *testing.T) {
	args, err := ScoutContext().Args("claude", "/f.json")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--disable-slash-commands", "--strict-mcp-config", "--tools=Read,Grep,Glob,mcp__tagent__task_show,mcp__tagent__task_move,mcp__tagent__task_comment", "--allowedTools=mcp__tagent__task_show,mcp__tagent__task_move,mcp__tagent__task_comment", "--disallowedTools=mcp__tagent__task_add"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
	if strings.Contains(joined, "Edit") || strings.Contains(joined, "Bash") {
		t.Fatalf("scout can write: %s", joined)
	}
}

package skill

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRenderFillsBinary(t *testing.T) {
	got := Render("/opt/bin/tagent")
	if strings.Contains(got, "{{BIN}}") || !strings.HasPrefix(got, "---\nname: tagent") || !strings.Contains(got, "/opt/bin/tagent task move 12") {
		t.Fatalf("bad skill:\n%s", got)
	}
}

// The skill is loaded into every agent's context, so it stays short and keeps
// its safety rules first.
func TestSkillIsSlimWithRulesFirst(t *testing.T) {
	got := Render("tagent")
	if n := strings.Count(got, "\n"); n > 50 {
		t.Fatalf("skill is %d lines, want about 40", n)
	}
	rules := strings.Index(got, "## Do NOT")
	example := strings.Index(got, "## Example")
	if rules < 0 || example < 0 || rules > example {
		t.Fatalf("Do NOT section must come before the example:\n%s", got)
	}
	lower := strings.ToLower(got)
	for _, want := range []string{"approve", "sleep", "delete tickets", "tmux_pane", "task_show", "docs/agent-reference.md"} {
		if !strings.Contains(lower, want) {
			t.Errorf("skill missing %q", want)
		}
	}
}

// Harnesses (pi, claude) reject a skill whose front matter isn't valid YAML.
func TestFrontMatterIsYAML(t *testing.T) {
	parts := strings.SplitN(Render("tagent"), "---\n", 3)
	var fm struct{ Name, Description string }
	if len(parts) < 3 || yaml.Unmarshal([]byte(parts[1]), &fm) != nil || fm.Name != "tagent" || fm.Description == "" {
		t.Fatalf("bad front matter: %q", parts[1])
	}
}

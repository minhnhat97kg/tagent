package tasks

import (
	"fmt"
	"sort"

	"tagent/internal/roles"
	"tagent/internal/workflow"
)

// harnesses are the agents tagent can run in a pane.
var harnesses = map[string]bool{"claude": true, "codex": true, "pi": true, "opencode": true, "gemini": true, "copilot": true, "amp": true}

// CheckBoard lists what is wrong with the board and its workflows, one plain
// line each: a role on an unsupported agent, a workflow that does not parse,
// a job whose agent is unknown, a job status that sits in the todo, blocked or
// done lane. The TUI shows the first as a notice and the web puts them in the
// bell, whenever the app opens or a workflow is created.
func CheckBoard(dir string, cfg BoardConfig) []string {
	var out []string
	for name, r := range cfg.Roles {
		if r.Tier == "" && !harnesses[r.Agent] {
			out = append(out, fmt.Sprintf("role %s: unsupported agent %q", name, r.Agent))
		}
	}
	sort.Strings(out)
	for _, w := range workflow.List(MainRoot(dir)) {
		if w.Err != nil {
			out = append(out, fmt.Sprintf("workflow %s: %v", w.Name, w.Err))
			continue
		}
		for _, j := range w.Jobs {
			if j.Agent != "" {
				_, role := cfg.Roles[j.Agent]
				if _, named := roles.GetIn(dir, j.Agent); !role && !named {
					out = append(out, fmt.Sprintf("workflow %s: job %s uses unknown agent %q", w.Name, j.ID, j.Agent))
				}
			}
			if lane := cfg.Lane(j.Status); lane == "todo" || lane == "blocked" || lane == "done" {
				out = append(out, fmt.Sprintf("workflow %s: job %s status %q is in the %s lane", w.Name, j.ID, j.Status, lane))
			}
		}
	}
	return out
}

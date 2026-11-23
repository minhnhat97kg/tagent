package intake

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"tagent/internal/scrape"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
)

// liveSessions is the tmux view used to recheck targets at approval.
var liveSessions = tmux.QuerySessions

// SetLiveSessions replaces the target view for tests; call the result to restore it.
func SetLiveSessions(f func() []tmux.SessionInfo) (restore func()) {
	old := liveSessions
	liveSessions = f
	return func() { liveSessions = old }
}

type Recipient struct {
	Name, Pane, Role string
	Identity         string `json:"-"`
	Approval         string `json:"-"`
}

func Recipients(dir string, sessions []tmux.SessionInfo) []Recipient {
	out := []Recipient{}
	for _, s := range sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if tasks.Scope(p.Path) != tasks.Scope(dir) {
					continue
				}
				out = append(out, Recipient{p.Name, p.PaneID, tmux.GetPaneOptionValue(p.PaneID, "@agent_role"), fmt.Sprintf("%d:%d:%s", p.PanePID, p.StartedAt, p.Agent), string(p.Status) + ":" + p.WaitReason + ":" + p.Prompt})
			}
		}
	}
	return out
}
func freezeCall(dir string, c *ToolCall) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if project := c.Args["project"]; project != "" {
		info, err := os.Stat(project)
		if err != nil || !info.IsDir() || tasks.Scope(project) != tasks.Scope(dir) {
			return fmt.Errorf("project must be an existing directory in the current task scope")
		}
	}
	if name := c.Args["workflow"]; name != "" {
		w, err := workflow.Find(tasks.MainRoot(dir), name)
		if err != nil {
			return err
		}
		if c.Name == "run_workflow" {
			var inputs map[string]string
			if v := c.Args["inputs"]; v != "" {
				if err = json.Unmarshal([]byte(v), &inputs); err != nil {
					return err
				}
			}
			for k, v := range w.Inputs {
				if v.Required && v.Default == "" && inputs[k] == "" {
					return fmt.Errorf("workflow requires input %s", k)
				}
			}
			for k := range inputs {
				if _, ok := w.Inputs[k]; !ok {
					return fmt.Errorf("unknown workflow input %s", k)
				}
			}
		}
	}
	if id := c.Args["id"]; id != "" {
		if _, err := tasks.Get(tasks.Scope(dir), id); err != nil {
			// The model sometimes makes up an id for a ticket that does not exist yet.
			return fmt.Errorf("no ticket %q: use list_tickets for real ids, or create_ticket for new work", id)
		}
	}
	if c.Args["pane"] == "" {
		return nil
	}
	recipients := Recipients(dir, liveSessions())
	if c.Name == "answer_approval" {
		screen, ok := tmux.RunTmux("capture-pane", "-p", "-t", c.Args["pane"])
		if !ok {
			return fmt.Errorf("cannot inspect pending approval")
		}
		dialog, ok := scrape.ParseDialog(screen)
		if !ok {
			return fmt.Errorf("agent has no current approval dialog")
		}
		found := false
		for _, o := range dialog.Options {
			if o.Key == c.Args["option"] {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("option is not in the current approval")
		}
		b, _ := json.Marshal(dialog)
		for i := range recipients {
			if recipients[i].Pane == c.Args["pane"] {
				recipients[i].Approval = "waiting:" + string(b)
			}
		}
	}
	return validateRecipient(c, recipients)
}
func validateRecipient(c *ToolCall, recipients []Recipient) error {
	if dest := c.Args["destination"]; dest != "" {
		found := false
		for _, r := range recipients {
			if r.Pane == dest {
				if c.DestinationIdentity != "" && c.DestinationIdentity != r.Identity {
					return fmt.Errorf("destination changed; propose again")
				}
				c.DestinationIdentity = r.Identity
				found = true
			}
		}
		if !found || dest == c.Args["pane"] {
			return fmt.Errorf("handoff destination must be another live agent pane in this project")
		}
	}
	for _, r := range recipients {
		if r.Pane == c.Args["pane"] {
			if !c.ReadOnly() && (r.Identity == "" || strings.HasPrefix(r.Identity, "0:0:")) {
				return fmt.Errorf("cannot establish stable agent identity; inspect and retry")
			}
			if c.Identity != "" && c.Identity != r.Identity {
				return fmt.Errorf("agent changed; propose again")
			}
			c.Identity = r.Identity
			if c.Name == "answer_approval" {
				if !strings.HasPrefix(r.Approval, "waiting:") {
					return fmt.Errorf("agent is not waiting for approval")
				}
				if c.Approval != "" && c.Approval != r.Approval {
					return fmt.Errorf("approval changed; propose again")
				}
				c.Approval = r.Approval
			}
			return nil
		}
	}
	return fmt.Errorf("agent %s is no longer running in this project", c.Args["pane"])
}

// MentionCalls routes the entire prompt to exact, distinct recipients. Code and emails are not addresses.
func MentionCalls(input string, recipients []Recipient) ([]ToolCall, string, error) {
	var names []string
	var text []string
	code := false
	last := 0
	for _, loc := range regexp.MustCompile(`\S+`).FindAllStringIndex(input, -1) {
		word := input[loc[0]:loc[1]]
		if !code && len(word) > 1 && strings.HasPrefix(word, "@") && !strings.Contains(word, "`") {
			names = append(names, strings.TrimPrefix(word, "@"))
			text = append(text, input[last:loc[0]])
			last = loc[1]
		}
		if strings.Count(word, "`")%2 == 1 {
			code = !code
		}
	}
	text = append(text, input[last:])
	if len(names) == 0 {
		return nil, input, nil
	}
	prompt := strings.TrimSpace(strings.Join(text, ""))
	if prompt == "" {
		return nil, "", fmt.Errorf("include a prompt after the mention")
	}
	seen := map[string]bool{}
	calls := []ToolCall{}
	for _, name := range names {
		var matches []Recipient
		for _, r := range recipients {
			if name == r.Pane || name == r.Name || name == r.Role {
				matches = append(matches, r)
			}
		}
		if len(matches) != 1 {
			return nil, "", fmt.Errorf("@%s resolves to %d agents; use an exact @%%pane", name, len(matches))
		}
		r := matches[0]
		if seen[r.Pane] {
			continue
		}
		seen[r.Pane] = true
		if len(calls) >= 16 {
			return nil, "", fmt.Errorf("mention at most 16 distinct agents per approval batch")
		}
		calls = append(calls, ToolCall{Name: "send_prompt", Args: map[string]string{"pane": r.Pane, "prompt": prompt}, Identity: r.Identity})
	}
	return calls, prompt, nil
}

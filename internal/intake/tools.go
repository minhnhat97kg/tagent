package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/usage"
	"time"
)

// ToolCall is a validated, frozen application operation, never a shell command.
type ToolCall struct {
	Name                string            `json:"name"`
	Args                map[string]string `json:"args"`
	Identity            string            `json:"identity,omitempty"`
	DestinationIdentity string            `json:"destinationIdentity,omitempty"`
	Approval            string            `json:"approval,omitempty"`
}
type toolSpec struct {
	read               bool
	required, optional string
}

var toolSpecs = map[string]toolSpec{
	"list_agents": {true, "", ""}, "inspect_agent": {true, "pane", ""},
	"start_agent": {false, "harness role", "worktree project"}, "send_prompt": {false, "pane prompt", ""},
	"answer_approval": {false, "pane option", ""}, "kill_agent": {false, "pane", ""},
	"handoff": {false, "pane destination", ""}, "resize": {false, "pane width height", ""},
	"list_tickets": {true, "", ""}, "create_ticket": {false, "title goal", "workflow"},
	"move_ticket": {false, "id status", "comment"}, "comment_ticket": {false, "id comment", ""},
	"start_ticket": {false, "id workflow", ""}, "run_workflow": {false, "workflow", "inputs"},
	"board_summary": {true, "", ""}, "usage_summary": {true, "", ""}, "open_view": {true, "view", "pane"},
}

func (c ToolCall) Validate() error {
	s, ok := toolSpecs[c.Name]
	if !ok {
		return fmt.Errorf("unknown tool %q", c.Name)
	}
	allowed := " " + s.required + " " + s.optional + " "
	total := 0
	for _, v := range c.Args {
		total += len(v)
	}
	if total > 16000 {
		return fmt.Errorf("tool arguments exceed 16000 bytes")
	}
	for k, v := range c.Args {
		if !strings.Contains(allowed, " "+k+" ") || len(v) > 16000 || strings.ContainsRune(v, 0) {
			return fmt.Errorf("invalid argument %s", k)
		}
	}
	for _, k := range strings.Fields(s.required) {
		if strings.TrimSpace(c.Args[k]) == "" {
			return fmt.Errorf("%s requires %s", c.Name, k)
		}
	}
	for _, k := range []string{"pane", "destination"} {
		if v := c.Args[k]; v != "" {
			if len(v) < 2 || v[0] != '%' {
				return fmt.Errorf("%s must be an exact pane ID", k)
			}
			if _, e := strconv.ParseUint(v[1:], 10, 32); e != nil {
				return e
			}
		}
	}
	for _, k := range []string{"width", "height"} {
		if v := c.Args[k]; v != "" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 2 || n > 1000 {
				return fmt.Errorf("invalid %s", k)
			}
		}
	}
	for _, k := range []string{"id", "status", "workflow", "harness", "role", "option"} {
		if strings.HasPrefix(c.Args[k], "-") {
			return fmt.Errorf("invalid %s", k)
		}
	}
	if c.Name == "open_view" && c.Args["view"] == "agent" && c.Args["pane"] == "" {
		return fmt.Errorf("agent view requires pane")
	}
	if c.Name == "open_view" && c.Args["view"] != "chat" && c.Args["view"] != "board" && c.Args["view"] != "agent" {
		return fmt.Errorf("unknown view")
	}
	if v := c.Args["worktree"]; v != "" && v != "true" && v != "false" {
		return fmt.Errorf("worktree must be true or false")
	}
	return nil
}
func (c ToolCall) ReadOnly() bool  { return toolSpecs[c.Name].read }
func (c ToolCall) Summary() string { b, _ := json.Marshal(c); return string(b) }

type boundedToolOutput struct{ bytes.Buffer }

func (b *boundedToolOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 16000 - b.Len(); remaining > 0 {
		_, _ = b.Buffer.Write(p[:min(len(p), remaining)])
	}
	return n, nil
}

// ToolExecutor is injected by tests; production invokes validated argv only.
type ToolExecutor func(context.Context, string, string, []string) (string, error)

func executeToolProcess(ctx context.Context, dir, binary string, args []string) (string, error) {
	if binary == "tagent" {
		var err error
		binary, err = os.Executable()
		if err != nil {
			return "", err
		}
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	var output boundedToolOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return output.String(), err
}
func ExecuteTool(ctx context.Context, dir string, c ToolCall, execute ToolExecutor) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	if execute == nil {
		if err := freezeCall(dir, &c); err != nil {
			return "", err
		}
		if c.Name == "list_agents" {
			b, _ := json.Marshal(Recipients(dir, tmux.QuerySessions()))
			return string(b), nil
		}
		if c.Name == "usage_summary" {
			values := map[string]any{}
			for _, s := range tmux.QuerySessions() {
				for _, w := range s.Windows {
					for _, p := range w.Panes {
						if tasks.Scope(p.Path) == tasks.Scope(dir) {
							count, ok := usage.Read(string(p.Agent), p.SessionID)
							values[p.PaneID] = map[string]any{"available": ok, "usage": count}
						}
					}
				}
			}
			b, _ := json.Marshal(values)
			return string(b), nil
		}
		execute = executeToolProcess
	}
	a := c.Args
	binary := "tagent"
	var argv []string
	switch c.Name {
	case "list_agents":
		argv = []string{"list"}
	case "inspect_agent":
		argv = []string{"read", a["pane"]}
	case "start_agent":
		project := dir
		if a["project"] != "" {
			project = a["project"]
		}
		argv = []string{"new", "--agent", a["harness"], "--role", a["role"], "--cwd", project, "--no-wait"}
		if a["worktree"] == "true" {
			argv = append(argv, "--worktree")
		}
	case "send_prompt":
		argv = []string{"send", a["pane"], a["prompt"]}
	case "answer_approval":
		argv = []string{"answer", a["pane"], a["option"]}
	case "kill_agent":
		argv = []string{"kill", a["pane"]}
	case "handoff":
		binary = "tmux"
		argv = []string{"join-pane", "-s", a["pane"], "-t", a["destination"]}
	case "resize":
		binary = "tmux"
		argv = []string{"resize-pane", "-t", a["pane"], "-x", a["width"], "-y", a["height"]}
	case "list_tickets", "board_summary":
		argv = []string{"task", "ls"}
	case "usage_summary":
		argv = []string{"tokens"}
	case "create_ticket":
		argv = []string{"task", "add"}
		if a["workflow"] != "" {
			argv = append(argv, "--workflow", a["workflow"])
		}
		argv = append(argv, a["title"]+"\n\n"+a["goal"])
	case "move_ticket":
		argv = []string{"task", "move", a["id"], a["status"]}
		if a["comment"] != "" {
			argv = append(argv, a["comment"])
		}
	case "comment_ticket":
		argv = []string{"task", "comment", a["id"], a["comment"]}
	case "start_ticket":
		argv = []string{"task", "start", a["id"], a["workflow"]}
	case "run_workflow":
		argv = []string{"workflow", "run", a["workflow"]}
		var inputs map[string]string
		if a["inputs"] != "" {
			if err := json.Unmarshal([]byte(a["inputs"]), &inputs); err != nil {
				return "", err
			}
		}
		for k, v := range inputs {
			argv = append(argv, "--input", k+"="+v)
		}
	case "open_view":
		if a["view"] == "agent" {
			return "#/agent/" + url.PathEscape(a["pane"]), nil
		}
		if a["view"] == "board" {
			return "#/", nil
		}
		return "#/" + a["view"], nil
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return execute(ctx, dir, binary, argv)
}

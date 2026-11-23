package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

func directRequest(dir, input string, recipients []Recipient) (Result, bool, error) {
	calls, _, err := MentionCalls(input, recipients)
	if err != nil {
		return Result{}, true, err
	}
	if len(calls) == 0 {
		words := strings.Fields(input)
		if len(words) == 0 || !strings.HasPrefix(words[0], "/") {
			return Result{}, false, nil
		}
		switch words[0] {
		case "/agents":
			calls = []ToolCall{{Name: "list_agents"}}
		case "/board":
			calls = []ToolCall{{Name: "board_summary"}}
		case "/new":
			if len(words) < 3 {
				return Result{Question: "Which harness and role should I start? Use /new <harness> <role> [--worktree] in this project."}, true, nil
			}
			if len(words) > 4 || len(words) == 4 && words[3] != "--worktree" {
				return Result{}, true, fmt.Errorf("use /new <harness> <role> [--worktree]")
			}
			args := map[string]string{"harness": words[1], "role": words[2]}
			if len(words) == 4 {
				args["worktree"] = "true"
			}
			calls = []ToolCall{{Name: "start_agent", Args: args}}
		case "/run":
			if len(words) < 2 {
				return Result{Question: "Which workflow should I run? Use /run <workflow>."}, true, nil
			}
			w, e := workflow.Find(tasks.MainRoot(dir), words[1])
			if e != nil {
				return Result{}, true, e
			}
			inputs := map[string]string{}
			for _, item := range words[2:] {
				k, v, ok := strings.Cut(item, "=")
				if !ok {
					return Result{}, true, fmt.Errorf("workflow inputs use key=value")
				}
				inputs[k] = v
			}
			for name, v := range w.Inputs {
				if v.Required && v.Default == "" && inputs[name] == "" {
					return Result{Question: fmt.Sprintf("Workflow %s requires input %s. Supply its value before running.", w.Name, name)}, true, nil
				}
			}
			b, _ := json.Marshal(inputs)
			calls = []ToolCall{{Name: "run_workflow", Args: map[string]string{"workflow": words[1], "inputs": string(b)}}}
		default:
			return Result{}, true, fmt.Errorf("unknown command; use /agents, /new, /board, /run <workflow>, /clear")
		}
	}
	for i := range calls {
		if err := calls[i].Validate(); err != nil {
			return Result{}, true, err
		}
		if calls[i].Args["pane"] == "" {
			if err := freezeCall(dir, &calls[i]); err != nil {
				return Result{}, true, err
			}
		}
	}
	if len(calls) == 1 && calls[0].ReadOnly() {
		out, e := ExecuteTool(context.Background(), dir, calls[0], nil)
		return Result{Answer: out}, true, e
	}
	return Result{Calls: calls}, true, nil
}

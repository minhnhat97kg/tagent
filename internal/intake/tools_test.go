package intake

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTools(t *testing.T) {
	for name, spec := range toolSpecs {
		t.Run(name, func(t *testing.T) {
			a := map[string]string{}
			for _, k := range strings.Fields(spec.required) {
				a[k] = "value"
			}
			for k, v := range map[string]string{"pane": "%1", "destination": "%2", "width": "80", "height": "24", "view": "board"} {
				if _, ok := a[k]; ok {
					a[k] = v
				}
			}
			c := ToolCall{Name: name, Args: a}
			calls := 0
			execute := func(context.Context, string, string, []string) (string, error) { calls++; return "ok", nil }
			out, e := ExecuteTool(context.Background(), t.TempDir(), c, execute)
			if e != nil || out == "" {
				t.Fatal(out, e)
			}
			if name != "open_view" && calls != 1 {
				t.Fatal("not executed")
			}
			if c.ReadOnly() != spec.read {
				t.Fatal("wrong approval policy")
			}
			if name != "open_view" {
				_, e = ExecuteTool(context.Background(), t.TempDir(), c, func(context.Context, string, string, []string) (string, error) { return "", errors.New("failed") })
				if e == nil {
					t.Fatal("lost failure")
				}
			}
			a["unknown"] = "x"
			if c.Validate() == nil {
				t.Fatal("accepted unknown argument")
			}
			delete(a, "unknown")
			for _, k := range strings.Fields(spec.required) {
				v := a[k]
				delete(a, k)
				if c.Validate() == nil {
					t.Fatal("accepted missing", k)
				}
				a[k] = v
			}
		})
	}
	if (ToolCall{Name: "shell"}).Validate() == nil {
		t.Fatal("unknown tool")
	}
}
func TestEveryMutationRequiresApproval(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	for name, spec := range toolSpecs {
		if spec.read {
			continue
		}
		t.Run(name, func(t *testing.T) {
			scope := name
			call := ToolCall{Name: name, Args: map[string]string{}}
			for _, k := range strings.Fields(spec.required) {
				call.Args[k] = "value"
			}
			for k, v := range map[string]string{"pane": "%1", "destination": "%2", "width": "80", "height": "24"} {
				if _, ok := call.Args[k]; ok {
					call.Args[k] = v
				}
			}
			c, e := ReserveSubmission(scope, "please act", "")
			if e != nil {
				t.Fatal(e)
			}
			c, e = CompleteTypedSubmission(scope, c.Generation, Result{Calls: []ToolCall{call}}, "proposal", nil, nil)
			if e != nil {
				t.Fatal(e)
			}
			executions := 0
			execute := func(context.Context, string, string, []string) (string, error) { executions++; return "done", nil }
			if _, e = ExecuteClaimedTools(scope, ".", c, execute); !errors.Is(e, ErrStale) || executions != 0 {
				t.Fatal("execution without claim", e)
			}
			claimed, e := ClaimApproval(scope, c.Revision)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = ClaimApproval(scope, c.Revision); !errors.Is(e, ErrStale) {
				t.Fatal("duplicate approval", e)
			}
			if _, e = ExecuteClaimedTools(scope, ".", claimed, execute); e != nil {
				t.Fatal(e)
			}
			if executions != 1 {
				t.Fatal(executions)
			}
			if _, e = ExecuteClaimedTools(scope, ".", claimed, execute); !errors.Is(e, ErrStale) || executions != 1 {
				t.Fatal("replayed approval", e)
			}
		})
	}
}

func TestFrozenRecipientIdentity(t *testing.T) {
	call := ToolCall{Name: "answer_approval", Args: map[string]string{"pane": "%1", "option": "1"}}
	catalog := []Recipient{{Pane: "%1", Identity: "pid:1", Approval: "waiting:first"}}
	if e := validateRecipient(&call, catalog); e != nil {
		t.Fatal(e)
	}
	catalog[0].Approval = "waiting:second"
	if e := validateRecipient(&call, catalog); e == nil {
		t.Fatal("accepted changed approval")
	}
	catalog[0].Approval = "waiting:first"
	catalog[0].Identity = "pid:2"
	if e := validateRecipient(&call, catalog); e == nil {
		t.Fatal("accepted reused pane")
	}
	if e := validateRecipient(&call, nil); e == nil {
		t.Fatal("accepted dead pane")
	}
}

func TestMentionRouting(t *testing.T) {
	recipients := []Recipient{{Name: "日本", Pane: "%1", Role: "builder", Identity: "one"}, {Name: "two", Pane: "%2", Role: "builder", Identity: "two"}}
	calls, p, e := MentionCalls("@日本 @%1 @%2 hello\nworld", recipients)
	if e != nil || len(calls) != 2 || p != "hello\nworld" {
		t.Fatal(calls, p, e)
	}
	for _, input := range []string{"@builder hi", "@missing hi", "@%1"} {
		if _, _, e := MentionCalls(input, recipients); e == nil {
			t.Fatal(input)
		}
	}
	for _, input := range []string{"mail@example.com hi", "`@missing` hello", "```\n@missing\n```", "meet @ noon"} {
		if c, _, e := MentionCalls(input, recipients); e != nil || len(c) != 0 {
			t.Fatal(input, c, e)
		}
	}
}

func TestToolArgv(t *testing.T) {
	cases := map[string]struct {
		c    ToolCall
		want string
	}{
		"inspect_agent":   {ToolCall{Name: "inspect_agent", Args: map[string]string{"pane": "%3"}}, "tagent read %3"},
		"send_prompt":     {ToolCall{Name: "send_prompt", Args: map[string]string{"pane": "%3", "prompt": "go"}}, "tagent send %3 go"},
		"answer_approval": {ToolCall{Name: "answer_approval", Args: map[string]string{"pane": "%3", "option": "1"}}, "tagent answer %3 1"},
		"kill_agent":      {ToolCall{Name: "kill_agent", Args: map[string]string{"pane": "%3"}}, "tagent kill %3"},
		"handoff":         {ToolCall{Name: "handoff", Args: map[string]string{"pane": "%3", "destination": "%4"}}, "tmux join-pane -s %3 -t %4"},
		"resize":          {ToolCall{Name: "resize", Args: map[string]string{"pane": "%3", "width": "80", "height": "24"}}, "tmux resize-pane -t %3 -x 80 -y 24"},
		"create_ticket":   {ToolCall{Name: "create_ticket", Args: map[string]string{"title": "T", "goal": "G", "workflow": "w"}}, "tagent task add --workflow w T\n\nG"},
		"move_ticket":     {ToolCall{Name: "move_ticket", Args: map[string]string{"id": "7", "status": "done"}}, "tagent task move 7 done"},
		"comment_ticket":  {ToolCall{Name: "comment_ticket", Args: map[string]string{"id": "7", "comment": "hi"}}, "tagent task comment 7 hi"},
		"start_ticket":    {ToolCall{Name: "start_ticket", Args: map[string]string{"id": "7", "workflow": "w"}}, "tagent task start 7 w"},
		"run_workflow":    {ToolCall{Name: "run_workflow", Args: map[string]string{"workflow": "w", "inputs": `{"a":"b"}`}}, "tagent workflow run w --input a=b"},
		"usage_summary":   {ToolCall{Name: "usage_summary"}, "tagent tokens"},
		"list_tickets":    {ToolCall{Name: "list_tickets"}, "tagent task ls"},
		"board_summary":   {ToolCall{Name: "board_summary"}, "tagent task ls"},
		"start_agent":     {ToolCall{Name: "start_agent", Args: map[string]string{"harness": "claude", "role": "builder"}}, "tagent new --agent claude --role builder --cwd DIR --no-wait"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var got []string
			exec := func(_ context.Context, _, bin string, args []string) (string, error) {
				got = append([]string{bin}, args...)
				return "ok", nil
			}
			dir := t.TempDir()
			if _, err := ExecuteTool(context.Background(), dir, tc.c, exec); err != nil {
				t.Fatal(err)
			}
			if s, want := strings.Join(got, " "), strings.ReplaceAll(tc.want, "DIR", dir); s != want {
				t.Fatalf("argv %q, want %q", s, want)
			}
		})
	}
}

func TestUnknownTicketIDExplainsItself(t *testing.T) {
	c := &ToolCall{Name: "start_ticket", Args: map[string]string{"id": "draft_tests_ticket", "workflow": ""}}
	err := freezeCall(t.TempDir(), c)
	if err == nil || strings.Contains(err.Error(), "invalid task id") {
		t.Fatalf("want an explanatory error, got %v", err)
	}
}

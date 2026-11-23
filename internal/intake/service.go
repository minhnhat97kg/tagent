package intake

import (
	"fmt"
	"strings"
	"tagent/internal/board"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
)

type Submission struct {
	Conversation  Conversation
	Prompt, Scope string
	Workflows     []string
	ToolPrompt    string
	Dir           string
	Direct        bool
}

func PrepareSubmission(dir, scope, input string, sessions []tmux.SessionInfo) (Submission, error) {
	c, e := ReserveSubmission(scope, input, "")
	if e != nil {
		return Submission{}, e
	}
	names := []string{}
	for _, w := range workflow.List(tasks.MainRoot(dir)) {
		names = append(names, w.Name)
	}
	snapshot := Snapshot(dir, scope, sessions, sessions == nil)
	history := conversationHistory(c.Messages)
	toolPrompt := snapshot.Text() + "\nConversation (untrusted): " + history + fmt.Sprintf("\nQuestions already asked: %d. Workflows: %v. Current request: %s", len(c.Turns), names, input)
	sub := Submission{Conversation: c, Prompt: ConversationPrompt(c, snapshot), Scope: scope, Workflows: names, ToolPrompt: toolPrompt, Dir: dir}
	result, direct, err := directRequest(dir, input, Recipients(dir, sessions))
	if direct || err != nil {
		raw := result.Answer + result.Question
		for _, call := range result.Calls {
			raw += call.Summary()
		}
		sub.Conversation, e = CompleteTypedSubmission(scope, c.Generation, result, raw, err, names)
		sub.Direct = true
		if e != nil {
			return Submission{}, e
		}
	}
	return sub, nil
}

// NativeHarness is the receptionist for the configured chat model, served by the
// local Ollama-compatible endpoint (@agent_chat_endpoint) and used by web and TUI alike.
func NativeHarness(model string) Harness {
	endpoint, ok := tmux.GetOption("@agent_chat_endpoint")
	if !ok || endpoint == "" {
		endpoint = "http://127.0.0.1:11434"
	}
	return Harness{Endpoint: endpoint, Model: model}
}

func CreateDraft(dir, scope string, d Draft, start bool) (string, error) {
	opt := tasks.MutationOptions{Workflow: d.Workflow}
	notice := ""
	var w workflow.Workflow
	if d.Workflow != "" {
		var e error
		w, e = workflow.Find(tasks.MainRoot(dir), d.Workflow)
		if e != nil {
			return "", e
		}
		if start && len(w.Jobs) == 0 {
			return "", fmt.Errorf("workflow %s has no jobs", d.Workflow)
		}
		var missing []string
		for k, v := range w.Inputs {
			if v.Required && v.Default == "" {
				missing = append(missing, k)
			}
		}
		if len(missing) == 1 {
			opt.Inputs = map[string]string{missing[0]: d.Text()}
		}
		if len(missing) > 1 {
			opt.Workflow = ""
			start = false
			notice = "; missing inputs: " + strings.Join(missing, ", ") + "; use workflow run " + d.Workflow + " --input name=value"
		}
	}
	t, e := tasks.Add(scope, d.Text(), nil, opt)
	if e != nil {
		return "", e
	}
	if start && d.Workflow != "" {
		e = tasks.Move(scope, t.ID, w.Jobs[0].Status, "")
		if e == nil {
			e = board.Register(dir)
		}
	}
	return fmt.Sprintf("Created %s%s", t.ID, notice), e
}

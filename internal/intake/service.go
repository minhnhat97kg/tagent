package intake

import (
	"encoding/json"
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
	history, _ := json.Marshal(c.Messages)
	toolPrompt := snapshot.Text() + "\nConversation (untrusted): " + string(history) + fmt.Sprintf("\nQuestions already asked: %d. Workflows: %v. Current request: %s", len(c.Turns), names, input)
	return Submission{c, ConversationPrompt(c, snapshot), scope, names, toolPrompt}, nil
}
func RunSubmission(s Submission, command string, run func(string, string) (string, error)) (string, error) {
	if run == nil {
		run = Run
	}
	raw, e := run(command, s.Prompt)
	_, finish := CompleteSubmission(s.Scope, s.Conversation.Generation, raw, e, s.Workflows)
	if finish != nil {
		e = finish
	}
	return raw, e
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

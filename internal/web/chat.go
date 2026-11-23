package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"tagent/internal/intake"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
)

type draftJSON struct {
	Title      string   `json:"title"`
	Goal       string   `json:"goal"`
	Acceptance []string `json:"acceptance"`
	OutOfScope []string `json:"outOfScope"`
	Workflow   string   `json:"workflow"`
}

func draftDTO(d intake.Draft) *draftJSON {
	if d.Title == "" {
		return nil
	}
	a, o := d.Acceptance, d.OutOfScope
	if a == nil {
		a = []string{}
	}
	if o == nil {
		o = []string{}
	}
	return &draftJSON{d.Title, d.Goal, a, o, d.Workflow}
}
func (s *server) intakeCommand() string {
	if s.deps.command != nil {
		return s.deps.command()
	}
	c, ok := tmux.GetOption("@agent_chat_model")
	if !ok {
		c = "bonsai-2:9b"
	}
	return c
}
func chatDTO(dir string, enabled bool, c intake.Conversation) any {
	type message struct {
		Role     string `json:"role"`
		Text     string `json:"text"`
		Revision uint64 `json:"revision"`
	}
	messages := []message{}
	for _, m := range c.Messages {
		messages = append(messages, message{m.Role, m.Text, m.Revision})
	}
	actions := []string{}
	if enabled && c.Status == "proposal" {
		if c.Response.Draft.Title != "" {
			actions = []string{"create", "start", "edit", "cancel"}
		} else if c.Response.Command != "" {
			actions = []string{"approve", "edit", "cancel"}
		}
	}
	names := []string{}
	for _, w := range workflow.List(tasks.MainRoot(dir)) {
		names = append(names, w.Name)
	}
	status := c.Status
	if status == "" {
		status = "idle"
	}
	return map[string]any{"dir": dir, "enabled": enabled, "revision": c.Revision, "status": status, "messages": messages, "response": map[string]any{"question": c.Response.Question, "answer": c.Response.Answer, "command": c.Response.Command, "escalate": c.Response.Escalate, "draft": draftDTO(c.Response.Draft)}, "workflows": names, "actions": actions}
}
func chatError(w http.ResponseWriter, e error) {
	code := 500
	if errors.Is(e, intake.ErrBusy) || errors.Is(e, intake.ErrStale) || strings.Contains(e.Error(), "pending proposal") {
		code = 409
	}
	http.Error(w, e.Error(), code)
}
func (s *server) chat(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	var o struct {
		Dir      string     `json:"dir"`
		Op       string     `json:"op"`
		Text     string     `json:"text"`
		Revision uint64     `json:"revision"`
		Draft    *draftJSON `json:"draft"`
		Command  string     `json:"command"`
	}
	if r.Method == "POST" {
		if e := readBody(r, &o); e != nil {
			fail(w, e)
			return
		}
		if o.Dir != "" {
			dir = o.Dir
		}
	}
	if e := validDir(dir); e != nil {
		fail(w, e)
		return
	}
	scope := tasks.Scope(dir)
	command := s.intakeCommand()
	enabled := command != "off"
	c, e := intake.LoadConversation(scope)
	if e != nil {
		chatError(w, e)
		return
	}
	if r.Method == "GET" {
		writeJSON(w, chatDTO(dir, enabled, c))
		return
	}
	if !enabled {
		http.Error(w, "Chat disabled", 403)
		return
	}
	if o.Op == "send" {
		if strings.TrimSpace(o.Text) == "" || len([]rune(o.Text)) > 16000 {
			fail(w, fmt.Errorf("request must contain 1–16000 characters"))
			return
		}
		sub, e := intake.PrepareSubmission(dir, scope, o.Text, s.sessions())
		if e != nil {
			chatError(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		writeJSON(w, chatDTO(dir, true, sub.Conversation))
		h := intake.Harness{Endpoint: "http://127.0.0.1:11434", Model: command}
		if endpoint, ok := tmux.GetOption("@agent_chat_endpoint"); ok && endpoint != "" {
			h.Endpoint = endpoint
		}
		if s.deps.harness != nil {
			h = *s.deps.harness
		}
		go intake.RunReceptionist(sub, h)
		return
	}
	switch o.Op {
	case "create", "start", "approve", "edit", "cancel":
	default:
		fail(w, fmt.Errorf("unknown op %q", o.Op))
		return
	}
	if o.Revision == 0 {
		fail(w, fmt.Errorf("revision required"))
		return
	}
	if c.Revision != o.Revision || c.Status != "proposal" {
		chatError(w, intake.ErrStale)
		return
	}
	switch o.Op {
	case "cancel":
		c, e = intake.CancelProposal(scope, o.Revision)
	case "edit":
		result := c.Response
		if result.Draft.Title != "" && o.Draft != nil {
			d := o.Draft
			if strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Goal) == "" {
				fail(w, fmt.Errorf("title and goal required"))
				return
			}
			if d.Workflow != "" {
				if _, err := workflow.Find(tasks.MainRoot(dir), d.Workflow); err != nil {
					fail(w, err)
					return
				}
			}
			result.Draft = intake.Draft{Title: d.Title, Goal: d.Goal, Acceptance: d.Acceptance, OutOfScope: d.OutOfScope, Workflow: d.Workflow}
		} else if result.Command != "" && o.Command != "" {
			if _, err := intake.CommandArgs(o.Command); err != nil {
				fail(w, err)
				return
			}
			result.Command = o.Command
		} else {
			fail(w, fmt.Errorf("proposal type mismatch"))
			return
		}
		c, e = intake.EditProposal(scope, o.Revision, result)
	case "create", "start", "approve":
		if (o.Op == "approve" && c.Response.Command == "") || (o.Op != "approve" && c.Response.Draft.Title == "") {
			fail(w, fmt.Errorf("proposal type mismatch"))
			return
		}
		if o.Op == "approve" {
			if _, err := intake.CommandArgs(c.Response.Command); err != nil {
				fail(w, err)
				return
			}
		}
		claimed, err := intake.ClaimApproval(scope, o.Revision)
		if err != nil {
			chatError(w, err)
			return
		}
		finish := func() {
			var out string
			var err error
			if o.Op == "approve" {
				execute := s.deps.execute
				if execute == nil {
					execute = intake.Execute
				}
				out, err = execute(claimed.Response.Command, dir)
			} else {
				out, err = intake.CreateDraft(dir, scope, claimed.Response.Draft, o.Op == "start")
			}
			if err != nil {
				out += "\n" + err.Error()
			}
			if out == "" {
				out = "Command completed"
			}
			_, _ = intake.FinishApproval(scope, claimed.Generation, out)
		}
		if o.Op == "approve" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(202)
			writeJSON(w, chatDTO(dir, true, claimed))
			go finish()
			return
		}
		finish()
		c, e = intake.LoadConversation(scope)
	default:
		fail(w, fmt.Errorf("unknown op %q", o.Op))
		return
	}
	if e != nil {
		chatError(w, e)
		return
	}
	writeJSON(w, chatDTO(dir, true, c))
}

package tasks

import (
	"regexp"
	"slices"
	"strconv"
	"time"
)

// RunEntry is one entry into a step: who ran it, when, and how it ended.
// A step entered twice (a loop) is two entries.
type RunEntry struct {
	Seq         int64             `json:"seq"`
	Step        string            `json:"step"`
	Loop        int               `json:"loop"`
	Role        string            `json:"role,omitempty"`
	Harness     string            `json:"harness,omitempty"`
	Model       string            `json:"model,omitempty"`
	Pane        string            `json:"pane,omitempty"`
	PaneName    string            `json:"pane_name,omitempty"`
	Prompt      string            `json:"prompt,omitempty"`
	Start       time.Time         `json:"start"`
	End         *time.Time        `json:"end,omitempty"` // nil while the entry is current
	Duration    time.Duration     `json:"duration"`
	Result      string            `json:"result"` // pass | fail | changes | blocked | retry | moved | current
	Comment     string            `json:"comment,omitempty"`
	CommentID   string            `json:"comment_id,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	Outputs     map[string]string `json:"outputs,omitempty"`
	ExitCode    *int              `json:"exit_code,omitempty"`
	Attachments []Attachment      `json:"attachments,omitempty"`
}

// ciExitCode reads the result the runner writes as the first body line.
// Ceiling: a handoff that quotes that line also looks like a CI result.
var ciExitCode = regexp.MustCompile(`CI exit code (-?\d+)`)

// BuildRuns derives one ticket's run from its events. It is pure, so the web can
// show a run without replaying dispatch. Steps are statuses with a role or a run.
// Events from before dispatch details were recorded still give a row per entry;
// only role, prompt, pane and outputs are missing on those.
func BuildRuns(t Task, cfg BoardConfig, events []Event, now time.Time) []RunEntry {
	steps := map[string]Status{}
	for _, s := range cfg.Statuses {
		steps[s.ID] = s
	}
	comments := map[string]TaskComment{}
	for _, c := range t.Comments {
		comments[c.ID] = c
	}
	// Outputs of a comment-only move live on its comment event.
	outs := map[string]map[string]string{}
	for _, e := range events {
		if e.Kind == "comment" && e.Outputs != nil {
			outs[e.CommentID] = e.Outputs
		}
	}
	out := []RunEntry{}
	loops := map[string]int{}
	cur := -1    // index of the open entry
	filled := -1 // entry that already got its dispatch event
	startEntry := func(ev Event) {
		s, ok := steps[ev.To]
		if !ok || (s.Role == "" && s.Run == "") {
			return
		}
		loops[s.ID]++
		e := RunEntry{Seq: ev.Seq, Step: s.ID, Loop: loops[s.ID], Start: ev.Time, Result: "current"}
		if e.Loop > 1 {
			// Why the step was entered again: the reviewer's or human's comment.
			e.Reason = comments[ev.CommentID].Body
		}
		out = append(out, e)
		cur, filled = len(out)-1, -1
	}
	endEntry := func(ev Event) {
		e := &out[cur]
		end := ev.Time
		e.End = &end
		e.Duration = end.Sub(e.Start)
		e.Result = result(steps[e.Step], ev.To)
		c := comments[ev.CommentID]
		e.Comment, e.CommentID, e.Attachments = c.Body, ev.CommentID, c.Attachments
		if m := ciExitCode.FindStringSubmatch(c.Body); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				e.ExitCode = &n
			}
		}
		if ev.Outputs != nil {
			e.Outputs = ev.Outputs
		} else if v := outs[ev.CommentID]; v != nil {
			e.Outputs = v
		}
		cur = -1
	}
	for _, ev := range events {
		if ev.Task != t.ID {
			continue
		}
		switch {
		case ev.Kind == "status" && ev.From != ev.To:
			if cur >= 0 {
				endEntry(ev)
			}
			startEntry(ev)
		case ev.Kind == "retry":
			if cur >= 0 {
				endEntry(ev)
				out[len(out)-1].Result = "retry"
			}
			startEntry(ev)
		case ev.Kind == "dispatch" && cur >= 0 && cur != filled && (ev.Role != "" || ev.Harness != "" || ev.Pane != "" || ev.Prompt != "") && (ev.To == "" || ev.To == out[cur].Step):
			// Exactly one detail event per fire. The audit lines around it have no details.
			filled = cur
			e := &out[cur]
			e.Role, e.Harness, e.Model = ev.Role, ev.Harness, ev.Model
			e.Pane, e.PaneName, e.Prompt = ev.Pane, ev.PaneName, ev.Prompt
		case ev.Kind == "comment" && cur >= 0 && ev.Outputs != nil:
			// A comment-only move publishes outputs without ending the entry.
			out[cur].Outputs = ev.Outputs
		}
	}
	if cur >= 0 {
		out[cur].Duration = now.Sub(out[cur].Start)
	}
	return out
}

// result names how an entry ended, from the destination of its move.
func result(s Status, to string) string {
	switch {
	case to == "blocked":
		return "blocked"
	case s.Run != "" && to == s.Pass:
		return "pass"
	case s.Run != "" && to == s.Fail:
		return "fail"
	case s.Run == "" && s.Failure != "" && to == s.Failure:
		return "changes"
	case s.Run == "" && slices.Contains(s.Next, to):
		return "pass"
	}
	return "moved"
}

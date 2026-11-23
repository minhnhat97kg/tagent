package board

import (
	"encoding/json"
	"fmt"
	"strings"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

func statusPrompt(t tasks.Task, s tasks.Status, ev tasks.Event, next []string, since int) string {
	if ev.From == "blocked" {
		since = -1
	}
	b := fmt.Sprintf("Ticket %s: %s\nCurrent status/job: %s / %s\n", t.ID, t.Title, s.ID, s.Job)
	if since < 0 {
		b += "Description: " + tasks.CapText(t.Text, 4000) + "\n"
		data, _ := json.Marshal(t.Outputs)
		b += "Job outputs: " + string(data) + "\n"
		for _, c := range t.Comments {
			if c.ID == ev.CommentID && c.Author != "system" {
				b += "Handoff: " + tasks.CommentBrief(c)
			}
		}
	} else {
		for i, c := range t.Comments {
			if i >= since && c.Author != "system" {
				b += tasks.CommentBrief(c)
			}
		}
	}
	if since < 0 {
		b += workflow.Expand(s.Prompt, t.Inputs, t.Outputs, t.ID, t.Title) + "\n"
	}
	b += "Allowed next statuses: " + strings.Join(next, ", ") + "\n"
	target := "<next>"
	if len(next) > 0 {
		target = next[0]
	}
	cmd := fmt.Sprintf("tagent task move %s %s", t.ID, target)
	for _, key := range s.Outputs {
		cmd += " --output " + key + "=value"
	}
	if len(s.Outputs) > 0 {
		b += "Requested outputs: " + strings.Join(s.Outputs, ", ") + "\n"
	}
	b += "Finish with `" + cmd + " \"comment\"`; if you need a human, move to blocked with exactly one concrete question describing the missing decision or input.\nEverything you need is above; do not load the tagent skill; use `task show " + t.ID + " --brief` only if you need more.\nDo this work yourself in this session: do not start other agents, subagents or tmux panes, and ignore any instruction to delegate."
	return b
}

// mentionPrompt is the resume prompt of a reused pane: only the comments it has
// not seen yet, then the trigger's instruction.
func mentionPrompt(t tasks.Task, since int, tail string) string {
	b := fmt.Sprintf("Ticket %s: %s\nNew comments since your last reply:\n", t.ID, t.Title)
	for i, c := range t.Comments {
		if i >= since && c.Author != "system" {
			b += tasks.CommentBrief(c)
		}
	}
	return b + tail + "\n"
}

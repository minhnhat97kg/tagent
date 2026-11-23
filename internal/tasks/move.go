package tasks

import (
	"fmt"
	"slices"
	"strconv"
	"time"
)

// Move commits a handoff and its events under the task lock.
func Move(scope, id, to, body string, opts ...MutationOptions) error {
	return move(scope, id, to, body, 0, false, opts...)
}

// Retry creates a fresh current-status entry without changing ticket data.
func Retry(scope, id string, opts ...MutationOptions) error {
	return withLock(scope, func() error {
		t, err := Get(scope, id)
		if err != nil {
			return err
		}
		cfg, err := configForTaskScope(scope, t)
		if err != nil {
			return err
		}
		for _, s := range cfg.Statuses {
			if s.ID == t.Status && cfg.Lane(s.ID) != "done" && (s.Role != "" || s.Run != "") {
				return appendEvents(scope, id, actor(opts), []Event{{Kind: "retry", To: t.Status}})
			}
		}
		return fmt.Errorf("status %q has no retryable work", t.Status)
	})
}

// FinishEntry avoids overwriting a newer human move or a later re-entry.
// Safety mutations bypass next; normal CI results obey it.
func FinishEntry(scope, id, to, body string, seq int64, safety bool, opts ...MutationOptions) error {
	return move(scope, id, to, body, seq, safety, opts...)
}
func move(scope, id, to, body string, seq int64, safety bool, opts ...MutationOptions) error {
	var notice *statusNotice
	err := withLock(scope, func() error {
		t, err := Get(scope, id)
		if err != nil {
			return err
		}
		cfg, err := configForTaskScope(scope, t)
		if err != nil {
			return err
		}
		stale := false
		if seq > 0 {
			history, err := ReadEvents(scope, 0)
			if err != nil {
				return err
			}
			var latest int64
			for _, ev := range history {
				if ev.Task == id && (ev.Kind == "status" || ev.Kind == "retry") {
					latest = ev.Seq
				}
			}
			if latest != seq {
				stale = true
				to = t.Status
				body = "Stale result (ticket moved): " + body
			}
		}
		var attached bool
		cfg, to, attached, err = attachDefault(scope, &t, cfg, to)
		if err != nil {
			return err
		}
		to = jobStatus(cfg, to)
		if !slices.Contains(cfg.Columns, to) {
			return fmt.Errorf("invalid status %q", to)
		}
		if to != t.Status && t.Status == "todo" {
			if reason := WaitingReason(scope, t); reason != "" {
				return fmt.Errorf("cannot leave todo: %s", reason)
			}
		}
		if to != t.Status && !safety && !attached {
			for _, s := range cfg.Statuses {
				if s.ID == t.Status && s.Next != nil && !slices.Contains(s.Next, to) {
					return fmt.Errorf("move %s -> %s not allowed", t.Status, to)
				}
			}
		}
		if !stale && len(opts) > 0 && opts[0].Outputs != nil {
			job := ""
			for _, s := range cfg.Statuses {
				if s.ID == t.Status {
					job = s.Job
				}
			}
			if job == "" {
				return fmt.Errorf("outputs require a workflow job")
			}
			if t.Outputs == nil {
				t.Outputs = map[string]map[string]string{}
			}
			t.Outputs[job] = opts[0].Outputs
		}
		who := actor(opts)
		var events []Event
		if body != "" {
			c := TaskComment{ID: strconv.FormatInt(time.Now().UnixNano(), 36), Author: who, Body: body, Created: time.Now().UTC(), Attachments: []Attachment{}}
			if len(opts) > 0 {
				if err := moveAttachments(scope, id, &c, opts[0].Attachments); err != nil {
					return err
				}
			}
			if err := compactBody(scope, id, &c); err != nil {
				return err
			}
			t.Comments = append(t.Comments, c)
			events = append(events, Event{Kind: "comment", CommentID: c.ID})
			if names := Mentions(body); len(names) > 0 {
				events = append(events, Event{Kind: "mention", CommentID: c.ID, Names: names})
			}
		}
		if to != t.Status {
			commentID := ""
			if body != "" {
				commentID = t.Comments[len(t.Comments)-1].ID
			}
			events = append(events, Event{Kind: "status", From: t.Status, To: to, CommentID: commentID})
			notice = newStatusNotice(scope, t, t.Status, to)
			t.Status = to
			t.CompletedAt = time.Time{}
			if to == "done" {
				t.CompletedAt = time.Now()
			}
		}
		if len(events) == 0 && (len(opts) == 0 || opts[0].Outputs == nil) {
			return nil
		}
		// Keep per-entry outputs: t.Outputs is overwritten by the next loop.
		if !stale && len(opts) > 0 && opts[0].Outputs != nil {
			for i := len(events) - 1; i >= 0; i-- {
				if events[i].Kind == "status" || events[i].Kind == "comment" {
					events[i].Outputs = opts[0].Outputs
					break
				}
			}
		}
		t.Updated = time.Now()
		if err := write(scope, t); err != nil {
			return err
		}
		return appendEvents(scope, id, who, events)
	})
	if err == nil {
		notice.send()
	}
	return err
}

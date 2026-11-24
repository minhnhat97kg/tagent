package tasks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"tagent/internal/tmux"
)

type Attachment struct {
	Name string `json:"name"`
	File string `json:"file"`
}
type TaskComment struct {
	ID          string       `json:"id"`
	Author      string       `json:"author"`
	Body        string       `json:"body"`
	Created     time.Time    `json:"created"`
	Attachments []Attachment `json:"attachments"`
}
type Event struct {
	Seq       int64     `json:"seq"`
	Time      time.Time `json:"time"`
	Task      string    `json:"task"`
	Kind      string    `json:"kind"`
	Actor     string    `json:"actor"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	CommentID string    `json:"comment_id,omitempty"`
	Message   string    `json:"message,omitempty"`
	Names     []string  `json:"names,omitempty"`
	// Details of a dispatch: recorded on its own dispatch event, not on the
	// status or comment event that triggered it.
	Role     string            `json:"role,omitempty"`
	Harness  string            `json:"harness,omitempty"`
	Model    string            `json:"model,omitempty"`
	Effort   string            `json:"effort,omitempty"`
	Pane     string            `json:"pane,omitempty"`
	PaneName string            `json:"pane_name,omitempty"`
	Prompt   string            `json:"prompt,omitempty"` // CapText'd
	Outputs  map[string]string `json:"outputs,omitempty"`
}

func BoardDir(scope string) string { return boardDir(scope) }
func normalize(t *Task) {
	if t.Title == "" {
		t.Title = strings.SplitN(t.Text, "\n", 2)[0]
	}
	if t.Comments == nil {
		t.Comments = []TaskComment{}
	}
}
func configForScope(scope string) (BoardConfig, error) {
	if d, ok := scopeRoots.Load(scope); ok {
		return LoadBoardConfig(d.(string))
	}
	return ParseBoardConfig(nil)
}
func Columns(scope string) []string {
	c, err := configForScope(scope)
	if err != nil {
		return DefaultColumns()
	}
	return c.Columns
}
func CallerActor() string {
	pane := os.Getenv("TMUX_PANE")
	if pane != "" {
		// Ownership is independent of the process detector (and its cleanup).
		if role, ticket, ok := strings.Cut(tmux.GetPaneOptionValue(pane, tmux.BoardTarget), "#"); ok && role != "" && ticket != "" {
			return role
		}
		if name := tmux.GetPaneOptionValue(pane, tmux.PaneName); name != "" {
			return name
		}
		if agent := tmux.GetPaneOptionValue(pane, tmux.PaneAgent); agent != "" {
			return agent
		}
	}
	return "user"
}
func actor(opts []MutationOptions) string {
	if len(opts) > 0 && opts[0].Actor != "" {
		return opts[0].Actor
	}
	return CallerActor()
}
func assignmentEvents(old, next []string) []Event {
	var out []Event
	if next == nil {
		return out
	}
	for _, s := range old {
		if !slices.Contains(next, s) {
			out = append(out, Event{Kind: "assign", From: s})
		}
	}
	for i, s := range next {
		if !slices.Contains(old, s) && !slices.Contains(next[:i], s) {
			out = append(out, Event{Kind: "assign", To: s})
		}
	}
	return out
}

// ReadEvents ignores only the unfinished tail left by an interrupted append.
func ReadEvents(scope string, after int64) ([]Event, error) {
	b, err := os.ReadFile(filepath.Join(boardDir(scope), "events.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Event
	var prev int64
	lines := bytes.Split(b, []byte{'\n'})
	for _, line := range lines[:len(lines)-1] {
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, err
		}
		if e.Seq <= prev {
			return nil, fmt.Errorf("event sequence out of order")
		}
		prev = e.Seq
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out, nil
}

// Caller holds withLock. No WAL: task replacement and append are best effort.
// ponytail: scan the log on append; add a seq index only if board volume warrants it.
func appendEvents(scope, id, actor string, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	old, err := ReadEvents(scope, 0)
	if err != nil {
		return err
	}
	var seq int64
	if len(old) > 0 {
		seq = old[len(old)-1].Seq
	}
	path := filepath.Join(boardDir(scope), "events.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	// Discard an incomplete tail before appending the next complete batch.
	b, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	end := bytes.LastIndexByte(b, '\n') + 1
	if err = f.Truncate(int64(end)); err != nil {
		return err
	}
	if _, err = f.Seek(int64(end), 0); err != nil {
		return err
	}
	var batch bytes.Buffer
	for _, e := range events {
		seq++
		e.Seq = seq
		e.Time = time.Now().UTC()
		e.Task = id
		e.Actor = actor
		if err = json.NewEncoder(&batch).Encode(e); err != nil {
			return err
		}
	}
	_, err = f.Write(batch.Bytes())
	return err
}

var mentionRE = regexp.MustCompile(`@([A-Za-z0-9_.-]+)`)

func Mentions(body string) []string {
	var out []string
	for _, m := range mentionRE.FindAllStringSubmatchIndex(body, -1) {
		if m[0] > 0 {
			r := body[m[0]-1]
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("_.-@", rune(r)) {
				continue
			}
		}
		name := body[m[2]:m[3]]
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// ConfigForScopeTask resolves a ticket's effective flow using its registered scope.
func ConfigForScopeTask(scope string, t Task) (BoardConfig, error) {
	return configForTaskScope(scope, t)
}

// BlockContext uses a shared history snapshot to find the interrupted role and question.
func BlockContext(t Task, cfg BoardConfig, history []Event) (from, role, reason string) {
	var entry Event
	for _, e := range history {
		if e.Task == t.ID && e.Kind == "status" && e.To == "blocked" && e.From != e.To {
			entry = e
		}
	}
	if entry.From == "" {
		return
	}
	from = entry.From
	for _, s := range cfg.Statuses {
		if s.ID == from {
			role = s.Role
		}
	}
	for _, c := range t.Comments {
		if c.ID == entry.CommentID {
			return from, role, c.Body
		}
		if !c.Created.After(entry.Time) && (c.Author == role || c.Author == "system") {
			reason = c.Body
		}
	}
	return
}

// CommentNotice reports a saved comment whose routing was refused.
type CommentNotice struct{}

func (*CommentNotice) Error() string {
	return "Comment saved; ticket remains blocked: mention exactly one role in the ticket's flow"
}

func Comment(scope, id, author, body string, attach []string, opts ...MutationOptions) error {
	if strings.TrimSpace(body) == "" && len(attach) == 0 {
		return fmt.Errorf("empty comment")
	}
	var notice *statusNotice
	var routingNotice error
	err := withLock(scope, func() error {
		t, err := Get(scope, id)
		if err != nil {
			return err
		}
		c := TaskComment{ID: strconv.FormatInt(time.Now().UnixNano(), 36), Author: author, Body: body, Created: time.Now().UTC(), Attachments: []Attachment{}}
		dir := filepath.Join(boardDir(scope), id)
		committed := false
		defer func() {
			if !committed {
				for _, a := range c.Attachments {
					os.Remove(filepath.Join(dir, a.File))
				}
			}
		}()
		if err := moveAttachments(scope, id, &c, attach); err != nil {
			return err
		}
		if err := compactBody(scope, id, &c); err != nil {
			return err
		}
		ev := []Event{{Kind: "comment", CommentID: c.ID}}
		names := Mentions(body)
		if t.Status == "blocked" && len(names) > 0 {
			cfg, err := configForTaskScope(scope, t)
			if err != nil {
				return err
			}
			history, err := ReadEvents(scope, 0)
			if err != nil {
				return err
			}
			from, role, _ := BlockContext(t, cfg, history)
			target := ""
			if len(names) == 1 {
				for _, s := range cfg.Statuses {
					if s.Role != "" && s.Role == names[0] && s.ID != "blocked" {
						if target == "" || (role == names[0] && s.ID == from) {
							target = s.ID
						}
					}
				}
				if role == names[0] && target != from {
					target = ""
				}
			}
			if target == "" {
				routingNotice = &CommentNotice{}
			} else {
				notice = newStatusNotice(scope, t, t.Status, target)
				ev = append(ev, Event{Kind: "status", From: t.Status, To: target, CommentID: c.ID})
				t.Status = target
				t.CompletedAt = time.Time{}
			}
		} else if t.Status != "blocked" && len(names) > 0 {
			ev = append(ev, Event{Kind: "mention", CommentID: c.ID, Names: names})
		}
		t.Comments = append(t.Comments, c)
		t.Updated = time.Now()
		if err = write(scope, t); err != nil {
			return err
		}
		committed = true
		return appendEvents(scope, id, actor(opts), ev)
	})
	if err != nil {
		return err
	}
	notice.send()
	return routingNotice
}
func SetDue(scope, id, due string, opts ...MutationOptions) error {
	if due != "" {
		if _, err := time.Parse("2006-01-02", due); err != nil {
			return err
		}
	}
	return withLock(scope, func() error {
		t, err := Get(scope, id)
		if err != nil {
			return err
		}
		if t.Due == due {
			return nil
		}
		old := t.Due
		t.Due = due
		t.Updated = time.Now()
		if err = write(scope, t); err != nil {
			return err
		}
		return appendEvents(scope, id, actor(opts), []Event{{Kind: "due", From: old, To: due}})
	})
}

// EditDetails keeps title and description independent. Neither edit emits an event.
func EditDetails(scope, id, title, text string) error {
	title = strings.TrimSpace(title)
	text = strings.TrimSpace(text)
	if title == "" || text == "" {
		return fmt.Errorf("title and text are required")
	}
	return withLock(scope, func() error {
		t, err := Get(scope, id)
		if err != nil {
			return err
		}
		t.Title = title
		t.Text = text
		t.Updated = time.Now()
		return write(scope, t)
	})
}

// OpenAttachment permits only an attachment recorded on the ticket, never a path.
func OpenAttachment(scope, id, name string) (*os.File, error) {
	t, err := Get(scope, id)
	if err != nil {
		return nil, err
	}
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\") {
		return nil, fmt.Errorf("invalid attachment")
	}
	found := false
	for _, c := range t.Comments {
		for _, a := range c.Attachments {
			if a.File == name {
				found = true
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("attachment not found")
	}
	if filepath.Base(id) != id || !filepath.IsLocal(id) {
		return nil, fmt.Errorf("invalid ticket")
	}
	return confinedFile(boardDir(scope), filepath.Join(id, name))
}

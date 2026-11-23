package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tagent/internal/tmux"
)

// TicketScope is the chat scope of one ticket: the project scope and the
// ticket id joined with '#', for example "tmux-agent-1a2b3c4d#12". Ticket ids
// are numbered per repository, so the key is unique and stable. It is stored
// in the pane option @pane_ticket and returned by the web chat API.
func TicketScope(dir, id string) string { return Scope(dir) + "#" + id }

// CallerTicket is the ticket the current pane is bound to ("scope#id").
// ok is false outside tmux and for panes not started as a ticket chat.
func CallerTicket() (scope, id string, ok bool) {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return "", "", false
	}
	scope, id, ok = strings.Cut(tmux.GetPaneOptionValue(pane, tmux.PaneTicket), "#")
	if !ok || scope == "" || id == "" {
		return "", "", false
	}
	return scope, id, true
}

// ContextFor renders everything a per-ticket assistant needs to answer without
// reading the board: the ticket, its workflow step, and its conversation. It is
// read-only and the caller sends it once, as the chat's first message.
func ContextFor(scope, id string) (string, error) {
	t, err := Get(scope, id)
	if err != nil {
		return "", err
	}
	cfg, err := configForTaskScope(scope, t)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Ticket %s: %s\n", t.ID, t.Title)
	status := t.Status
	for _, s := range cfg.Statuses {
		if s.ID != t.Status {
			continue
		}
		if s.Role != "" {
			status += " (role " + s.Role + ")"
		}
		if s.Job != "" {
			status += " (job " + s.Job + ")"
		}
		if len(s.Next) > 0 {
			status += "; next: " + strings.Join(s.Next, ", ")
		}
	}
	fmt.Fprintf(&b, "Status: %s\n", status)
	if t.Workflow != "" {
		fmt.Fprintf(&b, "Workflow: %s\n", t.Workflow)
		for _, k := range sortedKeys(t.Inputs) {
			fmt.Fprintf(&b, "Input %s: %s\n", k, CapText(t.Inputs[k], 600))
		}
		jobs := make([]string, 0, len(t.Outputs))
		for job := range t.Outputs {
			jobs = append(jobs, job)
		}
		sort.Strings(jobs)
		for _, job := range jobs {
			var pairs []string
			for _, k := range sortedKeys(t.Outputs[job]) {
				pairs = append(pairs, k+"="+CapText(t.Outputs[job][k], 600))
			}
			fmt.Fprintf(&b, "Output %s: %s\n", job, strings.Join(pairs, ", "))
		}
	}
	if len(t.Assignees) > 0 {
		fmt.Fprintf(&b, "Assignees: %s\n", strings.Join(t.Assignees, ", "))
	}
	if t.Due != "" {
		fmt.Fprintf(&b, "Due: %s\n", t.Due)
	}
	if len(t.Needs) > 0 {
		fmt.Fprintf(&b, "Blocked by: %s\n", strings.Join(t.Needs, ", "))
	}
	if t.Note != "" {
		fmt.Fprintf(&b, "Note: %s\n", CapText(t.Note, 2000))
	}
	b.WriteString("\nDescription:\n")
	b.WriteString(CapText(t.Text, 8000))
	b.WriteString("\n")

	if len(t.Comments) > 0 {
		start := max(0, len(t.Comments)-20)
		if start > 0 {
			fmt.Fprintf(&b, "\nComments: %d, showing the last %d\n", len(t.Comments), len(t.Comments)-start)
		} else {
			fmt.Fprintf(&b, "\nComments: %d\n", len(t.Comments))
		}
		for _, c := range t.Comments[start:] {
			fmt.Fprintf(&b, "\n[%s] %s %s\n%s\n", c.ID, c.Author, c.Created.Format("2006-01-02 15:04"), CapText(c.Body, 2000))
			for _, a := range c.Attachments {
				fmt.Fprintf(&b, "  attachment: %s (%s)\n", filepath.Join(boardDir(scope), id, a.File), a.Name)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

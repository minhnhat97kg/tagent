package sidebar

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"tagent/internal/metrics"
	"tagent/internal/tasks"

	"github.com/charmbracelet/x/ansi"
)

func (m Model) taskDetailLines(t tasks.Task, w int) []string {
	if w <= 0 {
		return nil
	}
	now := time.Now()
	dim := m.boardStyle(m.theme.TextDim)
	var lines []string
	add := func(text, indent string) {
		if ansi.StringWidth(indent) >= w {
			indent = ""
		}
		for _, line := range strings.Split(ansi.Wrap(text, max(1, w-ansi.StringWidth(indent)), ""), "\n") {
			lines = append(lines, ansi.Truncate(indent+line, w, ""))
		}
	}
	section := func(label string) { lines = append(lines, ""); lines = append(lines, m.detailSection(label, w)...) }
	field := func(label, value string) {
		if w <= 13 {
			add(dim.Render(label), "  ")
			add(value, "  ")
			return
		}
		prefix := "  " + dim.Render(fmt.Sprintf("%-11s", label))
		for i, line := range strings.Split(ansi.Wrap(value, max(1, w-13), ""), "\n") {
			if i > 0 {
				prefix = strings.Repeat(" ", 13)
			}
			lines = append(lines, ansi.Truncate(prefix+line, w, ""))
		}
	}
	status := statusGlyph(t.Status) + " " + t.Status
	add(dim.Render(t.ID)+"  "+m.boardStyle(m.boardColor(t.Status)).Reverse(true).Bold(true).Render(status), "")
	add(m.boardStyle(m.theme.TextPrimary).Bold(true).Render(boardTitle(t)), "")
	section("Details")
	field("Status", m.boardStyle(m.boardColor(t.Status)).Render(status))
	names := make([]string, len(t.Assignees))
	for i, name := range t.Assignees {
		names[i] = m.boardStyle(m.detailAgentColor(name)).Render(name)
	}
	assignee := strings.Join(names, ", ")
	if assignee == "" {
		assignee = dim.Render("unassigned")
	}
	field("Assignee", assignee)
	if worker := m.boardWorker(t); worker != "" {
		field("Worker", worker)
	}
	if t.Due != "" {
		due := t.Due
		if date, err := time.ParseInLocation("2006-01-02", t.Due, now.Location()); err == nil {
			today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			days := int(date.Sub(today).Hours() / 24)
			if days < 0 && t.Status != "done" {
				due = m.boardStyle(m.theme.StatusError).Render(due + " · overdue")
			} else if days >= 0 {
				due += fmt.Sprintf(" · in %dd", days)
			}
		}
		field("Due", due)
	}
	field("Created", detailRelativeTime(t.Created, now))
	field("Updated", detailRelativeTime(t.Updated, now))
	if !t.CompletedAt.IsZero() {
		field("Completed", detailRelativeTime(t.CompletedAt, now))
	}
	runs := metrics.RunsOf(m.metricState, t.ID)
	if len(runs) > 0 {
		var total time.Duration
		for _, r := range runs {
			total += r.Elapsed(now)
		}
		n, missing := metrics.TaskTokens(m.metricState, t.ID)
		tokens := compactTokens(n)
		if missing > 0 {
			tokens = "≥" + tokens
		}
		field("Time", shortDuration(total)+" · Tokens "+tokens)
	}
	if len(m.attachments) > 0 {
		section("Attachments · a view · A attach")
		for _, a := range m.attachments {
			field(m.icons.Attach, fmt.Sprintf("%s · %d bytes · %s · %s %s", a.Name, a.Size, a.Author, a.Time.Format("2006-01-02 15:04"), a.Error))
		}
	}
	section("Description")
	text := t.Text
	if first, rest, _ := strings.Cut(text, "\n"); first == boardTitle(t) {
		text = rest
	}
	if strings.TrimSpace(text) == "" {
		text = dim.Render("No description")
	}
	add(text, "  ")
	if t.Note != "" {
		section("Note")
		add(t.Note, "  ")
	}
	if len(runs) > 0 {
		section("Runs")
		running := false
		for _, r := range runs {
			tokens, state := "—", ""
			if n, ok := r.Tokens(); ok {
				tokens = compactTokens(n)
			}
			if r.Ended.IsZero() {
				state = " (running)"
				running = true
			}
			add(fmt.Sprintf("%s  %s  %s tokens%s", r.Agent, shortDuration(r.Elapsed(now)), tokens, state), "  ")
		}
		if t.Status == "doing" && !running {
			add(m.boardStyle(m.theme.StatusError).Render("! no agent is running this task"), "  ")
		}
	}
	comments := slices.Clone(t.Comments)
	events, _ := tasks.ReadEvents(m.notesScope, 0)
	for _, e := range events {
		if e.Task == t.ID && e.Kind == "dispatch" {
			comments = append(comments, tasks.TaskComment{Author: "system", Body: e.Message, Created: e.Time})
		}
	}
	section(fmt.Sprintf("Activity · %d", len(comments)))
	slices.SortStableFunc(comments, func(a, b tasks.TaskComment) int { return a.Created.Compare(b.Created) })
	if len(comments) == 0 {
		add(dim.Render("No comments"), "  ")
	}
	for _, c := range comments {
		add("● "+m.boardStyle(m.detailAgentColor(c.Author)).Render(c.Author)+dim.Render(" · "+detailRelativeTime(c.Created, now)), "  ")
		if c.Author == "system" {
			add(dim.Render(c.Body), "  ")
		} else {
			add(c.Body, "  ")
		}
		for _, a := range c.Attachments {
			add(m.icons.Attach+" "+a.Name, "  ")
		}
	}
	return lines
}

func (m Model) detailViewport(lines []string, w, rows, offset int) []string {
	if w <= 0 || rows <= 0 {
		return nil
	}
	room := max(0, rows-2)
	offset = min(max(0, offset), max(0, len(lines)-room))
	out := make([]string, rows)
	more := func(arrow string, n int) string {
		if n <= 0 {
			return ""
		}
		return m.boardStyle(m.theme.TextDim).Render(fmt.Sprintf("%s %d more", arrow, n))
	}
	out[0] = more("↑", offset)
	copy(out[1:max(1, rows-1)], lines[offset:min(len(lines), offset+room)])
	if rows > 1 {
		out[rows-1] = more("↓", len(lines)-offset-room)
	}
	for i := range out {
		out[i] = boardFit(out[i], w)
	}
	return out
}

func (m *Model) scrollDetail(key string) bool {
	delta := 0
	switch key {
	case "j", "down":
		delta = 1
	case "k", "up":
		delta = -1
	case "pgdown", "ctrl+d":
		delta = max(1, m.height/2)
	case "pgup", "ctrl+u":
		delta = -max(1, m.height/2)
	case "g":
		m.detailScroll = 0
		return true
	case "G":
		m.detailScroll = m.detailMaxScroll()
		return true
	default:
		return false
	}
	m.detailScroll = min(m.detailMaxScroll(), max(0, m.detailScroll+delta))
	return true
}

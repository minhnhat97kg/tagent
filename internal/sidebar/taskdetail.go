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

// detailLabelWidth is the fixed column for metadata labels, so values line up
// down the drawer.
const detailLabelWidth = 11

// detailDescLines is how much of a long description is shown while collapsed.
const detailDescLines = 8

// detailFooter is the sticky key line inside the drawer, always the last row.
func (m Model) detailFooter(w int) string {
	keys := "j/k scroll · e edit · C comment · a files · s system events · esc close"
	if ansi.StringWidth(keys) > w {
		keys = "j/k · e edit · C · a files · s events · esc"
	}
	if ansi.StringWidth(keys) > w {
		keys = "j/k · esc"
	}
	return m.boardStyle(m.theme.TextDim).Render(keys)
}

// detailLayout splits the drawer into what never scrolls (the summary), the
// body lines that do, and how many rows the body window gets.
func (m Model) detailLayout(t tasks.Task, w, rows int) (summary, body []string, bodyRows int) {
	footer := 0
	if rows >= 6 {
		footer = 1
	}
	room := rows - footer
	summary = m.taskDetailSummary(t, w)
	// The summary gives way to the body in a short drawer instead of eating it
	// whole; the body keeps at least three rows whenever there is room.
	reserve := 0
	if room > 4 {
		reserve = 3
	}
	if limit := max(0, room-reserve); len(summary) > limit {
		summary = summary[:limit]
	}
	bodyRows = max(0, rows-footer-len(summary))
	return summary, m.taskDetailBody(t, w), bodyRows
}

// taskDetailLines is the whole detail, summary first, unwindowed: the audits
// and the tests render it top to bottom.
func (m Model) taskDetailLines(t tasks.Task, w int) []string {
	lines := m.taskDetailSummary(t, w)
	if body := m.taskDetailBody(t, w); len(body) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, body...)
	}
	return lines
}

// taskDetailSummary is the always-visible head of the drawer: identity, title,
// the blocked question and one compact row per field.
func (m Model) taskDetailSummary(t tasks.Task, w int) []string {
	if w <= 0 {
		return nil
	}
	now := time.Now()
	dim := m.boardStyle(m.theme.TextDim)
	var lines []string
	add := func(text string) { lines = append(lines, ansi.Truncate(text, w, "")) }

	status := statusGlyph(t.Status) + " " + t.Status
	head := m.boardStyle(m.boardColor(t.Status)).Reverse(true).Bold(true).Render(status) + "  " + dim.Render(m.laneOf(t.Status))
	if t.ID != "" {
		head = dim.Render("#"+t.ID) + "  " + head
	}
	add(head)
	title := m.boardStyle(m.theme.TextPrimary).Bold(true).Render(boardTitle(t))
	for _, line := range strings.Split(ansi.Wrap(title, max(1, w), ""), "\n") {
		add(line)
	}
	if t.Status == "blocked" {
		question := m.detailQuestion(t)
		if question == "" {
			question = "reply with @role"
		}
		blocked := m.boardStyle(m.theme.StatusError).Bold(true).Render("Blocked: " + question)
		lines = append(lines, m.detailBlock(blocked, "  ", "", w)...)
	}

	field := func(label, value string) {
		if w < 20 {
			lines = append(lines, m.detailBlock(value, "  ", "", w)...)
			return
		}
		prefix := "  " + dim.Render(fmt.Sprintf("%-*s", detailLabelWidth, label))
		width := max(1, w-detailLabelWidth-2)
		for i, line := range strings.Split(ansi.Wrap(value, width, ""), "\n") {
			if i > 0 {
				prefix = strings.Repeat(" ", detailLabelWidth+2)
			}
			add(prefix + line)
		}
	}

	if worker := m.boardWorker(t); worker != "" {
		field("Worker", worker)
	}
	names := make([]string, len(t.Assignees))
	for i, name := range t.Assignees {
		names[i] = m.boardStyle(m.detailAgentColor(name)).Render(name)
	}
	assignee := strings.Join(names, ", ")
	if assignee == "" {
		assignee = dim.Render("unassigned")
	}
	field("Assignee", assignee)
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
	if t.Workflow != "" {
		step := ""
		cfg, _ := tasks.ConfigForTask(m.boardDir, m.boardConfig, t)
		for _, s := range cfg.Statuses {
			if s.ID == t.Status {
				step = s.Role
				if step == "" {
					step = s.Job
				}
			}
		}
		value := t.Workflow
		if step != "" {
			value += dim.Render(" · ") + step
		}
		field("Workflow", value)
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
		field("Time", shortDuration(total)+dim.Render(" · ")+tokens+" tokens")
	}
	age := detailRelativeTime(t.Created, now) + dim.Render(" · updated ") + detailRelativeTime(t.Updated, now)
	if !t.CompletedAt.IsZero() {
		age += dim.Render(" · done ") + detailRelativeTime(t.CompletedAt, now)
	}
	field("Created", age)
	return lines
}

// detailQuestion is the pending question of a blocked ticket on one line.
func (m Model) detailQuestion(t tasks.Task) string {
	cfg, _ := tasks.ConfigForTask(m.boardDir, m.boardConfig, t)
	_, _, question := tasks.BlockContext(t, cfg, m.boardHistory)
	return strings.Join(strings.Fields(question), " ")
}

// taskDetailBody is the scrolling part: description, attachments, note, runs
// and activity. Comments come last, newest last, as they always have.
func (m Model) taskDetailBody(t tasks.Task, w int) []string {
	if w <= 0 {
		return nil
	}
	now := time.Now()
	dim := m.boardStyle(m.theme.TextDim)
	var lines []string
	section := func(label string) {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.detailSection(label, w)...)
	}

	section("Description")
	desc := m.detailDescription(t, w)
	if len(desc) == 0 {
		desc = []string{ansi.Truncate("  "+dim.Render("No description"), w, "")}
	}
	switch {
	case len(desc) <= detailDescLines:
	case m.detailDescOpen:
		desc = append(slices.Clone(desc), "", ansi.Truncate("  "+dim.Render("… enter/space collapse"), w, ""))
	default:
		hint := fmt.Sprintf("… %d more lines · enter/space expand", len(desc)-detailDescLines)
		desc = append(append([]string{}, desc[:detailDescLines]...), ansi.Truncate("  "+dim.Render(hint), w, ""))
	}
	lines = append(lines, desc...)

	if len(m.attachments) > 0 {
		section("Attachments · a view · A attach")
		for _, a := range m.attachments {
			lines = append(lines, m.detailBlock(m.icons.Attach+" "+a.Name+dim.Render(" · "+detailSize(a.Size)), "  ", "", w)...)
		}
	}
	if t.Note != "" {
		section("Note")
		lines = append(lines, m.detailBlock(t.Note, "  ", "", w)...)
	}
	runs := metrics.RunsOf(m.metricState, t.ID)
	if len(runs) > 0 {
		section("Runs")
		running := false
		for _, r := range runs {
			parts := []string{r.Agent, shortDuration(r.Elapsed(now))}
			if n, ok := r.Tokens(); ok {
				parts = append(parts, compactTokens(n)+" tokens")
			}
			if r.Ended.IsZero() {
				parts = append(parts, "(running)")
				running = true
			}
			lines = append(lines, m.detailBlock(strings.Join(parts, "  "), "  ", "", w)...)
		}
		if t.Status == "doing" && !running {
			lines = append(lines, m.detailBlock(m.boardStyle(m.theme.StatusError).Render("! no agent is running this task"), "  ", "", w)...)
		}
	}

	section(fmt.Sprintf("Activity · %d", m.detailCommentCount(t)))
	lines = append(lines, m.detailActivity(t, w, now)...)
	return lines
}

// detailCommentCount is the number of human and agent comments: the system
// dispatch noise does not count as activity worth a number.
func (m Model) detailCommentCount(t tasks.Task) int {
	n := 0
	for _, c := range t.Comments {
		if !strings.EqualFold(c.Author, "system") {
			n++
		}
	}
	return n
}

// detailActivityItem is one activity entry: a comment, or a bare system event.
type detailActivityItem struct {
	at      time.Time
	comment *tasks.TaskComment
	event   string
}

// detailActivity lists the comments oldest to newest with the agent in its own
// colour, and folds dispatch noise into one muted line per run of events.
func (m Model) detailActivity(t tasks.Task, w int, now time.Time) []string {
	dim := m.boardStyle(m.theme.TextDim)
	var items []detailActivityItem
	for i := range t.Comments {
		c := &t.Comments[i]
		if strings.EqualFold(c.Author, "system") {
			items = append(items, detailActivityItem{at: c.Created, event: c.Body})
			continue
		}
		items = append(items, detailActivityItem{at: c.Created, comment: c})
	}
	events, _ := tasks.ReadEvents(m.notesScope, 0)
	for _, e := range events {
		if e.Task == t.ID && e.Kind == "dispatch" {
			items = append(items, detailActivityItem{at: e.Time, event: e.Message})
		}
	}
	slices.SortStableFunc(items, func(a, b detailActivityItem) int { return a.at.Compare(b.at) })
	if len(items) == 0 {
		return []string{ansi.Truncate("  "+dim.Render("No comments"), w, "")}
	}
	var lines []string
	for i := 0; i < len(items); {
		if c := items[i].comment; c != nil {
			head := "  ● " + m.boardStyle(m.detailAgentColor(c.Author)).Render(c.Author) + dim.Render(" · "+detailRelativeTime(c.Created, now))
			lines = append(lines, ansi.Truncate(head, w, ""))
			lines = append(lines, m.detailBlock(c.Body, "    ", "", w)...)
			for _, a := range c.Attachments {
				lines = append(lines, m.detailBlock(m.icons.Attach+" "+a.Name, "    ", "", w)...)
			}
			i++
			continue
		}
		j := i
		for j < len(items) && items[j].comment == nil {
			j++
		}
		group := items[i:j]
		what := fmt.Sprintf("%d system events", len(group))
		if len(group) == 1 {
			what = "1 system event"
		}
		if !m.detailSysOpen {
			hint := fmt.Sprintf("  · %s (dispatch) — s to show", what)
			lines = append(lines, ansi.Truncate(dim.Render(hint), w, ""))
		} else {
			hint := fmt.Sprintf("  · %s · %s — s to hide", what, detailRelativeTime(group[len(group)-1].at, now))
			lines = append(lines, ansi.Truncate(dim.Render(hint), w, ""))
			for _, g := range group {
				body := strings.Join(strings.Fields(g.event), " ")
				lines = append(lines, ansi.Truncate(dim.Render("    "+body), w, "…"))
			}
		}
		i = j
	}
	return lines
}

// detailDescription renders the ticket text: markdown-ish headings bold, list
// items with a hanging indent so wrapped lines stay under the text.
func (m Model) detailDescription(t tasks.Task, w int) []string {
	text := t.Text
	if first, rest, _ := strings.Cut(text, "\n"); first == boardTitle(t) {
		text = rest
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	head := m.boardStyle(m.theme.TextPrimary).Bold(true)
	var out []string
	blank := func() {
		if len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}
	}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		switch {
		case line == "":
			blank()
		case detailHeading(line):
			blank()
			out = append(out, m.detailBlock(head.Render(detailHeadingText(line)), "  ", "", w)...)
		case detailListLine(line):
			marker, body, _ := detailListItem(line)
			out = append(out, m.detailBlock(body, "  ", marker, w)...)
		default:
			out = append(out, m.detailBlock(line, "  ", "", w)...)
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// detailHeadings are the short section words the ticket format uses; they read
// as headings even without a leading "##".
var detailHeadings = []string{"goal", "goals", "design", "acceptance", "acceptance criteria", "out of scope", "context", "problem", "plan", "notes", "todo", "tasks", "done when", "verify", "verification", "how"}

// detailHeading reports a markdown-ish heading line.
func detailHeading(line string) bool {
	if strings.HasPrefix(line, "#") {
		return true
	}
	if strings.ContainsAny(line, ".!?,;") || ansi.StringWidth(line) > 40 {
		return false
	}
	return slices.Contains(detailHeadings, strings.ToLower(detailHeadingText(line)))
}

// detailHeadingText strips the markdown marks and the trailing colon.
func detailHeadingText(line string) string {
	line = strings.TrimSpace(strings.TrimLeft(line, "#"))
	return strings.TrimSuffix(line, ":")
}

// detailListLine reports a bullet or numbered list line.
func detailListLine(line string) bool {
	_, _, ok := detailListItem(line)
	return ok
}

// detailListItem splits a list line into its marker and its text.
func detailListItem(line string) (marker, body string, ok bool) {
	for _, p := range []string{"- ", "* ", "• "} {
		if strings.HasPrefix(line, p) {
			return "• ", strings.TrimSpace(line[len(p):]), true
		}
	}
	if i := strings.IndexAny(line, ".)"); i > 0 && i <= 3 && strings.Trim(line[:i], "0123456789") == "" {
		if i+1 < len(line) && line[i+1] == ' ' {
			return line[:i+1] + " ", strings.TrimSpace(line[i+1:]), true
		}
	}
	return "", "", false
}

// detailBlock wraps text at indent and hangs continuation lines under the text
// rather than under the marker.
func (m Model) detailBlock(text, indent, marker string, w int) []string {
	pad := indent + strings.Repeat(" ", ansi.StringWidth(marker))
	width := max(1, w-ansi.StringWidth(pad))
	out := make([]string, 0, 2)
	for _, line := range strings.Split(ansi.Wrap(text, width, ""), "\n") {
		prefix := pad
		if len(out) == 0 {
			prefix = indent + marker
		}
		out = append(out, ansi.Truncate(prefix+line, w, ""))
	}
	return out
}

// detailSize is a short human size for the attachments list.
func detailSize(n int64) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	case n < 1<<30:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
}

// detailViewport windows the body to exactly rows lines: the content plus a
// dim "↑ N more" / "↓ N more" row, blank when nothing is hidden so the text
// does not jump while scrolling.
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

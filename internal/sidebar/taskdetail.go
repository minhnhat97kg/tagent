package sidebar

import (
	"cmp"
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

// The ticket panel's tabs: what the ticket is, the ticket's chat, the
// conversation about it, and what actually ran. The summary above them never
// changes with the tab.
const (
	detailTabDetails = iota
	detailTabChat
	detailTabActivity
	detailTabRun
)

var detailTabs = []string{"Details", "Chat", "Activity", "Run"}

// detailFooter is the sticky key line inside the drawer, always the last row.
// Each tab line is shortened rather than cut, so a narrow pane keeps the keys
// that work where the cursor is.
func (m Model) detailFooter(w int) string {
	ladder := []string{"tab/1-4 tabs · j/k scroll · e edit · C comment · a files · s events · esc close", "tab tabs · j/k scroll · e edit · C · a files · s · esc", "tab · j/k · esc"}
	if m.commentBox {
		ladder = []string{"enter send · tab @role · esc cancel", "enter · tab · esc"}
	}
	if m.detailTab == detailTabActivity && !m.commentBox {
		ladder = []string{"C comment · s system events · tab/1-4 tabs · j/k scroll · esc close", "C comment · s events · tab · j/k · esc", "C · s · esc"}
	}
	if m.detailTab == detailTabChat {
		ladder = []string{"i message · S save reply · tab/1-4 tabs · j/k scroll · esc close", "i message · S save · tab · j/k · esc", "i · S · esc"}
	}
	keys := ladder[len(ladder)-1]
	for _, line := range ladder {
		if ansi.StringWidth(line) <= w {
			keys = line
			break
		}
	}
	return m.boardStyle(m.theme.TextDim).Render(keys)
}

// detailTabTitle is the ticket panel's tab row, drawn in the frame's top
// border (like the Tasks card tabs): the active tab bracketed and bold, the
// others muted.
func (m Model) detailTabTitle() string {
	strip, _ := m.tabStrip(detailTabs, m.detailTab, 0)
	return strip
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
	outer := w
	if w >= 8 {
		w -= 4 // the summary is a pane too: border and a space on each side
	}
	now := time.Now()
	dim := m.boardStyle(m.theme.TextDim)
	var lines []string
	add := func(text string) { lines = append(lines, ansi.Truncate(text, w, "")) }

	status := statusGlyph(t.Status) + " " + t.Status
	head := m.boardStyle(m.boardColor(t.Status)).Reverse(true).Bold(true).Render(status)
	// The lane and the step only show when they say something the status does not.
	var extra []string
	if lane := m.laneOf(t.Status); lane != t.Status {
		extra = append(extra, lane)
	}
	if t.Workflow != "" {
		if step := m.detailStep(t); step != "" && step != t.Status {
			extra = append(extra, step)
		}
	}
	if len(extra) > 0 {
		head += "  " + dim.Render(strings.Join(extra, " · "))
	}
	if t.ID != "" {
		head = dim.Render("#"+t.ID) + "  " + head
	}
	title := m.boardStyle(m.theme.TextPrimary).Bold(true).Render(boardTitle(t))
	titleLines := strings.Split(ansi.Wrap(title, max(1, w), ""), "\n")
	if len(titleLines) > 2 { // a long title never eats the drawer
		titleLines = append(titleLines[:1], ansi.Truncate(titleLines[1], max(1, w-1), "")+"…")
	}
	for _, line := range titleLines {
		add(line)
	}
	// A Chat tab that cannot be entered says why, right where the tabs are: a
	// TUI has no tooltips, so the reason has to stay on screen.
	if m.chat.Loaded && m.chat.ID == t.ID && !m.chat.available() {
		lines = append(lines, ansi.Truncate(m.boardStyle(m.theme.StatusError).Render("Chat unavailable")+dim.Render(" · "+m.chat.Reason), w, ""))
	}
	if t.Status == "blocked" {
		question := m.detailQuestion(t)
		if question == "" {
			question = "reply with @role"
		}
		blocked := m.boardStyle(m.theme.StatusError).Bold(true).Render("Blocked: " + question)
		lines = append(lines, m.detailBlock(blocked, "", "", w)...)
	}

	field := func(label, value string) {
		if w < 20 {
			lines = append(lines, m.detailBlock(value, "", "", w)...)
			return
		}
		prefix := dim.Render(fmt.Sprintf("%-*s", detailLabelWidth, label))
		width := max(1, w-detailLabelWidth)
		for i, line := range strings.Split(ansi.Wrap(value, width, ""), "\n") {
			if i > 0 {
				prefix = strings.Repeat(" ", detailLabelWidth)
			}
			add(prefix + line)
		}
	}

	names := make([]string, len(t.Assignees))
	for i, name := range t.Assignees {
		names[i] = m.boardStyle(m.detailAgentColor(name)).Render(name)
	}
	assignee := strings.Join(names, ", ")
	// One row: who the ticket is assigned to, and the agent running it now.
	if worker := m.boardWorker(t); worker != "" {
		if assignee != "" {
			assignee += dim.Render(" · ")
		}
		assignee += worker
	}
	if assignee == "" {
		assignee = dim.Render("unassigned")
	}
	field("Assignee", assignee+dim.Render(" · @ assign"))
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
	if mark := tasks.PriorityMark(t.Priority); mark != "" {
		field("Priority", m.boardStyle(m.theme.TextPrimary).Render(mark+" "+tasks.NormalizePriority(t.Priority)))
	}
	if t.Workflow != "" {
		value := t.Workflow
		if step := m.detailStep(t); step != "" {
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
		field("Time", shortDuration(total))
	}
	age := detailRelativeTime(t.Created, now) + dim.Render(" · updated ") + detailRelativeTime(t.Updated, now)
	if !t.CompletedAt.IsZero() {
		age += dim.Render(" · done ") + detailRelativeTime(t.CompletedAt, now)
	}
	field("Created", age)
	return m.detailFrame(head+dim.Render("  m move · H/L"), lines, outer)
}

// detailStep is the workflow step the ticket sits in: its role, else its job.
func (m Model) detailStep(t tasks.Task) string {
	cfg, _ := tasks.ConfigForTask(m.boardDir, m.boardConfig, t)
	for _, s := range cfg.Statuses {
		if s.ID == t.Status {
			return cmp.Or(s.Role, s.Job)
		}
	}
	return ""
}

// dependents is the tickets waiting for id, in ticket order.
func (m Model) dependents(id string) []tasks.Task {
	var out []tasks.Task
	for _, t := range m.notes {
		if slices.Contains(t.Needs, id) {
			out = append(out, t)
		}
	}
	return out
}

// detailQuestion is the pending question of a blocked ticket on one line.
func (m Model) detailQuestion(t tasks.Task) string {
	cfg, _ := tasks.ConfigForTask(m.boardDir, m.boardConfig, t)
	_, _, question := tasks.BlockContext(t, cfg, m.boardHistory)
	return strings.Join(strings.Fields(question), " ")
}

// taskDetailBody is the scrolling part of the ticket panel, for the active tab:
// the ticket's own facts, the conversation, or the run.
func (m Model) taskDetailBody(t tasks.Task, w int) []string {
	if w <= 0 {
		return nil
	}
	switch m.detailTab {
	case detailTabChat:
		return m.taskDetailChat(t, w)
	case detailTabActivity:
		return m.taskDetailActivity(t, w)
	case detailTabRun:
		return m.taskDetailRun(t, w)
	}
	return m.taskDetailDetails(t, w)
}

// taskDetailDetails is the Details tab: description, attachments, note and the
// agent runs the ticket's summary counts.
func (m Model) taskDetailDetails(t tasks.Task, w int) []string {
	if w <= 0 {
		return nil
	}
	now := time.Now()
	dim := m.boardStyle(m.theme.TextDim)
	iw := max(1, w-4) // inside a pane: border and a space on each side
	var lines []string
	pane := func(name, hint string, content []string) {
		lines = append(lines, m.detailPane(name, hint, content, w)...)
	}
	block := func(text string) []string { return m.detailBlock(text, "", "", iw) }
	none := func(text string) []string { return []string{dim.Render(text)} }

	desc := m.detailDescription(t, iw)
	for i := range desc { // the description helper indents for the old flat layout
		desc[i] = strings.TrimPrefix(desc[i], "  ")
	}
	if len(desc) == 0 {
		desc = none("No description")
	}
	switch {
	case len(desc) <= detailDescLines:
	case m.detailDescOpen:
		desc = append(slices.Clone(desc), "", dim.Render("… enter/space collapse"))
	default:
		desc = append(append([]string{}, desc[:detailDescLines]...), dim.Render(fmt.Sprintf("… %d more lines · enter/space expand", len(desc)-detailDescLines)))
	}
	pane("Description", "e edit", desc)

	files := none("None")
	if len(m.attachments) > 0 {
		files = nil
		for _, a := range m.attachments {
			files = append(files, block(m.icons.Attach+" "+a.Name+dim.Render(" · "+detailSize(a.Size)))...)
		}
	}
	pane(fmt.Sprintf("Attachments %d", len(m.attachments)), "a view · A attach", files)

	needs := none("None")
	if len(t.Needs) > 0 {
		needs = nil
		for _, id := range t.Needs {
			status, name := "missing", ""
			for _, n := range m.notes {
				if n.ID == id {
					status, name = n.Status, boardTitle(n)
					break
				}
			}
			line := m.boardStyle(m.boardColor(status)).Render("#" + id + " " + status)
			if name != "" {
				line += dim.Render(" · ") + name
			}
			needs = append(needs, block(line)...)
		}
	}
	pane(fmt.Sprintf("Dependencies %d", len(t.Needs)), "D edit", needs)

	if dependents := m.dependents(t.ID); len(dependents) > 0 {
		var blocks []string
		for _, d := range dependents {
			blocks = append(blocks, block(m.boardStyle(m.boardColor(d.Status)).Render("#"+d.ID+" "+d.Status)+dim.Render(" · ")+boardTitle(d))...)
		}
		pane(fmt.Sprintf("Blocks %d", len(dependents)), "", blocks)
	}
	if t.Note != "" {
		pane("Note", "", block(t.Note))
	}
	cfg := m.boardConfig
	if c, err := tasks.ConfigForTask(m.boardDir, cfg, t); err == nil {
		cfg = c
	}
	var usage []string
	for _, l := range metrics.TicketUsageOf(m.metricState, t, cfg, m.boardHistory, now).Lines(compactTokens) {
		usage = append(usage, block(l)...)
	}
	if t.Status == "doing" {
		running := false
		for _, r := range metrics.RunsOf(m.metricState, t.ID) {
			running = running || r.Ended.IsZero()
		}
		if !running && len(metrics.RunsOf(m.metricState, t.ID)) > 0 {
			usage = append(usage, block(m.boardStyle(m.theme.StatusError).Render("! no agent is running this task"))...)
		}
	}
	if len(usage) > 0 {
		pane("Usage", "", usage)
	}
	return lines
}

// detailPane is one section of the Details tab as its own pane: a light
// rounded frame with the name (and the keys that act on it) in the top border.
func (m Model) detailPane(name, hint string, content []string, w int) []string {
	if w < 8 {
		return content
	}
	edge := m.boardStyle(m.theme.TextDim)
	title := m.boardStyle(m.theme.TextPrimary).Bold(true).Render(name)
	if hint != "" {
		title += edge.Render(" · " + hint)
	}
	return m.detailFrame(title, content, w)
}

// detailFrame draws content inside a light rounded frame; title is already
// styled and sits in the top border.
func (m Model) detailFrame(title string, content []string, w int) []string {
	if w < 8 {
		return content
	}
	edge := m.boardStyle(m.theme.TextDim)
	title = ansi.Truncate(" "+title+" ", w-4, "…")
	out := []string{edge.Render("╭─") + title + edge.Render(strings.Repeat("─", max(0, w-3-ansi.StringWidth(title)))+"╮")}
	for _, l := range content {
		l = ansi.Truncate(l, w-4, "")
		out = append(out, edge.Render("│")+" "+l+strings.Repeat(" ", max(0, w-4-ansi.StringWidth(l)))+" "+edge.Render("│"))
	}
	return append(out, edge.Render("╰"+strings.Repeat("─", w-2)+"╯"))
}

// taskDetailActivity is the Activity tab: comments and the folded dispatch
// noise, newest last, as they always have been.
func (m Model) taskDetailActivity(t tasks.Task, w int) []string {
	if w <= 0 {
		return nil
	}
	lines := m.detailSection(fmt.Sprintf("Activity · %d", m.detailCommentCount(t)), w)
	lines = append(lines, m.replyingLines(t, w)...)
	return append(lines, m.detailActivity(t, w, time.Now())...)
}

// taskDetailRun is the Run tab: every entry into every step, what it returned,
// who ran it and how long it took, straight from the same events the web reads.
func (m Model) taskDetailRun(t tasks.Task, w int) []string {
	if w <= 0 {
		return nil
	}
	now := time.Now()
	dim := m.boardStyle(m.theme.TextDim)
	if t.Workflow == "" {
		return []string{ansi.Truncate("  "+dim.Render("No workflow on this ticket"), w, "")}
	}
	cfg, err := tasks.ConfigForTask(m.boardDir, m.boardConfig, t)
	if err != nil {
		return m.detailBlock(m.boardStyle(m.theme.StatusError).Render("! "+err.Error()), "  ", "", w)
	}
	entries := tasks.BuildRuns(t, cfg, m.boardHistory, now)
	return m.detailRunLines(t, cfg, entries, w, now)
}

// detailRunLines renders a ticket's run: the totals, the flow strip and one
// block per entry. Split from taskDetailRun so the rendering can be tested
// without a real workflow on disk.
func (m Model) detailRunLines(t tasks.Task, cfg tasks.BoardConfig, entries []tasks.RunEntry, w int, now time.Time) []string {
	if w <= 0 {
		return nil
	}
	dim := m.boardStyle(m.theme.TextDim)
	var lines []string
	push := func(l ...string) { lines = append(lines, l...) }
	if len(entries) == 0 {
		return []string{ansi.Truncate("  "+dim.Render("This ticket has not entered a step yet."), w, "")}
	}
	end := now
	if last := entries[len(entries)-1]; last.End != nil {
		end = *last.End
	}
	head := fmt.Sprintf("%d %s · total %s · started %s", len(entries), plural(len(entries), "entry", "entries"), shortDuration(end.Sub(entries[0].Start)), detailRelativeTime(entries[0].Start, now))
	push(m.detailBlock(head, "  ", "", w)...)
	push("")
	// The flow: every step of the recipe, entered ones with their count.
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Step]++
	}
	var flow []string
	for _, s := range cfg.Statuses {
		if s.Job == "" {
			continue
		}
		label := s.Job
		if n := counts[s.ID]; n > 0 {
			label += fmt.Sprintf(" %d×", n)
		}
		switch {
		case s.ID == t.Status:
			label = m.boardStyle(m.theme.TextPrimary).Bold(true).Render(label)
		case counts[s.ID] > 0:
			label = m.boardStyle(m.theme.TextMuted).Render(label)
		default:
			label = dim.Render(label)
		}
		flow = append(flow, label)
	}
	if len(flow) > 0 {
		push(m.detailBlock(strings.Join(flow, dim.Render(" › ")), "  ", "", w)...)
		push("")
	}
	for i, e := range entries {
		var prev tasks.RunEntry
		if i > 0 {
			push("")
			prev = entries[i-1]
		}
		push(m.detailBlock(m.boardStyle(m.theme.TextPrimary).Render(detailRunLabel(e, prev, i > 0)), "  ", "● ", w)...)
		parts := []string{m.detailRunResult(e.Result)}
		if who := strings.Join(nonEmpty(e.Role, e.Harness, e.Model), " · "); who != "" {
			parts = append(parts, who)
		}
		parts = append(parts, shortDuration(e.Duration))
		if e.End == nil {
			parts = append(parts, "(running)")
		} else {
			parts = append(parts, detailRelativeTime(e.Start, now))
		}
		push(m.detailBlock(strings.Join(parts, dim.Render(" · ")), "    ", "", w)...)
	}
	return lines
}

// detailRunLabel names an entry and why the step ran again, like the web run
// view: "build #2 after review: changes".
func detailRunLabel(e, prev tasks.RunEntry, hasPrev bool) string {
	if e.Loop < 2 {
		return e.Step
	}
	label := fmt.Sprintf("%s #%d", e.Step, e.Loop)
	switch {
	case !hasPrev:
		return label
	case prev.Result == "blocked":
		return label + " after blocked"
	case prev.Step == e.Step:
		return label + " after retry"
	}
	return fmt.Sprintf("%s after %s: %s", label, prev.Step, prev.Result)
}

// detailRunResult colours how an entry ended: pass/fail/changes/blocked/retry.
func (m Model) detailRunResult(result string) string {
	label := result
	color := m.theme.TextMuted
	switch result {
	case "pass":
		color = m.theme.StatusDone
	case "fail", "blocked":
		color = m.theme.StatusError
	case "changes", "retry":
		color = m.theme.StatusWaiting
	case "current":
		color = m.theme.Accent
	}
	if label == "" {
		label = "unknown"
	}
	return m.boardStyle(color).Render(label)
}

// plural picks the singular or the plural word for n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// nonEmpty drops the empty strings from a list.
func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
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
			head := "  ● " + m.boardStyle(m.detailAgentColor(c.Author)).Bold(true).Render(c.Author) + dim.Render(" · "+detailRelativeTime(c.Created, now))
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

// detailTabKey switches the ticket panel's tab: tab/l and shift+tab/h cycle,
// 1-4 jump straight to Details, Chat, Activity or Run. Scrolling restarts at the
// top so a new tab never opens half way down. A Chat tab that cannot be entered
// (its pane's harness cannot be read) is skipped by cycling and explained when
// it is asked for by number.
func (m *Model) detailTabKey(key string) bool {
	step := 0
	cycle := false
	switch key {
	case "tab", "l":
		step, cycle = 1, true
	case "shift+tab", "h":
		step, cycle = -1, true
	case "1", "2", "3", "4":
		step = int(key[0]-'1') - m.detailTab
	default:
		return false
	}
	next := ((m.detailTab+step)%len(detailTabs) + len(detailTabs)) % len(detailTabs)
	if next == detailTabChat && m.chatTabBlocked() {
		if !cycle {
			m.taskNotice = "Chat unavailable: " + m.chat.Reason
			return true
		}
		dir := 1
		if step < 0 {
			dir = -1
		}
		next = ((next+dir)%len(detailTabs) + len(detailTabs)) % len(detailTabs)
	}
	m.detailTab = next
	m.detailScroll = 0
	return true
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

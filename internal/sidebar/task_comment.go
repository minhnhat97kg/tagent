package sidebar

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/tasks"
)

// The Activity tab's inline composer: "c" on the Activity tab opens a comment
// box above the conversation. Typing "@" suggests roles (board.yaml roles and
// the agents on the board); a comment with @role hands the ticket to that role,
// and the role shows as replying until its reply comment arrives.

// commentBoxOpen opens the composer on the Activity tab of the open ticket.
func (m Model) commentBoxOpen() Model {
	if len(m.notes) == 0 {
		return m
	}
	m.commentBox, m.taskInput, m.commentPick = true, "", 0
	return m
}

// handleCommentBox routes keys while the composer is open: enter sends (or
// completes the highlighted role), tab completes, esc cancels.
func (m Model) handleCommentBox(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	options := m.commentOptions()
	switch key.Type {
	case tea.KeyEsc:
		m.commentBox, m.taskInput = false, ""
	case tea.KeyUp, tea.KeyDown:
		if len(options) > 0 {
			step := 1
			if key.Type == tea.KeyUp {
				step = len(options) - 1
			}
			m.commentPick = (m.commentPick + step) % len(options)
		}
	case tea.KeyTab:
		m = m.completeMention(options)
	case tea.KeyEnter:
		if len(options) > 0 {
			return m.completeMention(options), nil
		}
		text := strings.TrimSpace(m.taskInput)
		m.commentBox, m.taskInput = false, ""
		if text == "" || len(m.notes) == 0 {
			return m, nil
		}
		scope, id := m.notesScope, m.notes[m.taskCursor].ID
		return m, func() tea.Msg { return actionResult("Comment added", tasks.Comment(scope, id, "user", text, nil)) }
	case tea.KeyBackspace, tea.KeyDelete, tea.KeyRunes, tea.KeySpace:
		m.taskInput, _ = editText(m.taskInput, key)
		m.commentPick = 0
	}
	return m, nil
}

// completeMention replaces the "@word" being typed with the highlighted role.
func (m Model) completeMention(options []string) Model {
	start, _, ok := mentionAt(m.taskInput)
	if !ok || len(options) == 0 {
		return m
	}
	m.taskInput = m.taskInput[:start] + "@" + options[m.commentPick%len(options)] + " "
	m.commentPick = 0
	return m
}

// mentionAt finds an "@word" that ends the text: where it starts and what has been typed after the @.
func mentionAt(text string) (start int, query string, ok bool) {
	i := strings.LastIndex(text, "@")
	if i < 0 || (i > 0 && text[i-1] != ' ' && text[i-1] != '\n') {
		return 0, "", false
	}
	q := text[i+1:]
	if strings.ContainsAny(q, " \t\n") {
		return 0, "", false
	}
	return i, strings.ToLower(q), true
}

// commentOptions are the roles the typed "@word" can complete to: the board's
// roles and the agents on the board, filtered by what has been typed.
func (m Model) commentOptions() []string {
	_, query, ok := mentionAt(m.taskInput)
	if !ok {
		return nil
	}
	var names []string
	for role := range m.boardConfig.Roles {
		names = append(names, role)
	}
	for _, r := range m.rows {
		if !r.IsHeader && !r.IsRepoAgent && r.Base != "" {
			names = append(names, r.Base)
		}
	}
	slices.Sort(names)
	var out []string
	for i, n := range names {
		if strings.HasPrefix(strings.ToLower(n), query) && (i == 0 || names[i-1] != n) {
			out = append(out, n)
		}
	}
	return out
}

// commentBoxLines is the composer shown at the top of the Activity tab.
func (m Model) commentBoxLines(w int) []string {
	dim := m.boardStyle(m.theme.TextDim)
	var lines []string
	lines = append(lines, m.detailBlock(m.boardStyle(m.theme.TextPrimary).Render(m.taskInput)+"▏", "  ", "✎ ", w)...)
	if options := m.commentOptions(); len(options) > 0 {
		var chips []string
		for i, o := range options {
			chip := "@" + o
			if i == m.commentPick%len(options) {
				chip = m.boardStyle(m.theme.Accent).Bold(true).Render("[" + chip + "]")
			}
			chips = append(chips, chip)
		}
		lines = append(lines, ansi.Truncate("    "+strings.Join(chips, "  "), w, ""))
	}
	return append(lines, ansi.Truncate("  "+dim.Render("enter send · tab complete @role · esc cancel"), w, ""))
}

// replyingRoles lists the roles that were @mentioned on the ticket, have a
// pane working on it now, and have not commented since the mention.
func (m Model) replyingRoles(t tasks.Task) []string {
	var out []string
	for _, r := range m.rows {
		if r.IsHeader || r.IsRepoAgent || r.PaneID == "" || !r.Status.IsActive() {
			continue
		}
		role, id, _ := strings.Cut(r.BoardTarget, "#")
		if id != t.ID || role == "" || slices.Contains(out, role) {
			continue
		}
		asked := -1
		for i, c := range t.Comments {
			if mentions(c.Body, role) {
				asked = i
			}
		}
		replied := -1
		for i, c := range t.Comments {
			if c.Author == role {
				replied = i
			}
		}
		if asked > replied {
			out = append(out, role)
		}
	}
	return out
}

// mentions reports whether body tags role as "@role", not as a longer name.
func mentions(body, role string) bool {
	for rest := body; ; {
		i := strings.Index(rest, "@"+role)
		if i < 0 {
			return false
		}
		after := rest[i+len(role)+1:]
		if after == "" || !(isNameRune(after[0])) {
			return true
		}
		rest = after
	}
}

func isNameRune(c byte) bool {
	return c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// replyingLines is the muted "role replying…" row at the top of the Activity list.
func (m Model) replyingLines(t tasks.Task, w int) []string {
	dim := m.boardStyle(m.theme.TextDim)
	var lines []string
	for _, role := range m.replyingRoles(t) {
		lines = append(lines, ansi.Truncate("  "+dim.Render("● "+role+" replying…"), w, ""))
	}
	return lines
}

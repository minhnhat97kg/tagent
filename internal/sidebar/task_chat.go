package sidebar

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/transcript"
)

// The Chat tab reads the transcript of the pane bound to the ticket (the same
// conversation the web's Chat tab shows, see docs/agent-chat.md). It is polled
// quickly only while it is on screen, and slowly from other tabs so the tab
// strip knows whether Chat can be entered at all.
const (
	chatPollTTL = 2 * time.Second
	chatIdleTTL = 30 * time.Second
	chatItems   = 40 // messages kept for the drawer; older ones stay in the pane
)

// ticketChat is the loaded state of one ticket's chat pane.
type ticketChat struct {
	Loaded  bool
	ID      string // the ticket this state belongs to
	Key     string // tasks.TicketScope: "<project scope>#<id>"
	Pane    string // "" until the chat is started
	Harness string
	Status  string
	Reason  string // why Chat cannot be entered; empty means it can
	Items   []transcript.Item
	At      time.Time
}

// available reports whether the Chat tab can be entered: a readable harness and
// a session to read. A ticket whose chat was never started is available.
func (c ticketChat) available() bool { return c.Reason == "" }

// started reports whether a chat pane exists for the ticket.
func (c ticketChat) started() bool { return c.Pane != "" && c.available() }

// ticketChatMsg carries a freshly read chat state.
type ticketChatMsg struct{ Chat ticketChat }

// chatStartedMsg is the result of spawning the ticket's chat pane.
type chatStartedMsg struct {
	ID  string
	Err error
}

// chatLoadCmd reloads the chat of the ticket under the cursor when its state is
// missing or older than its TTL. It returns nil when nothing needs reading, so
// the 1s refresh tick stays free of file I/O on the other tabs.
func (m Model) chatLoadCmd() (Model, tea.Cmd) {
	if !m.boardMode || !m.boardDrawer || len(m.notes) == 0 || m.boardDir == "" {
		return m, nil
	}
	t := m.notes[max(0, min(m.taskCursor, len(m.notes)-1))]
	if m.chatBusy {
		return m, nil
	}
	ttl := chatIdleTTL
	if m.detailTab == detailTabChat {
		ttl = chatPollTTL
	}
	if m.chat.Loaded && m.chat.ID == t.ID && time.Since(m.chat.At) < ttl {
		return m, nil
	}
	m.chatBusy = true
	return m, loadTicketChat(m.boardDir, t.ID)
}

// loadTicketChat reads the pane bound to the ticket and the last page of its
// transcript. Nothing is spawned here: starting is the human's key (enter).
func loadTicketChat(dir, id string) tea.Cmd {
	return func() tea.Msg {
		chat := ticketChat{Loaded: true, ID: id, Key: tasks.TicketScope(dir, id), At: time.Now()}
		panes, err := tmux.TicketPanes()
		if err != nil {
			chat.Reason = "tmux did not list panes: " + err.Error()
			return ticketChatMsg{chat}
		}
		p, ok := ticketChatPane(panes, chat.Key)
		if !ok {
			return ticketChatMsg{chat}
		}
		chat.Pane, chat.Harness, chat.Status = p.PaneID, p.Agent, string(p.Status)
		switch p.Agent {
		case "":
			chat.Reason = p.PaneID + " is bound to this ticket but has no agent yet; it is still starting."
			return ticketChatMsg{chat}
		case "claude", "pi":
		default:
			chat.Reason = fmt.Sprintf("Chat needs a claude or pi agent, but %s runs %s.", p.PaneID, p.Agent)
			return ticketChatMsg{chat}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		page, err := transcript.Read(ctx, p.Agent, tmux.GetPaneOptionValue(p.PaneID, tmux.PaneSessionID), "")
		switch {
		case errors.Is(err, transcript.ErrUnsupported):
			chat.Reason = "Chat cannot read a " + p.Agent + " transcript."
		case err != nil:
			chat.Reason = "The transcript is not readable yet; " + p.PaneID + " is still starting."
		default:
			chat.Items = page.Items
		}
		return ticketChatMsg{chat}
	}
}

// ticketChatPane picks the pane bound to key, preferring one Chat can read so a
// stale non-claude pane does not hide a usable chat.
func ticketChatPane(panes []tmux.TicketPane, key string) (tmux.TicketPane, bool) {
	var found tmux.TicketPane
	ok := false
	for _, p := range panes {
		if p.Ticket != key {
			continue
		}
		if !ok || (chatHarness(p.Agent) && !chatHarness(found.Agent)) {
			found, ok = p, true
		}
	}
	return found, ok
}

func chatHarness(agent string) bool { return agent == "claude" || agent == "pi" }

// startTicketChat spawns the ticket's chat through `tagent chat`, which owns the
// @pane_ticket binding, the reused pane and the first-message context.
func startTicketChat(dir, id string) tea.Cmd {
	return func() tea.Msg {
		bin, err := os.Executable()
		if err != nil {
			return chatStartedMsg{ID: id, Err: err}
		}
		cmd := exec.Command(bin, "chat", "--dir", dir, id)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			text := strings.TrimSpace(string(out))
			if text == "" {
				text = err.Error()
			}
			return chatStartedMsg{ID: id, Err: errors.New(text)}
		}
		return chatStartedMsg{ID: id}
	}
}

// sendTicketChat sends text to the chat pane through `tagent send`, which types
// into an idle agent and queues the message while it is busy or waiting.
func sendTicketChat(pane, text string) tea.Cmd {
	return func() tea.Msg {
		bin, err := os.Executable()
		if err != nil {
			return actionResult("", err)
		}
		if out, err := exec.Command(bin, "send", pane, text).CombinedOutput(); err != nil {
			text := strings.TrimSpace(string(out))
			if text == "" {
				text = err.Error()
			}
			return actionResult("", errors.New(text))
		}
		return actionResult("Sent to the chat", nil)
	}
}

// chatTabBlocked reports whether the Chat tab cannot be entered for the ticket
// under the cursor: its pane runs a harness whose transcript cannot be read.
func (m Model) chatTabBlocked() bool {
	if len(m.notes) == 0 {
		return false
	}
	t := m.notes[max(0, min(m.taskCursor, len(m.notes)-1))]
	return m.chat.Loaded && m.chat.ID == t.ID && !m.chat.available()
}

// saveChatReply is the human's "save to ticket": it posts the newest reply as a
// comment, so what the chat settled survives the pane and reaches the agents.
func (m Model) saveChatReply() Model {
	if len(m.notes) == 0 {
		return m
	}
	reply := chatLastReply(m.chat.Items)
	if reply == "" {
		m.taskNotice = "No chat reply to save yet"
		return m
	}
	scope, id := m.notesScope, m.notes[m.taskCursor].ID
	err := tasks.Comment(scope, id, "user", reply, nil)
	if _, ok := err.(*tasks.CommentNotice); ok {
		m.taskNotice = "✓ Saved to ticket · " + err.Error()
	} else if err != nil {
		m.taskNotice = "! " + err.Error()
	} else {
		m.taskNotice = "✓ Saved to ticket"
	}
	m.chat = ticketChat{} // the new comment is part of the chat's next context
	return m
}

// saveChatInput posts what was typed in the chat composer as a ticket comment
// instead of sending it to the agent: the human's own words, kept on the board.
func (m Model) saveChatInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.taskInput)
	m.taskForm = ""
	if text == "" || len(m.notes) == 0 {
		return m, nil
	}
	scope, id := m.notesScope, m.notes[m.taskCursor].ID
	return m, func() tea.Msg { return actionResult("Saved to ticket", tasks.Comment(scope, id, "user", text, nil)) }
}

// chatLastReply is the newest assistant text: what "save to ticket" records.
func chatLastReply(items []transcript.Item) string {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Role == "assistant" && strings.TrimSpace(items[i].Text) != "" {
			return strings.TrimSpace(items[i].Text)
		}
	}
	return ""
}

// taskDetailChat is the Chat tab: the ticket's conversation, or why there is
// none. Rendering is the same shape as Activity: a head per message and the
// body wrapped under it.
func (m Model) taskDetailChat(t tasks.Task, w int) []string {
	if w <= 0 {
		return nil
	}
	dim := m.boardStyle(m.theme.TextDim)
	c := m.chat
	if !c.Loaded || c.ID != t.ID {
		return []string{ansi.Truncate("  "+dim.Render("Loading the chat…"), w, "")}
	}
	if !c.available() {
		lines := m.detailBlock(m.boardStyle(m.theme.StatusError).Bold(true).Render("Chat unavailable"), "  ", "", w)
		lines = append(lines, m.detailBlock(c.Reason, "  ", "", w)...)
		lines = append(lines, m.detailBlock(dim.Render("The ticket's comments and the Details tab do not depend on it."), "  ", "", w)...)
		return lines
	}
	if c.Pane == "" {
		lines := m.detailBlock("No chat for this ticket yet.", "  ", "", w)
		lines = append(lines, m.detailBlock(dim.Render("A claude or pi agent starts with a brief (summary, status, workflow step) and reads more with tagent task show. enter starts it."), "  ", "", w)...)
		return lines
	}
	now := time.Now()
	head := m.boardStyle(m.detailAgentColor(c.Harness)).Render(c.Harness) + dim.Render(" · "+c.Status+" · "+c.Pane)
	lines := m.detailBlock(head, "  ", "● ", w)
	if len(c.Items) == 0 {
		return append(lines, m.detailBlock(dim.Render("No messages yet."), "  ", "", w)...)
	}
	items := c.Items
	if len(items) > chatItems {
		lines = append(lines, m.detailBlock(dim.Render(fmt.Sprintf("… %d earlier messages (open the pane for the whole conversation)", len(items)-chatItems)), "  ", "", w)...)
		items = items[len(items)-chatItems:]
	}
	for _, item := range items {
		switch item.Role {
		case "user", "assistant": // the two voices; each body wraps under its head
			name, color := "you", m.theme.Accent
			if item.Role == "assistant" {
				name = nonEmptyText(item.Model, c.Harness)
				color = m.detailAgentColor(name)
			}
			head := "  ● " + m.boardStyle(color).Render(name)
			if stamp := chatTime(item.Time, now); stamp != "" {
				head += dim.Render(" · " + stamp)
			}
			lines = append(lines, ansi.Truncate(head, w, ""))
			if strings.TrimSpace(item.Text) != "" {
				lines = append(lines, m.detailBlock(item.Text, "    ", "", w)...)
			}
		case "thinking":
			lines = append(lines, ansi.Truncate(dim.Render("    … reasoning"), w, ""))
		case "tool":
			if item.Tool == nil {
				continue
			}
			name := item.Tool.Label
			if name == "" && item.Tool.Name != "" {
				name = item.Tool.Name
			}
			if name != "" {
				lines = append(lines, m.detailBlock(dim.Render("⚙ "+name), "    ", "", w)...)
			}
		}
	}
	return lines
}

// chatTime is a short relative time for a transcript item, which stores its time
// as the harness wrote it.
func chatTime(iso string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}
	return detailRelativeTime(t, now)
}

func nonEmptyText(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

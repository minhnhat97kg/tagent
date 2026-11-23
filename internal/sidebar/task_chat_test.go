package sidebar

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/tasks"
	"tagent/internal/transcript"
)

// chatModel is the board with the ticket drawer open on the Chat tab and a
// loaded chat for the first ticket.
func chatModel(chat ticketChat) Model {
	m := boardTestModel()
	m.width, m.height = 110, 40
	m.boardDir = "/repo"
	m.boardDrawer, m.detailTab = true, detailTabChat
	chat.Loaded, chat.ID = true, "1"
	m.chat = chat
	return m
}

func TestTicketChatUnavailableIsDisabledWithItsReason(t *testing.T) {
	m := chatModel(ticketChat{Pane: "%3", Harness: "codex", Reason: "Chat needs a claude or pi agent, but %3 runs codex."})
	// The reason stays on screen in the summary: a TUI has no tooltips.
	view := ansi.Strip(strings.Join(m.boardDetail(100, 30), "\n"))
	if !strings.Contains(view, "Chat unavailable") || !strings.Contains(view, "needs a claude or pi agent") {
		t.Fatalf("unavailable chat has no visible reason:\n%s", view)
	}
	if body := ansi.Strip(strings.Join(m.taskDetailBody(m.notes[0], 80), "\n")); !strings.Contains(body, "runs codex") {
		t.Fatalf("chat body does not explain itself:\n%s", body)
	}
	// Cycling skips the disabled tab, and a number key refuses it with a notice.
	m.detailTab = detailTabDetails
	next, _ := boardPress(m, "tab")
	if next.detailTab != detailTabActivity {
		t.Fatalf("tab landed on the disabled Chat tab (%d)", next.detailTab)
	}
	next, _ = boardPress(next, "2")
	if next.detailTab != detailTabActivity || !strings.Contains(next.taskNotice, "Chat unavailable") {
		t.Fatalf("number key entered the disabled chat tab: tab %d, notice %q", next.detailTab, next.taskNotice)
	}
}

func TestTicketChatShowsTheConversation(t *testing.T) {
	m := chatModel(ticketChat{Pane: "%7", Harness: "claude", Status: "idle", Items: []transcript.Item{
		{ID: "u.0", Role: "user", Text: "why is the build red?"},
		{ID: "t.0", Role: "tool", Tool: &transcript.Tool{Name: "Bash", Label: "Bash go test"}},
		{ID: "a.0", Role: "assistant", Text: "The queue is empty."},
	}})
	body := ansi.Strip(strings.Join(m.taskDetailBody(m.notes[0], 80), "\n"))
	for _, want := range []string{"you", "why is the build red?", "Bash go test", "The queue is empty."} {
		if !strings.Contains(body, want) {
			t.Fatalf("chat body misses %q:\n%s", want, body)
		}
	}
	if strings.Index(body, "why is the build red?") > strings.Index(body, "The queue is empty.") {
		t.Fatal("chat order is wrong")
	}
	// A chat that was never started offers to start it instead.
	empty := chatModel(ticketChat{})
	if body := ansi.Strip(strings.Join(empty.taskDetailBody(empty.notes[0], 80), "\n")); !strings.Contains(body, "No chat for this ticket yet") || !strings.Contains(body, "enter starts it") {
		t.Fatalf("unstarted chat body:\n%s", body)
	}
	// Starting is a key on that tab and never a side effect of opening the drawer.
	next, cmd := boardPress(empty, "enter")
	if cmd == nil || !strings.Contains(next.taskNotice, "Starting the chat") {
		t.Fatalf("enter did not start the chat: %q", next.taskNotice)
	}
}

func TestSaveChatReplyPostsAComment(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	scope := tasks.Scope(t.TempDir())
	added, err := tasks.Add(scope, "Chat ticket", nil)
	if err != nil {
		t.Fatal(err)
	}
	id := added.ID
	m := boardTestModel()
	m.notesScope, m.notes = scope, tasks.List(scope)
	m.boardDrawer, m.detailTab = true, detailTabChat
	m.chat = ticketChat{Loaded: true, ID: id, Pane: "%7", Harness: "claude", Items: []transcript.Item{
		{ID: "a.0", Role: "assistant", Text: "Keep the queue."},
	}}
	next, _ := boardPress(m, "S")
	if !strings.Contains(next.taskNotice, "Saved to ticket") {
		t.Fatalf("save said %q", next.taskNotice)
	}
	task, err := tasks.Get(scope, id)
	if err != nil {
		t.Fatal(err)
	}
	last := task.Comments[len(task.Comments)-1]
	if last.Author != "user" || last.Body != "Keep the queue." {
		t.Fatalf("saved comment is %+v", last)
	}
	if next.chat.Loaded {
		t.Fatal("the chat state was not invalidated for the next read")
	}
}

func TestChatComposerSendsAndSavesToTicket(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	scope := tasks.Scope(t.TempDir())
	added, err := tasks.Add(scope, "Chat composer", nil)
	if err != nil {
		t.Fatal(err)
	}
	id := added.ID
	m := boardTestModel()
	m.notesScope, m.notes = scope, tasks.List(scope)
	m.boardDrawer, m.detailTab = true, detailTabChat
	m.chat = ticketChat{Loaded: true, ID: id, Pane: "%7", Harness: "claude"}
	// i opens the composer; other tabs keep i as "close the drawer".
	m, _ = boardPress(m, "i")
	if m.taskForm != "chat" || !m.boardDrawer {
		t.Fatalf("i did not open the chat composer (form %q)", m.taskForm)
	}
	// Enter sends the text to the pane through `tagent send` (a command, not run here).
	m.taskInput = "look at the queue"
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || next.(Model).taskForm != "" {
		t.Fatal("enter did not send the chat message")
	}
	// ctrl+s saves the typed text as a ticket comment instead of sending it.
	m.taskInput = "Use the queue."
	next, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	saved := next.(Model)
	if saved.taskForm != "" || cmd == nil {
		t.Fatalf("ctrl+s did not save (form %q)", saved.taskForm)
	}
	msg, ok := cmd().(taskActionMsg)
	if !ok || !strings.Contains(msg.Text, "Saved to ticket") {
		t.Fatalf("save result: %#v", cmd())
	}
	task, err := tasks.Get(scope, id)
	if err != nil {
		t.Fatal(err)
	}
	if last := task.Comments[len(task.Comments)-1]; last.Body != "Use the queue." || last.Author != "user" {
		t.Fatalf("saved comment is %+v", last)
	}
}

func TestChatLoadsOnlyWhileTheDrawerShowsIt(t *testing.T) {
	m := boardTestModel()
	m.boardDir = t.TempDir()
	m.detailTab = detailTabChat
	if _, cmd := m.chatLoadCmd(); cmd != nil {
		t.Fatal("read the chat with the drawer closed")
	}
	m.boardDrawer = true
	loading, cmd := m.chatLoadCmd()
	if cmd == nil || !loading.chatBusy {
		t.Fatal("the chat tab did not load the chat")
	}
	if _, again := loading.chatLoadCmd(); again != nil {
		t.Fatal("a second read started while one was in flight")
	}
	// A chat read for another ticket is stale at once: Update asks for a new read.
	loaded, reload := loading.Update(ticketChatMsg{Chat: ticketChat{Loaded: true, ID: "other", At: time.Now()}})
	if reload == nil || loaded.(Model).chat.ID != "other" || !loaded.(Model).chatBusy {
		t.Fatal("a chat of another ticket was kept")
	}
}

func TestTicketChatFitsWidth(t *testing.T) {
	long := strings.Repeat("a long chat line with 界 and more words than fit ", 6)
	m := chatModel(ticketChat{Pane: "%7", Harness: "claude", Status: "idle", Items: []transcript.Item{
		{ID: "u.0", Role: "user", Text: long},
		{ID: "t.0", Role: "tool", Tool: &transcript.Tool{Name: "Bash", Label: long}},
		{ID: "a.0", Role: "assistant", Text: long, Model: "claude-opus-5-5"},
		{ID: "k.0", Role: "thinking", Text: long},
	}})
	for _, w := range []int{6, 12, 20, 40, 100} {
		for _, line := range m.taskDetailLines(m.notes[0], w) {
			if ansi.StringWidth(line) > w {
				t.Fatalf("chat overflow at width %d: %q", w, ansi.Strip(line))
			}
		}
	}
	// The reason is a line of its own, and it must fit too.
	off := chatModel(ticketChat{Reason: long})
	for _, w := range []int{6, 12, 20, 40, 100} {
		for _, line := range off.taskDetailLines(off.notes[0], w) {
			if ansi.StringWidth(line) > w {
				t.Fatalf("unavailable chat overflow at width %d: %q", w, ansi.Strip(line))
			}
		}
	}
}

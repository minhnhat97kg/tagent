package notify

import (
	"context"
	"strings"
	"testing"

	"tagent/internal/tmux"
)

func TestAlertCard(t *testing.T) {
	a := agentPane{ID: "%3", Name: "dev<1>", Agent: "claude"}
	info := paneInfo{Project: "tagent", Branch: "agent/x", Ticket: "#103 Custom <panes>"}
	screen := "│ Bash command\n│   rm -rf a && b\n│\n│ Do you want to proceed?\n│ ❯ 1. Yes\n│   2. No"
	opts := []dialogOpt{{"1", "Yes"}, {"2", "No"}}
	ctxLines := dialogContext(screen, "Do you want to proceed?", opts)
	if ctxLines != "Bash command\nrm -rf a && b" {
		t.Fatalf("context: %q", ctxLines)
	}
	got := alertCard(a, info, "needs your answer", "Do you want to proceed?", ctxLines)
	for _, want := range []string{"❓ <b>Needs your answer</b>", "dev&lt;1&gt; · claude · %3", "#103 Custom &lt;panes&gt;", "tagent · agent/x", "rm -rf a &amp;&amp; b"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	long := alertCard(a, info, "done "+strings.Repeat("&", 9000), "", "")
	if n := utf16Units(long); n > 4096 {
		t.Errorf("card is %d units", n)
	}
}

func TestParseTickets(t *testing.T) {
	rows := parseTickets("tagent", "1\tdone\t\ta\n103\tbuilding\tdev\tCustom panes\n9\tclosed\t\tz\n")
	if len(rows) != 1 || rows[0] != (ticketRow{"tagent", "103", "building", "Custom panes"}) {
		t.Errorf("tickets: %+v", rows)
	}
}

func fakeContext(t *testing.T) {
	oldPane, oldTickets := paneContext, listTickets
	paneContext = func(context.Context, string) paneInfo { return paneInfo{Project: "tagent"} }
	listTickets = func(context.Context, func() []string) []ticketRow {
		return []ticketRow{{"tagent", "103", "blocked", "Custom panes"}}
	}
	t.Cleanup(func() { paneContext, listTickets = oldPane, oldTickets })
}

func TestMenuEditsInPlace(t *testing.T) {
	r := setupRig(t, "tok", msg(1, 100, "/start", 0), tap(2, 100, "n|tickets"), tap(3, 100, "n|agents"))
	fakeContext(t)
	if err := pollOnce(loadConfig(), nil); err != nil {
		t.Fatal(err)
	}
	if !r.sentTo("sendMessage", "100", "Blocked tickets") || !r.sentTo("editMessageText", "100", "Custom panes") || !r.sentTo("editMessageText", "100", "p|rec") {
		t.Errorf("menu: %+v", r.http)
	}
}

func TestDialogCardLifecycle(t *testing.T) {
	r := setupRig(t, "tok")
	fakeContext(t)
	a := agentPane{ID: "%7", Name: "wait", Status: tmux.StatusWaiting}
	alert(context.Background(), "tok", 100, "needs your answer", a)
	alert(context.Background(), "tok", 100, "is still waiting", a)
	sends := 0
	for _, c := range r.http {
		if c.Method == "sendMessage" {
			sends++
		}
	}
	if sends != 1 || !r.sentTo("sendMessage", "100", "💬 Chat") {
		t.Fatalf("want one card with buttons, got %d: %+v", sends, r.http)
	}
	r.ups = []update{tap(4, 100, "a|%7|1|"+dialogHash("%7", []dialogOpt{{"1", "Yes"}, {"2", "No"}}))}
	if err := pollOnce(loadConfig(), nil); err != nil {
		t.Fatal(err)
	}
	if !r.sentTo("editMessageReplyMarkup", "100", "Answered: 1  Yes") || len(loadState().Alerts) != 0 {
		t.Errorf("outcome not shown or claim kept: %+v", r.http)
	}
}

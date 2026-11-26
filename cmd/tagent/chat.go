package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"tagent/internal/queue"
	"tagent/internal/roles"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

// cmdChat starts (or reuses) the one agent pane dedicated to a ticket's chat.
// The pane is bound to the ticket with @pane_ticket, so `tagent task` run in it
// refuses other tickets. The ticket context is sent once, as the first message;
// reusing the pane never re-sends it (--clear does).
func cmdChat(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "", "repository directory (default: the current directory)")
	agent := fs.String("agent", "", "harness for the chat (claude or pi; default: configured agent)")
	model := fs.String("model", "", "model (default: configured model)")
	clear := fs.Bool("clear", false, "restart the chat: close its pane and send the context again")
	if err := fs.Parse(args); err != nil || fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: tagent chat [--dir DIR] [--agent claude|pi] [--model NAME] [--clear] <ticket>")
		return exitUsage
	}
	id := fs.Arg(0)
	wd, _ := os.Getwd()
	if *dir == "" {
		*dir = wd
	}
	root := tasks.MainRoot(*dir)
	scope := tasks.Scope(root)
	if _, err := tasks.Get(scope, id); err != nil {
		fmt.Fprintf(stderr, "chat: no ticket %s in %s: %v\n", id, root, err)
		return exitFailure
	}
	harness := *agent
	if harness == "" {
		harness, _ = tmux.GetOption(tmux.AgentOption)
	}
	if !chatHarness(harness) {
		if *agent != "" {
			fmt.Fprintf(stderr, "chat: chat supports claude and pi, not %q\n", *agent)
			return exitUsage
		}
		harness = "claude" // the configured agent cannot render a transcript
	}
	context, err := tasks.ContextFor(scope, id)
	if err != nil {
		fmt.Fprintln(stderr, "chat:", err)
		return exitFailure
	}
	key := tasks.TicketScope(root, id)
	panes, err := tmux.TicketPanes()
	if err != nil {
		fmt.Fprintln(stderr, "chat:", err)
		return exitFailure
	}
	existing := ""
	for _, p := range panes {
		if p.Ticket != key {
			continue
		}
		if !chatHarness(p.Agent) {
			fmt.Fprintf(stderr, "chat: %s is bound to ticket %s but runs %s; chat needs claude or pi (use --agent with --clear)\n", tmux.PaneLabel(p.PaneID), id, p.Agent)
			return exitFailure
		}
		existing = p.PaneID
	}
	if existing != "" && !*clear {
		fmt.Fprintln(stdout, existing)
		return exitOK
	}
	if existing != "" {
		if _, err := tmux.RunTmuxCapture("kill-pane", "-t", existing); err != nil {
			fmt.Fprintln(stderr, "chat:", err)
			return exitFailure
		}
		queue.Clear(existing)
	}
	o := spawnOpts{Name: freeName("chat-" + id), Agent: harness, Model: *model, Cwd: root, ConfigDir: root, Ticket: id, Wait: true}
	pane, warn, err := spawnAgent(o)
	if err != nil {
		fmt.Fprintln(stderr, "chat:", err)
		return exitFailure
	}
	fmt.Fprintln(stdout, pane)
	if warn != "" {
		fmt.Fprintln(stderr, "chat:", warn)
	}
	prompt := fmt.Sprintf("You are the assistant for ticket %s. Answer here in the chat. To change the ticket, run `tagent task` commands in this pane — it is bound to this ticket, so it refuses other tickets. Comments and status moves are visible to the board and to the worker agents.\n\n%s", id, context)
	if err := sendTicketContext(pane, prompt); err != nil {
		// A pane without its context would be reused by the next attempt with no
		// way to deliver it: drop it so the retry starts clean.
		tmux.RunTmuxCapture("kill-pane", "-t", pane)
		queue.Clear(pane)
		fmt.Fprintln(stderr, "chat: started", pane, "but the context was not delivered:", err)
		return exitFailure
	}
	return exitOK
}

func chatHarness(agent string) bool {
	h := roles.Harness(agent)
	return h == "claude" || h == "pi"
}

// sendTicketContext types into an idle pane; a busy or waiting pane gets the
// context through the queue, delivered by the scraper when it is idle again.
func sendTicketContext(pane, text string) error {
	if tmux.ParsePaneStatus(tmux.GetPaneOptionValue(pane, tmux.PaneStatusOpt)) == tmux.StatusIdle {
		tmux.TypeLine(pane, text)
		return nil
	}
	if err := ensureBoardScraper(); err != nil {
		return err
	}
	_, _, err := queue.Push(pane, text)
	return err
}

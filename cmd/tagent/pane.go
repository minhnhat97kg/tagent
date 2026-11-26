package main

// `tagent pane add <name> --file <path|->` and `tagent pane ls`: custom web panes (internal/panes) for agents.
// An agent's pane is read-only in the web UI until the user allows actions there.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"tagent/internal/panes"
	"tagent/internal/tasks"
)

// paneAdd makes or changes the caller's pane; the user's own panes are never overwritten.
func paneAdd(name, html string) (string, error) {
	if err := panes.Put(name, html, tasks.CallerActor()); err != nil {
		return "", err
	}
	return fmt.Sprintf("Pane %s saved. It is read-only until the user allows actions; the web UI offers to add it to the layout.", name), nil
}

// paneList is one line per pane: its name, who made it and what it may do.
func paneList() string {
	var b strings.Builder
	for _, p := range panes.List() {
		kind := "user"
		if p.Agent {
			kind = "agent, read-only"
			if p.Actions {
				kind = "agent, actions allowed"
			}
		}
		fmt.Fprintf(&b, "%s\t%s\n", p.Name, kind)
	}
	return b.String()
}

func cmdPane(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "ls" {
		fmt.Fprint(stdout, paneList())
		return exitOK
	}
	if len(args) != 4 || args[0] != "add" || args[2] != "--file" {
		fmt.Fprintln(stderr, "usage: tagent pane add <name> --file <path|->\n       tagent pane ls")
		return exitUsage
	}
	html, err := readPaneFile(args[3])
	if err == nil {
		var msg string
		if msg, err = paneAdd(args[1], html); err == nil {
			fmt.Fprintln(stdout, msg)
			return exitOK
		}
	}
	fmt.Fprintln(stderr, "pane:", err)
	return exitFailure
}

func readPaneFile(path string) (string, error) {
	var r io.Reader = os.Stdin
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		r = f
	}
	b, err := io.ReadAll(io.LimitReader(r, panes.MaxHTML+1))
	if err != nil {
		return "", err
	}
	if len(b) > panes.MaxHTML {
		return "", errors.New("file over the size limit")
	}
	return string(b), nil
}

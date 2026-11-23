package main

import (
	"fmt"
	"io"
	"strings"
	"tagent/internal/sidebar"
	"tagent/internal/tmux"
)

func cmdHandoff(args []string, stderr io.Writer) int {
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: tagent handoff [PANE]")
		return exitUsage
	}
	pane := selfPane()
	if len(args) == 1 {
		pane = tmux.ResolvePane(args[0])
	}
	if !strings.HasPrefix(pane, "%") {
		fmt.Fprintln(stderr, "handoff: specify a pane or run inside tmux")
		return exitUsage
	}
	if err := sidebar.HandoffPane(pane); err != nil {
		fmt.Fprintln(stderr, err)
		return exitFailure
	}
	return exitOK
}

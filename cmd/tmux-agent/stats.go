package main

import (
	"fmt"
	"io"
	"os"

	"tmux-agent/internal/metrics"
	"tmux-agent/internal/tasks"
)

func cmdStats(args []string, stdout, stderr io.Writer) int {
	wd, _ := os.Getwd()
	scope := tasks.Scope(wd)
	if len(args) == 1 && args[0] == "clear" {
		if err := metrics.Clear(scope); err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(stderr, "stats:", err)
			return exitFailure
		}
		return exitOK
	}
	if len(args) != 0 && !(len(args) == 1 && args[0] == "ls") {
		fmt.Fprintln(stderr, "usage: tmux-agent stats [ls|clear]")
		return exitUsage
	}
	for _, s := range metrics.SessionList(metrics.Read(scope)) {
		tokens := "—"
		if s.HasUsage {
			tokens = fmt.Sprint(s.Usage.Total)
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", s.ID, s.Harness, s.Agent, s.Status, tokens)
	}
	return exitOK
}

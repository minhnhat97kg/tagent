package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"tmux-agent/internal/clipboard"
	"tmux-agent/internal/intake"
)

var executeIntakeCommand = intake.Execute

func cmdDo(args []string, in io.Reader, out, errs io.Writer) int {
	if len(args) == 0 {
		return exitUsage
	}
	// do is always an operation: the "!" prefix selects the command prompt.
	if !strings.HasPrefix(args[0], "!") {
		args = append([]string{"!" + args[0]}, args[1:]...)
	}
	return cmdTaskIntake(append([]string{"-i"}, args...), in, out, errs)
}
func escalateRequest(request, reason string, out io.Writer) int {
	fmt.Fprintln(out, "Ask the lead (Claude):", reason, "\nRequest:", request)
	fmt.Fprint(out, clipboard.OSC52Sequence(request))
	return exitOK
}
func approveCommand(command string, scan *bufio.Scanner, out, errs io.Writer, dir string) int {
	for {
		fmt.Fprintln(out, "Command:", command)
		fmt.Fprint(out, "y run / e edit / n abort: ")
		if !scan.Scan() {
			return exitOK
		}
		switch strings.TrimSpace(scan.Text()) {
		case "n":
			return exitOK
		case "e":
			fmt.Fprint(out, "Command: ")
			if !scan.Scan() {
				return exitOK
			}
			edited := strings.TrimSpace(scan.Text())
			if _, e := intake.CommandArgs(edited); e != nil {
				fmt.Fprintln(errs, e)
			} else {
				command = edited
			}
		case "y":
			if _, e := intake.CommandArgs(command); e != nil {
				fmt.Fprintln(errs, e)
				continue
			}
			output, e := executeIntakeCommand(command, dir)
			fmt.Fprint(out, output)
			if e != nil {
				fmt.Fprintln(errs, e)
				return exitFailure
			}
			return exitOK
		}
	}
}

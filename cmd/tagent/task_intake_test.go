package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"tagent/internal/tmux"
	"testing"
)

type intakeQueryRunner struct {
	output string
	err    error
}

func (r intakeQueryRunner) Run(args ...string) (string, error) {
	if len(args) > 0 && args[0] == "list-panes" {
		return r.output, r.err
	}
	return "", errors.New("unexpected tmux command")
}

func TestTaskIntakeAgentQueryAvailability(t *testing.T) {
	old := intakeCommand
	defer func() { intakeCommand = old }()
	// Echo the actual prompt's agent availability rather than mocking Context.
	intakeCommand = func() string { return `awk '/^Agents:$/ {getline; print "ANSWER: " $0}'` }
	fields := make([]string, 33)
	fields[0], fields[1], fields[3], fields[13], fields[15] = "shells", "@1", "shell", "zsh", "%1"
	for _, tc := range []struct {
		name, output, want string
		err                error
	}{
		{name: "healthy shells only", output: strings.Join(fields, "|"), want: "none"},
		{name: "successful empty query", want: "none"},
		{name: "failed query", want: "unavailable", err: errors.New("no server running")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer tmux.SetRunner(intakeQueryRunner{output: tc.output, err: tc.err})()
			var out, errs bytes.Buffer
			code := cmdTaskIntake([]string{"-i", "which agents are running?"}, strings.NewReader(""), &out, &errs)
			if code != exitOK || strings.TrimSpace(out.String()) != tc.want {
				t.Fatal(code, out.String(), errs.String())
			}
		})
	}
}

func TestTaskIntakeAnswer(t *testing.T) {
	old := intakeCommand
	defer func() { intakeCommand = old }()
	intakeCommand = func() string { return "printf 'ANSWER: Current snapshot only\\nCMD: never execute'" }
	var out, errs bytes.Buffer
	code := cmdTaskIntake([]string{"-i", "status?"}, strings.NewReader("y\n"), &out, &errs)
	if code != exitOK || !strings.Contains(out.String(), "Current snapshot only") || strings.Contains(out.String(), "create /") {
		t.Fatal(code, out.String(), errs.String())
	}
}

func TestTaskIntakeFake(t *testing.T) {
	old := intakeCommand
	defer func() { intakeCommand = old }()
	script := filepath.Join(t.TempDir(), "fake.sh")
	e := os.WriteFile(script, []byte("#!/bin/sh\np=$(cat)\ncase \"$p\" in *'Answer: small'*) printf 'TICKET\\ntitle: Build intake\\ngoal: Clarify\\nacceptance:\\n- Works\\nout of scope:\\n- Other\\nworkflow: none';; *) echo 'Q: Scope?';; esac\n"), 0700)
	if e != nil {
		t.Fatal(e)
	}
	intakeCommand = func() string { return script }
	var out, errs bytes.Buffer
	code := cmdTaskIntake([]string{"-i", "idea"}, strings.NewReader("small\nn\n"), &out, &errs)
	if code != exitOK || !strings.Contains(out.String(), "Build intake") {
		t.Fatal(code, out.String(), errs.String())
	}
}

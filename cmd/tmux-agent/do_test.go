package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestDoLoop(t *testing.T) {
	old, execOld := intakeCommand, executeIntakeCommand
	defer func() { intakeCommand = old; executeIntakeCommand = execOld }()
	for _, tt := range []struct {
		reply, input, want string
		calls              int
	}{{"CMD: tmux-agent reload", "y\n", "tmux-agent reload", 1}, {"CMD: tmux-agent reload", "e\ntmux-agent list\ny\n", "tmux-agent list", 1}, {"CMD: tmux-agent reload", "n\n", "", 0}, {"CMD: tmux-agent install", "y\nn\n", "", 0}, {"CMD: tmux-agent reload; kill all", "n\n", "", 0}, {"ESCALATE: Needs design judgement", "", "", 0}} {
		t.Run(tt.reply+tt.input, func(t *testing.T) {
			intakeCommand = func() string { return "printf '%s' '" + tt.reply + "'" }
			calls := 0
			executeIntakeCommand = func(c, d string) (string, error) {
				calls++
				if c != tt.want {
					t.Fatal(c)
				}
				return "result\n", nil
			}
			var out, errs bytes.Buffer
			code := cmdDo([]string{"request"}, strings.NewReader(tt.input), &out, &errs)
			if code != exitOK || calls != tt.calls {
				t.Fatal(code, calls, out.String(), errs.String())
			}
			if strings.HasPrefix(tt.reply, "ESCALATE") && !strings.Contains(out.String(), "Claude") {
				t.Fatal(out.String())
			}
		})
	}
}

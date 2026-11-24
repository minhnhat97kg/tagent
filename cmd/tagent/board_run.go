package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"tagent/internal/board"
	"tagent/internal/tasks"
)

var failureLine = regexp.MustCompile(`(?i)(--- FAIL|FAIL|panic|error|[^ ]+:[0-9]+)`)

func ciSummary(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var failures, other []string
	for scanner.Scan() {
		s := scanner.Text()
		trim := strings.TrimSpace(s)
		if strings.HasPrefix(trim, "ok ") || strings.HasPrefix(trim, "ok\t") || strings.HasPrefix(trim, "? ") || strings.HasPrefix(trim, "?\t") {
			continue
		}
		if trim == "" {
			continue
		}
		if failureLine.MatchString(s) {
			failures = append(failures, tasks.CapText(s, 400))
			if len(failures) > 12 {
				failures = failures[len(failures)-12:]
			}
		} else {
			other = append(other, tasks.CapText(s, 400))
			if len(other) > 12 {
				other = other[len(other)-12:]
			}
		}
	}
	room := 12 - len(failures)
	if len(other) > room {
		other = other[len(other)-room:]
	}
	return strings.Join(append(failures, other...), "\n")
}

// baseFailing reruns the named Go tests at the ticket's base (merge-base with
// main) and returns the ones that still fail there.
func baseFailing(dir string, tests []string) []string {
	base, err := exec.Command("git", "-C", dir, "merge-base", "HEAD", "main").Output()
	if err != nil {
		return nil
	}
	tmp, err := os.MkdirTemp("", "tagent-base-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(tmp)
	wt := filepath.Join(tmp, "wt")
	if err := exec.Command("git", "-C", dir, "worktree", "add", "--detach", wt, strings.TrimSpace(string(base))).Run(); err != nil {
		return nil
	}
	defer exec.Command("git", "-C", dir, "worktree", "remove", "--force", wt).Run()
	pattern := "^(" + strings.Join(tests, "|") + ")$"
	// Same environment as scripts/check.sh, so a base failure is never just a different setup.
	test := exec.Command("go", "test", "-skip", "TestSidebarLayoutHook", "-run", pattern, "./...")
	test.Dir = wt
	test.Env = append(os.Environ(), "TMPDIR=/tmp")
	out, _ := test.CombinedOutput()
	return board.FailedTests(string(out))
}

func runStatus(a board.Action) error {
	if a.Skip {
		return tasks.FinishEntry(tasks.Scope(a.Dir), a.Event.Task, a.Pass, "CI skipped: a user comment asked to skip this run", a.Event.Seq, false, tasks.MutationOptions{Actor: "ci"})
	}
	cmd := exec.Command("sh", "-c", a.Run)
	cmd.Dir = a.ExecDir
	if cmd.Dir == "" {
		cmd.Dir = tasks.MainRoot(a.Dir)
	}
	var outputs map[string]string
	var output *os.File
	if a.Job != "" {
		var err error
		output, err = os.CreateTemp("", "tagent-output-*")
		if err != nil {
			return err
		}
		defer os.Remove(output.Name())
		defer output.Close()
		cmd.Env = append(os.Environ(), "TMUX_AGENT_TICKET="+a.Event.Task, "TMUX_AGENT_OUTPUT="+output.Name())
	}
	logDir, e := os.MkdirTemp("", "tagent-ci-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(logDir)
	logPath := filepath.Join(logDir, "ci.log")
	log, e := os.Create(logPath)
	if e != nil {
		return e
	}
	defer log.Close()
	cmd.Stdout = log
	cmd.Stderr = cmd.Stdout
	err := cmd.Run()
	code := 0
	if err != nil {
		code = -1
		if e, ok := err.(*exec.ExitError); ok {
			code = e.ExitCode()
		} else {
			log.WriteString(err.Error())
		}
	}
	if _, e = log.Seek(0, 0); e != nil {
		return e
	}
	raw, e := io.ReadAll(log)
	if e != nil {
		return e
	}
	to := a.Pass
	preBody := ""
	if code != 0 {
		to = a.Fail
		// Failures that also happen on the base are not the ticket's: move forward.
		if failing := board.FailedTests(string(raw)); len(failing) > 0 && board.PreExisting(failing, baseFailing(cmd.Dir, failing)) {
			to = a.Pass
			preBody = "pre-existing: " + strings.Join(failing, ", ") + "\n"
		}
	}
	body := fmt.Sprintf("%sCI exit code %d\n%s", preBody, code, ciSummary(bytes.NewReader(raw)))
	if output != nil {
		outputs = map[string]string{}
		data, e := io.ReadAll(io.LimitReader(output, 65536))
		if e != nil {
			return e
		}
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if ok && k != "" {
				outputs[k] = v
			}
		}
		keys := []string{}
		for k := range outputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			body += "\n" + k + "=" + outputs[k]
		}
	}
	return tasks.FinishEntry(tasks.Scope(a.Dir), a.Event.Task, to, body, a.Event.Seq, false, tasks.MutationOptions{Actor: "ci", Outputs: outputs, Attachments: []string{logPath}})
}

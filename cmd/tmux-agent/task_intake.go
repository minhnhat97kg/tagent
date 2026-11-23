package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"tmux-agent/internal/board"
	"tmux-agent/internal/config"
	"tmux-agent/internal/intake"
	"tmux-agent/internal/tasks"
	"tmux-agent/internal/tmux"
	"tmux-agent/internal/workflow"
)

var intakeCommand = func() string { s, _ := config.Find("@agent_intake_llm"); return config.Get(s) }

func cmdTaskIntake(args []string, in io.Reader, out, errs io.Writer) int {
	wd, _ := os.Getwd()
	scope := tasks.Scope(wd)
	opt := tasks.MutationOptions{Actor: tasks.CallerActor()}
	var names, ideaParts []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-i" {
			continue
		}
		if strings.HasPrefix(a, "--") {
			i++
			if i == len(args) {
				return exitUsage
			}
			v := args[i]
			switch a {
			case "--workflow":
				opt.Workflow = v
			case "--to":
				names = strings.Split(v, ",")
			case "--needs":
				opt.Needs = strings.Split(v, ",")
			case "--input":
				k, v, ok := strings.Cut(v, "=")
				if !ok {
					return exitUsage
				}
				if opt.Inputs == nil {
					opt.Inputs = map[string]string{}
				}
				opt.Inputs[k] = v
			default:
				return exitUsage
			}
		} else {
			ideaParts = append(ideaParts, a)
		}
	}
	scan := bufio.NewScanner(in)
	read := func(prompt string) (string, bool) {
		fmt.Fprint(out, prompt)
		if !scan.Scan() {
			return "", false
		}
		return strings.TrimSpace(scan.Text()), true
	}
	idea := strings.Join(ideaParts, " ")
	if idea == "" {
		var ok bool
		idea, ok = read("Idea: ")
		if !ok || idea == "" {
			return exitOK
		}
	}
	ws := workflow.List(tasks.MainRoot(wd))
	var allowed []string
	for _, w := range ws {
		allowed = append(allowed, w.Name)
	}
	if opt.Workflow != "" {
		if _, e := workflow.Find(tasks.MainRoot(wd), opt.Workflow); e != nil {
			fmt.Fprintln(errs, e)
			return exitFailure
		}
	}
	command := intakeCommand()
	var d intake.Draft
	raw := ""
	var turns []intake.Turn
	if command != "off" {
		for {
			start := time.Now()
			sessions := tmux.QuerySessions()
			context := intake.Snapshot(wd, scope, sessions, sessions == nil)
			allowed = nil
			for _, w := range workflow.List(tasks.MainRoot(wd)) {
				allowed = append(allowed, w.Name)
			}
			reply, e := intake.Run(command, intake.Prompt(idea, turns, context, len(turns) >= 3))
			fmt.Fprintf(errs, "Intake turn %d: %s\n", len(turns)+1, time.Since(start))
			if e != nil {
				fmt.Fprintln(errs, e, "; using plain add")
				raw = ""
				break
			}
			raw = reply
			response, e := intake.ParseResponse(reply)
			if e == nil && response.Answer != "" {
				fmt.Fprintln(out, response.Answer)
				return exitOK
			}
			if e == nil && response.Escalate != "" {
				return escalateRequest(idea, response.Escalate, out)
			}
			if e == nil && response.Command != "" {
				return approveCommand(response.Command, scan, out, errs, wd)
			}
			q, draft := response.Question, response.Draft
			if e != nil || (q != "" && len(turns) >= 3) {
				fmt.Fprintln(out, reply)
				fmt.Fprintln(errs, "Invalid reply; use e to edit")
				break
			}
			if q == "" {
				d = draft
				d.Normalize(allowed)
				break
			}
			a, ok := read("🤖 " + q + "\nyou: ")
			if !ok {
				return exitOK
			}
			turns = append(turns, intake.Turn{Question: q, Answer: a})
		}
	}
	if raw == "" {
		d = intake.Draft{Title: idea, Goal: idea, Acceptance: []string{"Complete the requested task"}, OutOfScope: []string{"Unspecified work"}}
	}
	if opt.Workflow != "" {
		d.Workflow = opt.Workflow
	}
	for {
		if d.Title != "" {
			fmt.Fprintln(out, d.Text())
			fmt.Fprintln(out, "Workflow:", d.Workflow, "(w <name> or w none to change)")
		}
		choice, ok := read("y create / r create+start / e edit / n abort: ")
		if !ok || choice == "n" {
			return exitOK
		}
		if strings.HasPrefix(choice, "w ") {
			name := strings.TrimSpace(choice[2:])
			if name == "none" {
				d.Workflow = ""
			} else {
				for _, n := range allowed {
					if n == name {
						d.Workflow = n
					}
				}
			}
			continue
		}
		if choice == "e" {
			text := raw
			if d.Title != "" {
				text = intake.Block(d)
			}
			cmd, path, e := intake.Editor(text)
			if e == nil {
				cmd.Stdin = os.Stdin
				cmd.Stdout = out
				cmd.Stderr = errs
				e = cmd.Run()
				if e == nil {
					b, re := os.ReadFile(path)
					e = re
					if e == nil {
						var edited intake.Draft
						var q string
						q, edited, e = intake.Parse(string(b))
						if e == nil && q != "" {
							e = fmt.Errorf("editor must contain a ticket")
						}
						if e == nil {
							d = edited
							d.Normalize(allowed)
							raw = string(b)
						}
					}
				}
				os.Remove(path)
			}
			if e != nil {
				fmt.Fprintln(errs, e)
			}
			continue
		}
		if choice != "y" && choice != "r" || d.Title == "" {
			continue
		}
		opt.Workflow = d.Workflow
		var w workflow.Workflow
		if opt.Workflow != "" {
			var e error
			w, e = workflow.Find(tasks.MainRoot(wd), opt.Workflow)
			if e != nil {
				fmt.Fprintln(errs, e)
				continue
			}
			if opt.Inputs == nil {
				opt.Inputs = map[string]string{}
			}
			var missing []string
			for k, v := range w.Inputs {
				if v.Required && opt.Inputs[k] == "" && v.Default == "" {
					missing = append(missing, k)
				}
			}
			if len(missing) == 1 {
				opt.Inputs[missing[0]] = d.Text()
			}
			for k, v := range w.Inputs {
				if v.Required && opt.Inputs[k] == "" && v.Default == "" {
					value, ok := read(fmt.Sprintf("Input %s (default %q): ", k, v.Default))
					if !ok {
						return exitOK
					}
					opt.Inputs[k] = value
				}
			}
			if _, e = w.Values(opt.Inputs); e != nil {
				fmt.Fprintln(errs, e)
				continue
			}
		}
		t, e := tasks.Add(scope, d.Text(), names, opt)
		if e != nil {
			fmt.Fprintln(errs, e)
			continue
		}
		fmt.Fprintln(out, t.ID)
		if choice == "r" && opt.Workflow != "" {
			e = tasks.Move(scope, t.ID, w.Jobs[0].Status, "")
			if e == nil {
				e = board.Register(wd)
			}
			if e == nil {
				e = ensureBoardScraper()
			}
			if e != nil {
				fmt.Fprintln(errs, e)
				return exitFailure
			}
		}
		return exitOK
	}
}

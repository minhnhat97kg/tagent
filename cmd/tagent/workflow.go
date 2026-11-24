package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"tagent/internal/board"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
	"time"
)

func cmdWorkflow(args []string, out, errs io.Writer) int {
	dir, _ := os.Getwd()
	dir = tasks.MainRoot(dir)
	scope := tasks.Scope(dir)
	fail := func(e error) int { fmt.Fprintln(errs, e); return exitFailure }
	if len(args) == 0 {
		return exitUsage
	}
	if args[0] == "list" {
		for _, w := range workflow.List(dir) {
			n := 0
			for _, t := range tasks.List(scope) {
				if t.Workflow == w.Name && t.Status != "todo" && t.Status != "done" && t.Status != "blocked" {
					n++
				}
			}
			s := "local"
			if w.Global {
				s = "global"
			}
			fmt.Fprintf(out, "%s\t%s\t%d jobs\t%d in progress\n", w.Name, s, len(w.Jobs), n)
		}
		return exitOK
	}
	if len(args) < 2 {
		return exitUsage
	}
	name := args[1]
	if args[0] == "new" {
		if filepath.Base(name) != name || name == "." || name == ".." {
			return exitUsage
		}
		path := filepath.Join(workflow.LocalDir(dir), name+".yml")
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return fail(e)
		}
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return fail(e)
		}
		_, e = f.WriteString(workflow.Template(name))
		ce := f.Close()
		if e != nil {
			return fail(e)
		}
		if ce != nil {
			return fail(ce)
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = out
		cmd.Stderr = errs
		if e = cmd.Run(); e != nil {
			return fail(e)
		}
		if cfg, e := tasks.LoadBoardConfig(dir); e == nil { // verify what was just written
			for _, problem := range tasks.CheckBoard(dir, cfg) {
				fmt.Fprintln(errs, "! "+problem)
			}
		}
		return exitOK
	}
	if args[0] != "run" {
		return exitUsage
	}
	given := map[string]string{}
	wait := false
	var title []string
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--wait":
			wait = true
		case "--input":
			i++
			if i == len(args) {
				return exitUsage
			}
			k, v, ok := strings.Cut(args[i], "=")
			if !ok || k == "" {
				return exitUsage
			}
			given[k] = v
		default:
			title = append(title, args[i])
		}
	}
	w, e := workflow.Find(dir, name)
	if e != nil {
		return fail(e)
	}
	values, e := w.Values(given)
	if e != nil {
		return fail(e)
	}
	text := strings.Join(title, " ")
	if text == "" {
		text = w.Name
		keys := []string{}
		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			text += " " + k + "=" + values[k]
		}
	}
	t, e := tasks.Add(scope, text, nil, tasks.MutationOptions{Workflow: w.Name, Inputs: values})
	if e != nil {
		return fail(e)
	}
	if e = tasks.Move(scope, t.ID, w.Jobs[0].Status, ""); e != nil {
		return fail(e)
	}
	if e = board.Register(dir); e != nil {
		return fail(e)
	}
	if e = ensureBoardScraper(); e != nil {
		return fail(e)
	}
	fmt.Fprintln(out, t.ID)
	if wait {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return waitWorkflow(ctx, scope, t.ID, errs)
	}
	return exitOK
}

func waitWorkflow(ctx context.Context, scope, id string, errs io.Writer) int {
	for {
		t, e := tasks.Get(scope, id)
		if e != nil {
			fmt.Fprintln(errs, e)
			return exitFailure
		}
		switch t.Status {
		case "done":
			return exitOK
		case "blocked":
			return exitFailure
		}
		select {
		case <-ctx.Done():
			return exitFailure
		case <-time.After(100 * time.Millisecond):
		}
	}
}

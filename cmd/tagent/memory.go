package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"tagent/internal/memgraph"
	"tagent/internal/tasks"
)

const memoryUsage = `usage: tagent memory <search|show|related|add-node|add-link|edit-node|edit-link|rm-node|rm-link|check> [args] [--scope repo|shared]
  search <words...> [--limit N]      (default scope: repo and shared)
  show <id>
  related <id> [--depth 1-3] [--limit N]
  add-node --label L --kind text|markdown|code|json|yaml|url|file|image [--content C | --ref PATH] [--lang L] [--tags a,b]
  add-link <from> <to> <label>
  edit-node <id> [--label L] [--content C] [--ref R] [--lang L] [--tags a,b]
  edit-link <id> --label L
  rm-node <id> | rm-link <id>        (user only)
  check`

// cmdMemory is the agent/user entry point to the memory graph. Reads print JSON.
func cmdMemory(args []string, stdout, stderr io.Writer) int {
	return runMemory(args, tasks.CallerActor(), stdout, stderr)
}

// runMemory runs one memory subcommand as actor.
func runMemory(args []string, actor string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, memoryUsage)
		return exitUsage
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("memory "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	scope := fs.String("scope", "", "repo or shared (search: both)")
	limit := fs.Int("limit", 0, "max results")
	depth := fs.Int("depth", 1, "link depth 1-3")
	label := fs.String("label", "", "label")
	kind := fs.String("kind", "", "node kind")
	lang := fs.String("lang", "", "code language")
	content := fs.String("content", "", "content")
	ref := fs.String("ref", "", "file path or image source")
	tags := fs.String("tags", "", "comma-separated tags")

	// Accept flags before or after positional arguments.
	var pos []string
	for {
		if err := fs.Parse(rest); err != nil {
			return exitUsage
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		rest = fs.Args()[1:]
	}

	if *scope != "" && *scope != memgraph.ScopeRepo && *scope != memgraph.ScopeShared {
		fmt.Fprintln(stderr, "memory: --scope must be repo or shared")
		return exitUsage
	}
	wd, _ := os.Getwd()
	open := func(s string) (*memgraph.Graph, error) {
		if s == "" {
			s = memgraph.ScopeRepo
		}
		return memgraph.Open(s, tasks.MainRoot(wd))
	}
	var set = map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	fail := func(err error) int {
		fmt.Fprintln(stderr, "memory:", err)
		return exitFailure
	}
	need := func(n int) bool {
		if len(pos) < n {
			fmt.Fprintln(stderr, memoryUsage)
			return false
		}
		return true
	}

	switch sub {
	case "search":
		if len(pos) == 0 {
			fmt.Fprintln(stderr, memoryUsage)
			return exitUsage
		}
		scopes := []string{memgraph.ScopeRepo, memgraph.ScopeShared}
		if *scope != "" {
			scopes = []string{*scope}
		}
		var hits []memgraph.Hit
		for _, s := range scopes {
			g, err := open(s)
			if err != nil {
				return fail(err)
			}
			h, err := g.Search(strings.Join(pos, " "), *limit)
			if err != nil {
				return fail(err)
			}
			hits = append(hits, h...)
		}
		if *limit > 0 && len(hits) > *limit {
			hits = hits[:*limit]
		}
		return memJSON(stdout, hits)
	case "show":
		if !need(1) {
			return exitUsage
		}
		g, err := open(*scope)
		if err != nil {
			return fail(err)
		}
		h, err := g.Get(pos[0])
		if err != nil {
			return fail(err)
		}
		return memJSON(stdout, h)
	case "related":
		if !need(1) {
			return exitUsage
		}
		g, err := open(*scope)
		if err != nil {
			return fail(err)
		}
		steps, err := g.Related(pos[0], *depth, *limit)
		if err != nil {
			return fail(err)
		}
		return memJSON(stdout, steps)
	case "add-node":
		g, err := open(*scope)
		if err != nil {
			return fail(err)
		}
		n := memgraph.Node{Label: *label, Kind: *kind, Lang: *lang, Content: *content, Ref: *ref, Tags: splitTags(*tags)}
		out, err := g.AddNode(actor, n)
		if err != nil {
			return fail(err)
		}
		return memJSON(stdout, out)
	case "add-link":
		if !need(3) {
			return exitUsage
		}
		g, err := open(*scope)
		if err != nil {
			return fail(err)
		}
		out, err := g.AddLink(actor, pos[0], pos[1], pos[2])
		if err != nil {
			return fail(err)
		}
		return memJSON(stdout, out)
	case "edit-node":
		if !need(1) {
			return exitUsage
		}
		g, err := open(*scope)
		if err != nil {
			return fail(err)
		}
		var p memgraph.NodePatch
		if set["label"] {
			p.Label = label
		}
		if set["content"] {
			p.Content = content
		}
		if set["ref"] {
			p.Ref = ref
		}
		if set["lang"] {
			p.Lang = lang
		}
		if set["tags"] {
			t := splitTags(*tags)
			p.Tags = &t
		}
		out, err := g.EditNode(actor, pos[0], p)
		if err != nil {
			return fail(err)
		}
		return memJSON(stdout, out)
	case "edit-link":
		if !need(1) {
			return exitUsage
		}
		g, err := open(*scope)
		if err != nil {
			return fail(err)
		}
		out, err := g.EditLink(actor, pos[0], *label)
		if err != nil {
			return fail(err)
		}
		return memJSON(stdout, out)
	case "rm-node", "rm-link":
		if !need(1) {
			return exitUsage
		}
		g, err := open(*scope)
		if err != nil {
			return fail(err)
		}
		if sub == "rm-node" {
			err = g.DeleteNode(actor, pos[0])
		} else {
			err = g.DeleteLink(actor, pos[0])
		}
		if err != nil {
			return fail(err)
		}
		fmt.Fprintln(stdout, "deleted", pos[0])
		return exitOK
	case "check":
		scopes := []string{memgraph.ScopeRepo, memgraph.ScopeShared}
		if *scope != "" {
			scopes = []string{*scope}
		}
		var reps []memgraph.Report
		for _, s := range scopes {
			g, err := open(s)
			if err != nil {
				return fail(err)
			}
			r, err := g.Check()
			if err != nil {
				return fail(err)
			}
			reps = append(reps, r)
		}
		return memJSON(stdout, reps)
	}
	fmt.Fprintln(stderr, memoryUsage)
	return exitUsage
}

func splitTags(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func memJSON(w io.Writer, v any) int {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return exitFailure
	}
	return exitOK
}

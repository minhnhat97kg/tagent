package intake

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

const CommandReference = `Factual questions -> ANSWER; ! operations -> CMD; new work -> Q/TICKET; design/review/judgement -> ESCALATE.
Reply ONLY ANSWER: <facts>, CMD: tagent <args> (one command), ESCALATE: <one-line reason>, Q: <question>, or TICKET.
` + AllowedCommandReference

// AllowedCommandReference documents commands without prescribing a response protocol.
const AllowedCommandReference = `Allowed commands (no other subcommands):
list; task ls; task show ID; task move ID STATUS [NOTE]; task set ID STATUS [NOTE]; task comment ID "TEXT"; task assign ID ROLE[,ROLE]; task due ID YYYY-MM-DD;
workflow list; workflow run NAME [--input key=value] ["TITLE"];
reload; queue [PANE] | queue clear PANE; ctx [ls | add "TEXT"] [--global]; stats; kill AGENT.
Use double quotes for multiword arguments. Never use shell syntax or metacharacters. Never invent ticket IDs. Choose workflow by description; do not guess required input values.
`

type Response struct {
	Question                  string
	Draft                     Draft
	Command, Escalate, Answer string
}

func ParseResponse(raw string) (Response, error) {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		ls := strings.Split(s, "\n")
		if len(ls) >= 3 && strings.TrimSpace(ls[len(ls)-1]) == "```" {
			s = strings.TrimSpace(strings.Join(ls[1:len(ls)-1], "\n"))
		}
	}
	k, v, ok := strings.Cut(s, ":")
	if ok {
		v = strings.TrimSpace(v)
		switch strings.ToUpper(k) {
		case "ANSWER":
			if v == "" {
				return Response{}, fmt.Errorf("empty answer")
			}
			return Response{Answer: v}, nil
		case "CMD":
			if _, e := CommandArgs(v); e != nil {
				return Response{}, e
			}
			return Response{Command: v}, nil
		case "ESCALATE":
			if v == "" || strings.ContainsAny(v, "\r\n") {
				return Response{}, fmt.Errorf("invalid escalation")
			}
			return Response{Escalate: v}, nil
		}
	}
	q, d, e := Parse(s)
	return Response{Question: q, Draft: d}, e
}
func CommandArgs(s string) ([]string, error) {
	bad := func() ([]string, error) {
		return nil, fmt.Errorf("command refused: use one allowed tagent command without shell syntax")
	}
	if strings.ContainsAny(s, ";|&`$()><\r\n\x00") {
		return bad()
	}
	var args []string
	var b strings.Builder
	quoted, started := false, false
	for _, r := range s {
		if r == '"' {
			quoted = !quoted
			started = true
			continue
		}
		if r == '\\' || r == '\'' || unicode.IsControl(r) && r != '\t' {
			return bad()
		}
		if unicode.IsSpace(r) && !quoted {
			if started {
				args = append(args, b.String())
				b.Reset()
				started = false
			}
			continue
		}
		b.WriteRune(r)
		started = true
	}
	if quoted {
		return bad()
	}
	if started {
		args = append(args, b.String())
	}
	if len(args) < 2 || (args[0] != "tagent" && args[0] != "tmux-agent") {
		return bad()
	}
	switch args[1] {
	case "list", "reload", "queue", "stats", "kill":
	case "ctx":
		var rest []string
		for _, a := range args[2:] {
			if a != "--global" {
				rest = append(rest, a)
			}
		}
		if len(rest) > 0 && rest[0] != "add" && rest[0] != "ls" {
			return bad()
		}
	case "task":
		if len(args) < 3 {
			return bad()
		}
		switch args[2] {
		case "ls", "show", "move", "set", "comment", "assign", "due":
		default:
			return bad()
		}
	case "workflow":
		if len(args) < 3 || (args[2] != "list" && args[2] != "run") {
			return bad()
		}
	default:
		return bad()
	}
	return args, nil
}
func Execute(command, dir string) (string, error) {
	args, e := CommandArgs(command)
	if e != nil {
		return "", e
	}
	bin, e := os.Executable()
	if e != nil {
		return "", e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args[1:]...)
	cmd.Dir = dir
	out, e := cmd.CombinedOutput()
	return string(out), e
}

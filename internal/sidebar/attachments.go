package sidebar

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"io"
	"os"
	"os/exec"
	"strings"
	"tagent/internal/tasks"
	"unicode"
	"unicode/utf8"
)

func openTaskAttachment(scope, dir, id, ref, pane string) tea.Cmd {
	f, a, err := tasks.OpenTicketAttachment(scope, dir, id, ref)
	if err != nil {
		return func() tea.Msg { return actionResult("", err) }
	}
	data, err := io.ReadAll(io.LimitReader(f, 10<<20))
	f.Close()
	if err == nil && (!utf8.Valid(data) || strings.ContainsRune(string(data), 0)) {
		err = fmt.Errorf("%s is binary; use web Download", a.Name)
	}
	if err != nil {
		return func() tea.Msg { return actionResult("", err) }
	}
	safe := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, string(data))
	tmp, err := os.CreateTemp("", "tagent-view-*")
	if err != nil {
		return func() tea.Msg { return actionResult("", err) }
	}
	_, err = tmp.WriteString(safe)
	tmp.Close()
	done := func() tea.Msg { os.Remove(tmp.Name()); return actionResult("Viewer closed", nil) }
	if err != nil {
		os.Remove(tmp.Name())
		return func() tea.Msg { return actionResult("", err) }
	}
	args := strings.Fields(os.Getenv("PAGER"))
	if len(args) == 0 {
		args = []string{"less", "-S"}
	}
	args = append(args, tmp.Name())
	if pane != "" {
		quoted := make([]string, len(args))
		for i, a := range args {
			quoted[i] = shellQuote(a)
		}
		return popup(pane, "env LESSSECURE=1 LESSOPEN= LESSCLOSE= "+strings.Join(quoted, " "), done)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), "LESSSECURE=1", "LESSOPEN=", "LESSCLOSE=")
	return tea.ExecProcess(cmd, func(e error) tea.Msg { os.Remove(tmp.Name()); return actionResult("Viewer closed", e) })
}

package sidebar

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
		return openWithSystem(scope, dir, id, ref, a.Name)
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

// systemOpenable lists the binary types handed to the OS viewer. Anything that can run code or active content
// (scripts, .app, .command, html, svg) is left out on purpose.
var systemOpenable = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true, ".pdf": true, ".mp4": true, ".mov": true, ".webm": true, ".mp3": true, ".wav": true}

// openWithSystem copies a binary attachment to a temp file (extension kept) and opens it with the OS viewer
// (open / xdg-open). It only works on the machine that runs tmux, not over SSH.
// ponytail: the temp copy is left for the OS to clean up, since the viewer reads it asynchronously.
func openWithSystem(scope, dir, id, ref, name string) tea.Cmd {
	return func() tea.Msg {
		ext := strings.ToLower(filepath.Ext(name))
		if !systemOpenable[ext] {
			return actionResult("", fmt.Errorf("%s is binary; use web Download", name))
		}
		f, _, err := tasks.OpenTicketAttachment(scope, dir, id, ref)
		if err != nil {
			return actionResult("", err)
		}
		defer f.Close()
		tmp, err := os.CreateTemp("", "tagent-open-*"+ext)
		if err != nil {
			return actionResult("", err)
		}
		_, err = io.Copy(tmp, io.LimitReader(f, 100<<20))
		tmp.Close()
		if err != nil {
			os.Remove(tmp.Name())
			return actionResult("", err)
		}
		opener := "xdg-open"
		if runtime.GOOS == "darwin" {
			opener = "open"
		}
		cmd := exec.Command(opener, tmp.Name())
		if err := cmd.Start(); err != nil {
			os.Remove(tmp.Name())
			return actionResult("", fmt.Errorf("cannot open %s: %v; use web Download", name, err))
		}
		go cmd.Wait()
		return actionResult("Opened "+name, nil)
	}
}

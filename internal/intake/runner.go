package intake

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func Run(command, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = os.TempDir()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("intake: %w: %s", err, stderr.String())
	}
	return string(out), nil
}
func Block(d Draft) string {
	w := d.Workflow
	if w == "" {
		w = "none"
	}
	return "TICKET\ntitle: " + d.Title + "\ngoal: " + d.Goal + "\nacceptance:\n- " + strings.Join(d.Acceptance, "\n- ") + "\nout of scope:\n- " + strings.Join(d.OutOfScope, "\n- ") + "\nworkflow: " + w
}
func Editor(text string) (*exec.Cmd, string, error) {
	f, e := os.CreateTemp("", "intake-*.txt")
	if e != nil {
		return nil, "", e
	}
	path := f.Name()
	_, e = f.WriteString(text)
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		os.Remove(path)
		return nil, "", e
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	return exec.Command("sh", "-c", editor+` "$1"`, "sh", path), path, nil
}

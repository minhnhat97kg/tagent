package web

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"tagent/internal/roles"
)

const (
	usageTTL     = time.Minute
	usageTimeout = 5 * time.Second
	usageMax     = 64 // bytes of stdout; a percentage is far shorter
)

var percentOutput = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?%?$`)

// harnessRow is one usage row sent to the browser. It carries no command or
// environment. Neither a value nor Error means the first run is not done yet.
type harnessRow struct {
	Harness string   `json:"harness"`
	Label   string   `json:"label"`
	Used    *float64 `json:"usedPercent"`
	Error   bool     `json:"error"`
}

type usageResult struct {
	used    *float64
	at      time.Time // zero until the first run ends
	running bool
}

// usageCache keeps the last result of each command and environment.
// ponytail: results of commands edited out of harnesses.yml stay until restart.
type usageCache struct {
	sync.Mutex
	results map[string]*usageResult
}

// harnessUsage lists the configured usage rows from the cache and refreshes old
// ones in the background, so a slow command never delays the caller. The
// commands come only from the global harnesses.yml.
func (s *server) harnessUsage() []harnessRow {
	rows := []harnessRow{}
	list, _ := roles.LoadHarnesses()
	c := &s.usage
	c.Lock()
	defer c.Unlock()
	for _, h := range list {
		for _, u := range h.Usage {
			env := h.EnvAssignments()
			key := u.Command + "\x00" + strings.Join(env, "\x00")
			r := c.results[key]
			if r == nil {
				if c.results == nil {
					c.results = map[string]*usageResult{}
				}
				r = &usageResult{}
				c.results[key] = r
			}
			if !r.running && time.Since(r.at) >= usageTTL {
				r.running = true
				go func(command string) {
					used := runUsage(command, env, usageTimeout)
					c.Lock()
					defer c.Unlock()
					r.used, r.at, r.running = used, time.Now(), false
				}(u.Command)
			}
			rows = append(rows, harnessRow{h.Name, u.Label, r.used, r.used == nil && !r.at.IsZero()})
		}
	}
	return rows
}

// cappedOutput keeps just over usageMax bytes and drops the rest.
type cappedOutput struct{ b bytes.Buffer }

func (c *cappedOutput) Write(p []byte) (int, error) {
	c.b.Write(p[:min(len(p), max(0, usageMax+1-c.b.Len()))])
	return len(p), nil
}

// runUsage runs command with sh and returns the percentage it prints; nil on a
// failure, a timeout or any other output.
func runUsage(command string, env []string, timeout time.Duration) *float64 {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir, _ = os.UserHomeDir() // never resolve a relative path inside a repository
	// A timeout kills the whole process group, not only the shell.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var out cappedOutput
	cmd.Stdout = &out
	if cmd.Run() != nil || out.b.Len() > usageMax {
		return nil
	}
	text := strings.TrimSpace(out.b.String())
	if !percentOutput.MatchString(text) {
		return nil
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(text, "%"), 64)
	if err != nil || v > 100 {
		return nil
	}
	return &v
}

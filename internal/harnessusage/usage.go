// Package harnessusage reads the percentage-used commands of harnesses.yml. The
// web Usage panel and the routing quota share one cache, so each command runs
// at most once per TTL.
package harnessusage

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
	ttl     = time.Minute
	timeout = 5 * time.Second
	maxOut  = 64 // bytes of stdout; a percentage is far shorter
)

var percentOutput = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?%?$`)

// Row is one usage row sent to the browser. It carries no command or
// environment. Neither a value nor Error means the first run is not done yet.
type Row struct {
	Harness string   `json:"harness"`
	Label   string   `json:"label"`
	Used    *float64 `json:"usedPercent"`
	Error   bool     `json:"error"`
}

// Reading is one counted window of a harness: its label and percentage used.
type Reading struct {
	Label string
	Used  float64
}

type result struct {
	used    *float64
	at      time.Time // zero until the first run ends
	running bool
}

// cache keeps the last result of each command and environment.
// ponytail: results of commands edited out of harnesses.yml stay until restart.
var cache struct {
	sync.Mutex
	results map[string]*result
}

// sample returns the cached value of u and starts a background run when it is
// stale, so a slow command never delays the caller. The caller holds cache's lock.
func sample(h roles.HarnessSpec, u roles.UsageSpec) *result {
	env := h.EnvAssignments()
	key := u.Command + "\x00" + strings.Join(env, "\x00")
	r := cache.results[key]
	if r == nil {
		if cache.results == nil {
			cache.results = map[string]*result{}
		}
		r = &result{}
		cache.results[key] = r
	}
	if !r.running && time.Since(r.at) >= ttl {
		r.running = true
		go func(command string) {
			used := run(command, env, timeout)
			cache.Lock()
			defer cache.Unlock()
			r.used, r.at, r.running = used, time.Now(), false
		}(u.Command)
	}
	return r
}

// Rows lists the configured usage rows from the cache. The commands come only
// from the global harnesses.yml.
func Rows() []Row {
	rows := []Row{}
	list, _ := roles.LoadHarnesses()
	cache.Lock()
	defer cache.Unlock()
	for _, h := range list {
		for _, u := range h.Usage {
			r := sample(h, u)
			rows = append(rows, Row{h.Name, u.Label, r.used, r.used == nil && !r.at.IsZero()})
		}
	}
	return rows
}

// Read returns the cached percentages of the named harness's usage commands,
// labeled as configured. ok is false when the harness has no usage commands.
// Commands that failed or have not run yet are left out: unknown quota.
func Read(harness string) (readings []Reading, ok bool) {
	list, _ := roles.LoadHarnesses()
	cache.Lock()
	defer cache.Unlock()
	for _, h := range list {
		if h.Name != harness || len(h.Usage) == 0 {
			continue
		}
		ok = true
		for _, u := range h.Usage {
			if r := sample(h, u); r.used != nil {
				readings = append(readings, Reading{u.Label, *r.used})
			}
		}
	}
	return readings, ok
}

// capped keeps just over maxOut bytes and drops the rest.
type capped struct{ b bytes.Buffer }

func (c *capped) Write(p []byte) (int, error) {
	c.b.Write(p[:min(len(p), max(0, maxOut+1-c.b.Len()))])
	return len(p), nil
}

// run runs command with sh and returns the percentage it prints; nil on a
// failure, a timeout or any other output.
func run(command string, env []string, timeout time.Duration) *float64 {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir, _ = os.UserHomeDir() // never resolve a relative path inside a repository
	// A timeout kills the whole process group, not only the shell.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var out capped
	cmd.Stdout = &out
	if cmd.Run() != nil || out.b.Len() > maxOut {
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

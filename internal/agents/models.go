package agents

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Models is what each harness reports as its own model list; the web and the TUI
// pick from it instead of typing. It asks the harness CLI (pi --list-models,
// opencode models), codex's cache file, and knows claude's aliases (it has no
// list command). A harness with no list returns nothing: its field stays free
// text. The result is cached; a stale cache is returned at once and refreshed in
// the background, so a slow harness never blocks a request.
func Models() map[string][]string {
	modelMu.Lock()
	defer modelMu.Unlock()
	if !modelBusy && time.Since(modelAt) > modelTTL {
		modelBusy = true
		go refreshModels()
	}
	return modelCache
}

const modelTTL = 10 * time.Minute

var (
	modelMu    sync.Mutex
	modelCache = map[string][]string{"claude": {"opus", "sonnet", "haiku"}}
	modelAt    time.Time
	modelBusy  bool
)

func refreshModels() {
	next := map[string][]string{"claude": {"opus", "sonnet", "haiku"}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for name, list := range map[string]func() []string{"pi": piModels, "opencode": opencodeModels, "codex": codexModels} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if m := list(); len(m) > 0 {
				mu.Lock()
				next[name] = m
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	modelMu.Lock()
	modelCache, modelAt, modelBusy = next, time.Now(), false
	modelMu.Unlock()
}

func run(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, name, args...).Output()
	return string(out)
}

// piModels: `pi --list-models` prints a table "provider model ..."; pi's --model takes provider/model.
func piModels() (ms []string) {
	for i, l := range strings.Split(run("pi", "--list-models"), "\n") {
		if f := strings.Fields(l); i > 0 && len(f) >= 2 && !strings.HasPrefix(f[0], "[") {
			ms = append(ms, f[0]+"/"+f[1])
		}
	}
	return
}

// opencodeModels: `opencode models` prints provider/model per line.
func opencodeModels() (ms []string) {
	for _, l := range strings.Split(run("opencode", "models"), "\n") {
		if l = strings.TrimSpace(l); strings.Contains(l, "/") {
			ms = append(ms, l)
		}
	}
	return
}

// codexModels reads the model slugs codex caches in ~/.codex/models_cache.json.
func codexModels() (ms []string) {
	home, _ := os.UserHomeDir()
	b, _ := os.ReadFile(filepath.Join(home, ".codex", "models_cache.json"))
	var c struct {
		Models []struct{ Slug, Visibility string }
	}
	_ = json.Unmarshal(b, &c)
	for _, m := range c.Models {
		if m.Slug != "" && m.Visibility != "hide" {
			ms = append(ms, m.Slug)
		}
	}
	return
}

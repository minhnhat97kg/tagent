package install

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"tagent/internal/setup"
)

// rtk (output-compressing CLI proxy) is installed once and wired into each
// harness with its own `rtk init`. Markers under ~/.config/tagent record what
// tagent added: rtk-bin (the binary) and rtk-<harness> (the wiring). Uninstall
// only reverts what has a marker, so pre-existing rtk setups are never touched.

const rtkInstallURL = "https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh"

// rtkOpenCodePlugin is written by `rtk init --opencode`. `rtk init --uninstall`
// for any other harness also deletes it, so claude's uninstall restores it.
const rtkOpenCodePlugin = ".config/opencode/plugins/rtk.ts"

// rtkWiring is the `rtk init` flags for a harness. check is a file (relative
// to $HOME) that shows the harness is already wired when it contains needle.
type rtkWiring struct {
	args   []string
	check  string
	needle string
}

var rtkHarness = map[string]rtkWiring{
	"claude":   {args: []string{"-g", "--auto-patch"}, check: ".claude/settings.json", needle: "rtk hook claude"},
	"codex":    {args: []string{"-g", "--codex"}, check: ".codex/AGENTS.md", needle: "RTK.md"},
	"pi":       {args: []string{"-g", "--agent", "pi", "--auto-patch"}, check: ".pi/agent/extensions/rtk.ts"},
	"opencode": {args: []string{"-g", "--opencode", "--auto-patch"}, check: rtkOpenCodePlugin},
	"copilot":  {args: []string{"-g", "--copilot", "--auto-patch"}, check: ".copilot/hooks/rtk-rewrite.json"},
}

// rtkSkip explains harnesses rtk does not wire.
var rtkSkip = map[string]string{
	"gemini": "rtk skipped: `rtk init --gemini` overwrites GEMINI.md, and uninstall deletes it",
	"amp":    "rtk unsupported by amp",
}

func (w rtkWiring) wired(home string) bool {
	b, err := os.ReadFile(filepath.Join(home, w.check))
	return err == nil && strings.Contains(string(b), w.needle)
}

// marker is a tagent-owned file under ~/.config/tagent.
func (e env) marker(name string) string { return e.path(".config/tagent/" + name) }

// exec runs a command (through e.run when set, for tests); a failure carries the output.
func (e env) exec(name string, args ...string) error {
	if e.run != nil {
		return e.run(name, args...)
	}
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (e env) warn(msg string) { fmt.Fprintf(e.out, "  warn   %s\n", msg) }

// ensureRTK returns the rtk binary to wire, installing it when none is found.
// "" means rtk is off (TAGENT_NO_RTK=1, dry run, or install failed).
func (e env) ensureRTK() string {
	if os.Getenv("TAGENT_NO_RTK") == "1" {
		return ""
	}
	if p, err := exec.LookPath("rtk"); err == nil {
		return p
	}
	dir := cmp.Or(os.Getenv("RTK_INSTALL_DIR"), e.path(".local/bin"))
	p := filepath.Join(dir, "rtk")
	if _, err := os.Stat(p); err == nil { // not on PATH, not ours: use it, record nothing
		return p
	}
	if e.dry {
		e.say("rtk", "would install to "+dir)
		return ""
	}
	e.say("rtk", "installing to "+dir)
	script := fmt.Sprintf("curl -fsSL %s | RTK_EDITION=%s RTK_INSTALL_DIR=%s sh", rtkInstallURL,
		setup.ShellQuote(cmp.Or(os.Getenv("RTK_EDITION"), "oss")), setup.ShellQuote(dir))
	if err := e.exec("sh", "-c", script); err != nil {
		e.warn("rtk install failed (offline?): " + err.Error())
		return ""
	}
	if _, err := os.Stat(p); err != nil {
		e.warn("rtk install did not produce " + p)
		return ""
	}
	if err := e.write(e.marker("rtk-bin"), p+"\n"); err != nil {
		e.warn("rtk marker: " + err.Error())
	}
	if !slices.Contains(strings.Split(os.Getenv("PATH"), ":"), dir) {
		e.warn(dir + " is not on your PATH; harness hooks call rtk by name — add it to PATH")
	}
	return p
}

// rtkBinary is the rtk uninstall should run: the one tagent installed, else the one on PATH.
func (e env) rtkBinary() string {
	if b, err := os.ReadFile(e.marker("rtk-bin")); err == nil {
		if p := strings.TrimSpace(string(b)); p != "" {
			return p
		}
	}
	p, _ := exec.LookPath("rtk")
	return p
}

// rtkStep wires (or unwires) rtk for one harness. Failures are warnings only.
func (e env) rtkStep(name string, uninstall bool) {
	if err := e.rtkWire(name, uninstall); err != nil {
		e.warn(fmt.Sprintf("rtk %s: %v", name, err))
	}
}

func (e env) rtkWire(name string, uninstall bool) error {
	if why, ok := rtkSkip[name]; ok {
		if !uninstall {
			e.say("rtk", why)
		}
		return nil
	}
	w, ok := rtkHarness[name]
	if !ok {
		return nil
	}
	if !uninstall {
		if w.wired(e.home) {
			e.say("rtk", "kept existing wiring for "+name)
			return nil
		}
		if e.dry {
			e.say("rtk", "would wire "+name)
			return nil
		}
		if e.rtk == "" {
			return nil
		}
		if err := e.exec(e.rtk, append([]string{"init"}, w.args...)...); err != nil {
			return fmt.Errorf("rtk init: %w", err)
		}
		e.say("rtk", "wired "+name)
		return e.write(e.marker("rtk-"+name), "")
	}

	if _, err := os.Stat(e.marker("rtk-" + name)); err != nil {
		return nil // not ours
	}
	if e.dry {
		e.say("rtk", "would unwire "+name)
		return nil
	}
	if name == "opencode" {
		// `rtk init --opencode --uninstall` would also strip the claude wiring.
		if err := e.remove(rtkOpenCodePlugin); err != nil {
			return err
		}
	} else {
		if e.rtk == "" {
			return fmt.Errorf("rtk binary not found; run `rtk init %s --uninstall` yourself", strings.Join(w.args, " "))
		}
		saved, _ := os.ReadFile(e.path(rtkOpenCodePlugin))
		args := slices.DeleteFunc(slices.Clone(w.args), func(a string) bool { return a == "--auto-patch" })
		if err := e.exec(e.rtk, append(append([]string{"init"}, args...), "--uninstall")...); err != nil {
			return fmt.Errorf("rtk uninstall: %w", err)
		}
		if saved != nil {
			if err := e.write(e.path(rtkOpenCodePlugin), string(saved)); err != nil {
				return err
			}
		}
	}
	e.say("rtk", "unwired "+name)
	return e.remove(".config/tagent/rtk-" + name)
}

// rtkCleanup deletes the rtk binary tagent installed once no harness it wired remains.
func (e env) rtkCleanup() error {
	left, _ := filepath.Glob(e.marker("rtk-*"))
	for _, m := range left {
		if filepath.Base(m) != "rtk-bin" {
			return nil
		}
	}
	b, err := os.ReadFile(e.marker("rtk-bin"))
	if err != nil {
		return nil
	}
	bin := strings.TrimSpace(string(b))
	if !e.dry {
		if err := os.Remove(bin); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	e.say("remove", bin)
	return e.remove(".config/tagent/rtk-bin")
}

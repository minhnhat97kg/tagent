package roles

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"tagent/internal/apppath"
)

// HarnessSpec is one entry of harnesses.yml: the listed name, the binary it
// runs (the name when empty), and the arguments and environment every launch of
// it gets. A leading ~/ in an env value is the home directory. Usage commands
// feed the web Usage panel. Efforts are the levels it accepts: nil keeps the
// built-in levels of its kind, empty means it takes no effort.
type HarnessSpec struct {
	Name    string            `yaml:"name"`
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
	Usage   []UsageSpec       `yaml:"usage"`
	Efforts []string          `yaml:"efforts"`
}

// UsageSpec is one usage row of a harness: a shell command that prints the
// percentage used.
type UsageSpec struct {
	Label   string `yaml:"label"`
	Command string `yaml:"command"`
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var effortName = regexp.MustCompile(`^[a-z0-9]+$`)

// kindEfforts are the levels a kind's command can pass on (see nativeEffortArgs); nil when it takes none.
func kindEfforts(kind string) []string {
	switch kind {
	case "pi":
		return []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}
	case "claude":
		return []string{"low", "medium", "high", "xhigh", "max"}
	case "codex":
		return []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}
	}
	return nil
}

// Efforts are the levels the harness name accepts: the only list validation, dispatch and the web use. nil: none.
func Efforts(name string) []string {
	if name == "" {
		return nil
	}
	list, _ := LoadHarnesses()
	for _, h := range list {
		if h.Name == name && h.Efforts != nil {
			return h.Efforts
		}
	}
	return kindEfforts(Harness(name))
}

// EffortMap is every pickable harness with its levels, never nil, for the web.
func EffortMap() map[string][]string {
	out := map[string][]string{}
	for _, name := range HarnessNames() {
		out[name] = append([]string{}, Efforts(name)...)
	}
	return out
}

// KnownEffort reports whether some pickable harness accepts level: a tier's
// effort is a fallback for candidates, so it only needs to be a real level.
func KnownEffort(level string) bool {
	for _, name := range HarnessNames() {
		if slices.Contains(Efforts(name), level) {
			return true
		}
	}
	return false
}

var builtinHarnesses = []string{"claude", "codex", "opencode", "gemini", "pi", "copilot", "amp"}

// LoadHarnesses reads ~/.config/tagent/harnesses.yml; a missing or empty file
// means the built-in list. On error it still returns the built-ins, so lists
// keep working while a launch reports the error. Global only: a repository
// must not be able to add launch flags.
func LoadHarnesses() ([]HarnessSpec, error) {
	def := make([]HarnessSpec, len(builtinHarnesses))
	for i, name := range builtinHarnesses {
		def[i] = HarnessSpec{Name: name}
	}
	path := filepath.Join(apppath.ConfigDir(), "harnesses.yml")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return def, nil
	}
	if err != nil {
		return def, err
	}
	var file struct {
		Harnesses []HarnessSpec `yaml:"harnesses"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return def, fmt.Errorf("%s: %w", path, err)
	}
	home, _ := os.UserHomeDir()
	for _, h := range file.Harnesses {
		if h.Name == "" {
			return def, fmt.Errorf("%s: every harness needs a name", path)
		}
		if len(h.Efforts) > 0 && kindEfforts(kindOf(cmp.Or(h.Command, h.Name))) == nil {
			return def, fmt.Errorf("%s: %s: efforts need a claude, codex or pi command", path, h.Name)
		}
		for _, level := range h.Efforts {
			if !effortName.MatchString(level) {
				return def, fmt.Errorf("%s: %s: bad effort %q", path, h.Name, level)
			}
		}
		for k, v := range h.Env {
			if !envName.MatchString(k) {
				return def, fmt.Errorf("%s: bad env name %q", path, k)
			}
			// ponytail: ~ is this machine's home, also for a remote or docker launch.
			if rest, ok := strings.CutPrefix(v, "~/"); ok {
				h.Env[k] = filepath.Join(home, rest)
			}
		}
	}
	if len(file.Harnesses) == 0 {
		return def, nil
	}
	return file.Harnesses, nil
}

// HarnessNames is the pickable list, in file order.
func HarnessNames() []string {
	list, _ := LoadHarnesses()
	names := make([]string, len(list))
	for i, h := range list {
		names[i] = h.Name
	}
	return names
}

// KnownHarness reports whether name is in the pickable list: the only list that
// routing, agent definitions and the web accept. Without harnesses.yml it is the built-ins.
func KnownHarness(name string) bool {
	return slices.Contains(HarnessNames(), name)
}

// ResolveHarness turns a picked harness (a configured name, or a command) into
// its entry; Command is always set, to name itself when nothing else applies.
func ResolveHarness(name string) (HarnessSpec, error) {
	list, err := LoadHarnesses()
	if err != nil {
		return HarnessSpec{}, err
	}
	for _, want := range []string{name, filepath.Base(name)} {
		for _, h := range list {
			if h.Name == want {
				h.Command = cmp.Or(h.Command, name)
				return h, nil
			}
		}
	}
	return HarnessSpec{Name: name, Command: name}, nil
}

// EnvAssignments are the entry's environment as sorted KEY=value words.
func (h HarnessSpec) EnvAssignments() []string {
	var out []string
	for _, k := range slices.Sorted(maps.Keys(h.Env)) {
		out = append(out, k+"="+h.Env[k])
	}
	return out
}

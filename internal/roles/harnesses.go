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
// it gets. A leading ~/ in an env value is the home directory.
type HarnessSpec struct {
	Name    string            `yaml:"name"`
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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

// KnownHarness reports whether name is a built-in or configured harness.
func KnownHarness(name string) bool {
	return slices.Contains(builtinHarnesses, name) || slices.Contains(HarnessNames(), name)
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

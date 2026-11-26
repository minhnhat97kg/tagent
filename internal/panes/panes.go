// Package panes is the store of custom panes: HTML files in <config dir>/ui, shown by the web UI in sandboxed
// iframes. The web server, the CLI and the MCP tools share it, so one set of rules covers every way in.
//
// A pane an agent made (or changed) carries a sidecar <name>.meta.json: its creator and, once the user allowed
// actions, the hash of the HTML they allowed. A pane without that file is the user's own.
package panes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"tagent/internal/settings"
)

// MaxHTML is the largest pane an agent or the UI may write.
const MaxHTML = 256 << 10

var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// ValidName is whether name can be a pane: letters, digits, - and _, so never a path.
func ValidName(name string) bool { return nameRE.MatchString(name) }

// Dir is the folder of pane files, next to config.conf. A relative path (a relative XDG_CONFIG_HOME would
// resolve against the working directory, maybe a repo) means no panes.
func Dir() string {
	d := filepath.Dir(settings.Path())
	if !filepath.IsAbs(d) {
		return ""
	}
	return filepath.Join(d, "ui")
}

// File is the HTML file of a valid pane name, or "" when there is no such pane.
func File(name string) string {
	dir := Dir()
	if dir == "" || !ValidName(name) {
		return ""
	}
	f := filepath.Join(dir, name+".html")
	if fi, err := os.Stat(f); err != nil || fi.IsDir() {
		return ""
	}
	return f
}

type meta struct {
	Agent   string `json:"agent"`             // who made the pane
	Granted string `json:"granted,omitempty"` // sha256 of the HTML the user allowed actions for
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func metaFile(name string) string { return filepath.Join(Dir(), name+".meta.json") }

// readMeta is the sidecar of a pane. An unreadable one counts as an agent's, so a damaged file never widens access.
func readMeta(name string) (m meta, ok bool) {
	b, err := os.ReadFile(metaFile(name))
	if err != nil {
		return meta{}, false
	}
	if json.Unmarshal(b, &m) != nil || m.Agent == "" {
		return meta{Agent: "unknown"}, true
	}
	return m, true
}

// Info describes a pane. Agent: an agent made it, so it is read-only unless Actions: the user allowed actions
// for exactly this HTML.
type Info struct {
	Name    string `json:"name"`
	Agent   bool   `json:"agent"`
	Actions bool   `json:"actions"`
}

// Get is the pane called name, if there is one.
func Get(name string) (Info, bool) {
	f := File(name)
	if f == "" {
		return Info{}, false
	}
	info := Info{Name: name}
	if m, ok := readMeta(name); ok {
		b, _ := os.ReadFile(f)
		info.Agent, info.Actions = true, m.Granted != "" && b != nil && m.Granted == sum(b)
	}
	return info, true
}

// List is every pane, sorted by name.
func List() []Info {
	out := []Info{}
	entries, _ := os.ReadDir(Dir()) // a missing folder lists nothing
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".html"); ok {
			if info, ok := Get(name); ok {
				out = append(out, info)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// write puts b at path through a temp file in the same folder, so a reader never sees half a file. With
// replace false it fails when the file exists.
func write(path string, b []byte, replace bool) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(b)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if replace {
		return os.Rename(tmp.Name(), path)
	}
	err = os.Link(tmp.Name(), path)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("pane %s exists", strings.TrimSuffix(filepath.Base(path), ".html"))
	}
	return err
}

func check(name, html string) error {
	if !ValidName(name) {
		return fmt.Errorf("bad pane name %q: letters, digits, - and _, up to 64, starting with a letter or digit", name)
	}
	if strings.TrimSpace(html) == "" {
		return errors.New("html is empty")
	}
	if len(html) > MaxHTML {
		return fmt.Errorf("html over %d KiB", MaxHTML>>10)
	}
	if Dir() == "" {
		return errors.New("no config folder")
	}
	return os.MkdirAll(Dir(), 0o755)
}

// Create makes a pane the user wrote: no sidecar, full access. It fails when the name is taken.
func Create(name, html string) error {
	if err := check(name, html); err != nil {
		return err
	}
	if _, err := os.Lstat(metaFile(name)); err == nil {
		os.Remove(metaFile(name)) // left over from a deleted pane
	}
	return write(filepath.Join(Dir(), name+".html"), []byte(html), false)
}

// Put makes or changes a pane for an agent. The agent may change only a pane it made; the sidecar (without any
// grant) is written first, so the HTML is never there without it.
func Put(name, html, agent string) error {
	if err := check(name, html); err != nil {
		return err
	}
	if agent == "" {
		agent = "agent"
	}
	file := filepath.Join(Dir(), name+".html")
	_, exists := Get(name)
	if m, ok := readMeta(name); exists && (!ok || m.Agent != agent) {
		return fmt.Errorf("pane %s was not created by %s; pick another name", name, agent)
	}
	mb, _ := json.Marshal(meta{Agent: agent})
	if err := write(metaFile(name), mb, true); err != nil {
		return err
	}
	if err := write(file, []byte(html), exists); err != nil {
		if !exists {
			os.Remove(metaFile(name))
		}
		return err
	}
	return nil
}

// Grant lets an agent's pane call the API beyond reading (allow) or takes that back. The grant names the HTML
// as it is now: any later change to the file voids it.
func Grant(name string, allow bool) error {
	f := File(name)
	m, ok := readMeta(name)
	if f == "" || !ok {
		return errors.New("no such pane made by an agent")
	}
	m.Granted = ""
	if allow {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		m.Granted = sum(b)
	}
	mb, _ := json.Marshal(m)
	return write(metaFile(name), mb, true)
}

// Delete removes a pane with its sidecar and its saved data.
func Delete(name string) error {
	f := File(name)
	if f == "" {
		return errors.New("no such pane")
	}
	base := strings.TrimSuffix(f, ".html")
	err := os.Remove(f)
	os.Remove(base + ".meta.json")
	os.Remove(base + ".json")
	return err
}

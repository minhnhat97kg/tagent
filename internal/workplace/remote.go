package workplace

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Remote runs a ticket's agent over ssh on Host, in Dir. Nothing is copied or
// tracked: the files stay on the host, and done only records where they are.
type Remote struct {
	Host string `yaml:"host"`
	Dir  string `yaml:"dir"`
}

// remoteHost is a plain host name or ssh alias; a leading "-" would read as an ssh option.
var remoteHost = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.@-]*$`)

// Validate rejects a host ssh could read as an option and a Dir that is not absolute.
func (r Remote) Validate() error {
	if !remoteHost.MatchString(r.Host) {
		return fmt.Errorf("remote host %q: use a plain host name or ssh alias", r.Host)
	}
	if !path.IsAbs(r.Dir) || strings.ContainsFunc(r.Dir, func(c rune) bool { return c < ' ' }) {
		return fmt.Errorf("remote dir %q: must be an absolute path", r.Dir)
	}
	return nil
}

// Create checks the id and returns the main root: the pane runs locally and the
// remote dir travels in the ssh command.
func (Remote) Create(root, id string) (string, error) {
	if err := checkID(id); err != nil {
		return "", err
	}
	return root, nil
}

// Output is where the ticket's files live on the host.
func (r Remote) Output(root, id string) string { return r.Host + ":" + r.Dir }

// Gate has nothing to merge.
func (Remote) Gate(root, id, title string, _ func() ([]string, error)) (string, error) {
	return "", nil
}

// Cleanup removes nothing on the host; it records where the files stay. The note
// does not start with "cleanup: kept", so the ticket's panes are still closed.
func (r Remote) Cleanup(root, id string) []string {
	return []string{"cleanup: remote files stay in " + r.Output(root, id)}
}

package workplace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Docker runs a ticket's agent in a container of Image, with the ticket's
// directory Base/<id> mounted at /work. Create makes the container, the pane runs
// `docker exec` into it, and done removes the container unless Keep. Image must
// already exist: nothing is built here. The docker CLI runs on this host.
type Docker struct {
	Image string `yaml:"image"`
	Base  string `yaml:"-"`
	Keep  bool   `yaml:"-"`
}

var (
	dockerImage = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/@-]*$`)
	dockerName  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

// Validate rejects an image docker could read as an option.
func (d Docker) Validate() error {
	if !dockerImage.MatchString(d.Image) {
		return fmt.Errorf("docker image %q: use an image name such as devbox:latest", d.Image)
	}
	return nil
}

// Container is the ticket's container name.
func (Docker) Container(id string) string { return "tagent-" + id }

// Create makes the ticket's directory and its container, then starts it. A
// re-dispatch finds the container and only starts it again.
func (d Docker) Create(root, id string) (string, error) {
	if err := checkID(id); err != nil {
		return "", err
	}
	if !dockerName.MatchString(id) {
		return "", fmt.Errorf("docker workplace: ticket ID %q must be letters, digits, _, . or -", id)
	}
	dir := d.Output(root, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := d.Container(id)
	if _, err := dockerRun("start", name); err != nil { // no container yet: create it
		if _, err := dockerRun("create", "--name", name, "-v", dir+":/work", "-w", "/work", d.Image, "tail", "-f", "/dev/null"); err != nil {
			return "", err
		}
		if _, err := dockerRun("start", name); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// Output is the ticket's directory, whether or not it exists.
func (d Docker) Output(root, id string) string { return filepath.Join(d.Base, id) }

// Gate has nothing to merge.
func (Docker) Gate(root, id, title string, _ func() ([]string, error)) (string, error) {
	return "", nil
}

// Cleanup removes the container and the ticket's directory unless Keep. A failed
// removal is kept and reported; the directory stays while its container exists.
func (d Docker) Cleanup(root, id string) []string {
	if checkID(id) != nil { // an empty id would remove Base itself
		return nil
	}
	name := d.Container(id)
	if d.Keep {
		return []string{"cleanup: kept container " + name}
	}
	if _, err := dockerRun("rm", "-f", name); err != nil {
		return []string{"cleanup: kept container " + name + " (" + err.Error() + ")"}
	}
	if err := os.RemoveAll(d.Output(root, id)); err != nil {
		return []string{"cleanup: kept folder " + d.Output(root, id) + " (" + err.Error() + ")"}
	}
	return nil
}

// dockerRun runs docker and returns its trimmed output, or an error carrying it.
func dockerRun(args ...string) (string, error) {
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

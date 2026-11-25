package workplace

import (
	"os"
	"path/filepath"
)

// Folder keeps each ticket in a plain directory Base/<id>: no git, no branch, no
// merge. Base/.repo records the repo root, so the board can map the folder back
// to its repo. Cleanup removes the directory unless Keep (`cleanup: keep`).
type Folder struct {
	Base string
	Keep bool
}

var _ Workplace = Folder{}

// Create makes the ticket's directory and returns its path. It keeps the files
// already in it, so a re-dispatch resumes in place.
func (f Folder) Create(root, id string) (string, error) {
	if err := checkID(id); err != nil {
		return "", err
	}
	if err := os.MkdirAll(f.Output(root, id), 0o700); err != nil {
		return "", err
	}
	if b, _ := os.ReadFile(filepath.Join(f.Base, ".repo")); string(b) != root {
		if err := os.WriteFile(filepath.Join(f.Base, ".repo"), []byte(root), 0o600); err != nil {
			return "", err
		}
	}
	return f.Output(root, id), nil
}

// Output is the ticket's directory, whether or not it exists.
func (f Folder) Output(root, id string) string { return filepath.Join(f.Base, id) }

// Gate has nothing to merge.
func (Folder) Gate(root, id, title string, _ func() ([]string, error)) (string, error) {
	return "", nil
}

// Cleanup removes the ticket's directory unless Keep. A failed removal is kept
// and reported.
func (f Folder) Cleanup(root, id string) []string {
	if f.Keep || checkID(id) != nil { // an empty id would remove Base itself
		return nil
	}
	path := f.Output(root, id)
	if err := os.RemoveAll(path); err != nil {
		return []string{"cleanup: kept folder " + path + " (" + err.Error() + ")"}
	}
	return nil
}

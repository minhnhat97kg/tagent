package workplace

import (
	"os"
	"path/filepath"
)

// Browser keeps a ticket's Chrome profile and captures under Base/<id>; the ticket
// itself runs in the main root. Only captures/ is attached on done; the profile
// (cookies) is removed with the directory unless Keep (`cleanup: keep`).
type Browser struct {
	Base string
	Keep bool
}

var _ Workplace = Browser{}

// Create makes the profile and captures directories and returns the main root,
// where the ticket runs. Existing files are kept, so a re-dispatch resumes.
func (b Browser) Create(root, id string) (string, error) {
	if err := checkID(id); err != nil {
		return "", err
	}
	for _, dir := range []string{b.Profile(id), b.Output(root, id)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
	}
	return root, nil
}

// Profile is the ticket's Chrome profile directory, whether or not it exists.
func (b Browser) Profile(id string) string { return filepath.Join(b.Base, id, "profile") }

// Output is the ticket's captures directory, whether or not it exists.
func (b Browser) Output(root, id string) string { return filepath.Join(b.Base, id, "captures") }

// Gate has nothing to merge.
func (Browser) Gate(root, id, title string, _ func() ([]string, error)) (string, error) {
	return "", nil
}

// Cleanup removes the ticket's directory (profile and captures) unless Keep.
func (b Browser) Cleanup(root, id string) []string { return Folder(b).Cleanup(root, id) }

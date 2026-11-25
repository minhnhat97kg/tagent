package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/workplace"
)

const browserFlow = "name: browse\nworkplace: browser\njobs:\n  look:\n    agent: planner\n    prompt: Look\n"

// browserTicket returns the ticket's captures and profile dirs, as dispatch makes them.
func browserTicket(t *testing.T, yml string) (root, scope string, task Task, captures, profile string) {
	t.Helper()
	root, scope, task, _ = newFolderTicket(t, yml)
	wp, err := TicketWorkplace(scope, task)
	if err != nil {
		t.Fatal(err)
	}
	return root, scope, task, wp.Output(root, task.ID), wp.(workplace.Browser).Profile(task.ID)
}

func TestBrowserDoneAttachesCapturesAndRemovesProfile(t *testing.T) {
	root, scope, task, captures, profile := browserTicket(t, browserFlow)
	os.WriteFile(filepath.Join(captures, "shot.png"), []byte("s"), 0o644)
	os.WriteFile(filepath.Join(profile, "Cookies"), []byte("c"), 0o644)
	cleanupDone(scope, task.ID, "")
	names := attached(t, scope, root, task.ID)
	if !names["shot.png"] {
		t.Fatalf("capture not attached: %v", names)
	}
	if names["Cookies"] {
		t.Fatalf("profile file attached: %v", names)
	}
	if _, err := os.Stat(filepath.Dir(captures)); !os.IsNotExist(err) {
		t.Fatalf("browser dir kept after done: %v", err)
	}
}

func TestBrowserKeepAttachesAndKeepsProfile(t *testing.T) {
	root, scope, task, captures, profile := browserTicket(t, strings.Replace(browserFlow, "workplace: browser\n", "workplace: browser\ncleanup: keep\n", 1))
	os.WriteFile(filepath.Join(captures, "shot.png"), []byte("s"), 0o644)
	cleanupDone(scope, task.ID, "")
	if !attached(t, scope, root, task.ID)["shot.png"] {
		t.Fatal("shot.png not attached")
	}
	if _, err := os.Stat(profile); err != nil {
		t.Fatalf("keep removed the profile: %v", err)
	}
}

// A running ticket's absolute capture ref resolves inside captures/.
func TestBrowserOutputResolvesWhileRunning(t *testing.T) {
	root, scope, task, captures, _ := browserTicket(t, browserFlow)
	abs := filepath.Join(captures, "shot.png")
	os.WriteFile(abs, []byte("s"), 0o644)
	task.Outputs = map[string]map[string]string{"look": {"abs": abs}}
	if err := write(scope, task); err != nil {
		t.Fatal(err)
	}
	list, err := AttachmentList(scope, root, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range list {
		if a.Source == "output" && a.Error == "" {
			return
		}
	}
	t.Fatalf("capture ref did not resolve: %+v", list)
}

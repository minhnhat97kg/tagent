package tasks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"tagent/internal/workplace"
)

// PurgeItem is what purge did, or would do, to one closed ticket.
type PurgeItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Removed bool   `json:"removed"`
	Warning string `json:"warning,omitempty"` // what removing the ticket discards or leaves behind
	Kept    string `json:"kept,omitempty"`    // why the ticket stays; "" when it goes
}

// Purge deletes closed tickets with their worktrees, branches and attachments.
// Dirty worktrees and unmerged branches go too: the Warning says so. A closed
// ticket that a live ticket still needs stays, since deleting it would unblock
// that ticket. Without yes nothing changes. Event history is kept, so ids are
// never reused.
func Purge(scope string, yes bool) ([]PurgeItem, error) {
	root, ok := scopeRoots.Load(scope)
	if !ok {
		return nil, errors.New("unknown repository scope")
	}
	r := root.(string)
	items := []PurgeItem{}
	err := withLock(scope, func() error {
		all := List(scope)
		cands := map[string]bool{}
		for _, t := range all {
			if t.Status == "closed" {
				cands[t.ID] = true
			}
		}
		keptBy := map[string]string{}
		for changed := true; changed; { // a kept ticket keeps what it needs too
			changed = false
			for _, d := range all {
				if cands[d.ID] || d.Status == "done" {
					continue
				}
				for _, n := range d.Needs {
					if cands[n] {
						delete(cands, n)
						keptBy[n] = "needed by #" + d.ID
						changed = true
					}
				}
			}
		}
		for _, t := range all {
			if t.Status != "closed" {
				continue
			}
			it := PurgeItem{ID: t.ID, Title: t.Title}
			if !cands[t.ID] {
				it.Kept = keptBy[t.ID]
				items = append(items, it)
				continue
			}
			wp := workplace.Workplace(workplace.Git{})
			if t.Workflow != "" {
				w, err := TicketWorkplace(scope, t)
				if err != nil { // never guess what to delete
					it.Kept = "workplace: " + err.Error()
					items = append(items, it)
					continue
				}
				wp = w
			}
			switch w := wp.(type) {
			case workplace.Git:
				if why := w.PruneBlocker(r, t.ID); why != "" {
					it.Warning = "discards " + why
				}
			case workplace.Folder, workplace.Browser, workplace.Docker:
				it.Warning = "discards the ticket's workplace files"
			case workplace.Remote:
				it.Warning = "remote files stay on the host"
			}
			if !yes {
				items = append(items, it)
				continue
			}
			panes := ticketPanes(scope, t.ID) // before the worktree goes: Scope() needs it
			killPanes(panes)
			err := discardWorkplace(wp, r, t.ID, &it)
			if err == nil {
				err = os.RemoveAll(filepath.Join(boardDir(scope), t.ID))
			}
			if err == nil {
				err = os.Remove(file(scope, t.ID))
			}
			if err != nil {
				it.Kept = "discard failed: " + err.Error()
			} else {
				it.Removed = true
			}
			items = append(items, it)
		}
		return nil
	})
	return items, err
}

// discardWorkplace removes what the ticket's workplace holds. A remote workplace
// keeps its files on the host and says so in the item's warning.
func discardWorkplace(wp workplace.Workplace, root, id string, it *PurgeItem) error {
	var notes []string
	switch w := wp.(type) {
	case workplace.Git:
		return w.Discard(root, id)
	case workplace.Folder:
		w.Keep = false
		notes = w.Cleanup(root, id)
	case workplace.Browser:
		w.Keep = false
		notes = w.Cleanup(root, id)
	case workplace.Docker:
		w.Keep = false
		notes = w.Cleanup(root, id)
	case workplace.Remote:
		it.Warning = strings.Join(w.Cleanup(root, id), "; ")
	}
	if len(notes) > 0 {
		return errors.New(strings.Join(notes, "; "))
	}
	return nil
}

package tasks

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// TicketAttachment is derived metadata; Ref is opaque and scoped to a ticket.
type TicketAttachment struct {
	Ref       string    `json:"ref"`
	Source    string    `json:"source"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	Author    string    `json:"author"`
	Time      time.Time `json:"time"`
	CommentID string    `json:"commentId,omitempty"`
	Error     string    `json:"error,omitempty"`
	path      string
	root      string
}

// confinedFile opens name under root. Symlinks above root are resolved once by
// resolveRoot; every component below root must be a non-symlink.
func confinedFile(root, name string) (*os.File, error) {
	if !filepath.IsLocal(name) || strings.Contains(name, "\\") {
		return nil, fmt.Errorf("invalid attachment path")
	}
	parts := strings.Split(filepath.Clean(name), "/")
	dfd, err := openDirs(root, parts[:len(parts)-1], false)
	if err != nil {
		return nil, err
	}
	defer unix.Close(dfd)
	fd, err := unix.Openat(dfd, parts[len(parts)-1], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("attachment must be regular")
	}
	return f, nil
}

func AttachmentList(scope, project, id string) ([]TicketAttachment, error) {
	if !filepath.IsLocal(id) || filepath.Base(id) != id {
		return nil, fmt.Errorf("invalid ticket")
	}
	t, err := Get(scope, id)
	if err != nil {
		return nil, err
	}
	out := []TicketAttachment{}
	add := func(a TicketAttachment, key string) {
		a.Ref = base64.RawURLEncoding.EncodeToString([]byte(key))
		switch strings.ToLower(filepath.Ext(a.Name)) {
		case ".md", ".markdown":
			a.Kind = "markdown"
		case ".diff", ".patch":
			a.Kind = "diff"
		case ".json":
			a.Kind = "json"
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
			a.Kind = "image"
		default:
			a.Kind = "file"
		}
		f, e := confinedFile(a.root, a.path)
		if e != nil {
			a.Error = e.Error()
		} else {
			st, _ := f.Stat()
			a.Size = st.Size()
			if a.Time.IsZero() {
				a.Time = st.ModTime()
			}
			f.Close()
		}
		out = append(out, a)
	}
	for _, c := range t.Comments {
		for _, a := range c.Attachments {
			p := ""
			if filepath.Base(a.File) == a.File && !strings.ContainsAny(a.File, "/\\") {
				p = filepath.Join(id, a.File)
			}
			add(TicketAttachment{Source: "comment", Name: a.Name, Author: c.Author, Time: c.Created, CommentID: c.ID, root: boardDir(scope), path: p}, "comment:"+c.ID+":"+a.File)
		}
	}
	for job, values := range t.Outputs {
		for key, value := range values {
			if value == "" {
				continue
			}
			path := value
			if filepath.IsAbs(path) {
				path, err = filepath.Rel(project, path)
				if err != nil {
					continue
				}
			}
			_, e := os.Stat(filepath.Join(project, path))
			if e != nil && !strings.ContainsAny(value, "/\\") {
				continue
			}
			add(TicketAttachment{Source: "output", Name: filepath.Base(value), Author: "unknown", root: project, path: path}, "output:"+job+":"+key)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out, nil
}

func OpenTicketAttachment(scope, project, id, ref string) (*os.File, TicketAttachment, error) {
	list, err := AttachmentList(scope, project, id)
	if err != nil {
		return nil, TicketAttachment{}, err
	}
	for _, a := range list {
		if a.Ref == ref {
			f, e := confinedFile(a.root, a.path)
			return f, a, e
		}
	}
	return nil, TicketAttachment{}, fmt.Errorf("attachment not found")
}

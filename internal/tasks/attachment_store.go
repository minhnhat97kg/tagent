package tasks

import (
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// resolveRoot resolves symlinks in the trusted root once. Missing tail
// components are kept verbatim so they can be created under the resolved base.
func resolveRoot(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rest := ""
	for cur := abs; ; {
		if r, e := filepath.EvalSymlinks(cur); e == nil {
			return filepath.Join(r, rest), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// openDirs opens root (after resolveRoot) and descends parts with O_NOFOLLOW
// at every step, so no symlink below the trusted root can redirect the walk.
func openDirs(root string, parts []string, mkdir bool) (int, error) {
	abs, err := resolveRoot(root)
	if err != nil {
		return -1, err
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	for _, p := range append(strings.Split(strings.TrimPrefix(abs, "/"), "/"), parts...) {
		if p == "" {
			continue
		}
		next, e := unix.Openat(fd, p, flags, 0)
		if e == unix.ENOENT && mkdir {
			if e = unix.Mkdirat(fd, p, 0700); e == nil || e == unix.EEXIST {
				next, e = unix.Openat(fd, p, flags, 0)
			}
		}
		unix.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
	}
	return fd, nil
}

func storeAttachment(scope, id, name string, src io.Reader) (err error) {
	if filepath.Base(id) != id || !filepath.IsLocal(id) || filepath.Base(name) != name || !filepath.IsLocal(name) || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid attachment path")
	}
	dfd, err := openDirs(boardDir(scope), []string{id}, true)
	if err != nil {
		return err
	}
	defer unix.Close(dfd)
	out, e := unix.Openat(dfd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(out), name)
	_, err = io.Copy(f, src)
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		unix.Unlinkat(dfd, name, 0)
	}
	return err
}

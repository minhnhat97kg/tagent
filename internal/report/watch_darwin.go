package report

import (
	"time"

	"golang.org/x/sys/unix"
)

const vnodeFlags = unix.NOTE_WRITE | unix.NOTE_EXTEND | unix.NOTE_ATTRIB | unix.NOTE_RENAME | unix.NOTE_DELETE

type watcher struct{ kq, dir, file int }

func newWatcher(dir string) (*watcher, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open(dir, unix.O_EVTONLY, 0)
	if err != nil {
		unix.Close(kq)
		return nil, err
	}
	w := &watcher{kq: kq, dir: fd, file: -1}
	if err := w.add(fd); err != nil {
		w.close()
		return nil, err
	}
	return w, nil
}

func (w *watcher) add(fd int) error {
	ch := []unix.Kevent_t{{Ident: uint64(fd), Filter: unix.EVFILT_VNODE, Flags: unix.EV_ADD | unix.EV_CLEAR, Fflags: vnodeFlags}}
	_, err := unix.Kevent(w.kq, ch, nil, nil)
	return err
}

// watchFile follows the current file at path; a renamed-in replacement is a
// new inode, so the old descriptor is dropped (closing removes its event).
func (w *watcher) watchFile(path string) {
	if w.file >= 0 {
		unix.Close(w.file)
		w.file = -1
	}
	if fd, err := unix.Open(path, unix.O_EVTONLY, 0); err == nil {
		if w.add(fd) == nil {
			w.file = fd
		} else {
			unix.Close(fd)
		}
	}
}

func (w *watcher) wait(d time.Duration) error {
	ts := unix.NsecToTimespec(int64(d))
	out := make([]unix.Kevent_t, 4)
	_, err := unix.Kevent(w.kq, nil, out, &ts)
	if err == unix.EINTR {
		return nil
	}
	return err
}

func (w *watcher) close() {
	if w.file >= 0 {
		unix.Close(w.file)
	}
	unix.Close(w.dir)
	unix.Close(w.kq)
}

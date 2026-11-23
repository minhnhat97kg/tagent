package report

import (
	"time"

	"golang.org/x/sys/unix"
)

// A directory watch also reports in-place writes to files inside it.
type watcher struct{ fd int }

func newWatcher(dir string) (*watcher, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return nil, err
	}
	if _, err := unix.InotifyAddWatch(fd, dir, unix.IN_CREATE|unix.IN_CLOSE_WRITE|unix.IN_MOVED_TO|unix.IN_MODIFY); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return &watcher{fd}, nil
}

func (w *watcher) watchFile(string) {}

func (w *watcher) wait(d time.Duration) error {
	ms := int((d + time.Millisecond - 1) / time.Millisecond)
	fds := []unix.PollFd{{Fd: int32(w.fd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, ms)
	if err == unix.EINTR {
		return nil
	}
	if err != nil || n == 0 {
		return err
	}
	buf := make([]byte, 4096)
	_, err = unix.Read(w.fd, buf)
	return err
}

func (w *watcher) close() { unix.Close(w.fd) }

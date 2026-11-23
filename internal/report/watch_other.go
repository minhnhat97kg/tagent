//go:build !darwin && !linux

package report

import (
	"errors"
	"time"
)

type watcher struct{}

func newWatcher(string) (*watcher, error) {
	return nil, errors.New("report: wait unsupported on this OS")
}
func (w *watcher) watchFile(string)         {}
func (w *watcher) wait(time.Duration) error { return nil }
func (w *watcher) close()                   {}

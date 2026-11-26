package schedule

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	lastCheck time.Time // only the scraper loop calls MaybeRun, so no lock
	checking  sync.Mutex
)

// MaybeRun is called every scraper tick. It checks the schedules at most once a
// minute, in the background: a spawn waits for its pane and must not stall the loop.
func MaybeRun(now time.Time) {
	if now.Sub(lastCheck) < time.Minute || !checking.TryLock() {
		return
	}
	lastCheck = now
	go func() {
		defer checking.Unlock()
		checkAll(now)
	}()
}

// checkAll covers the global file and the trusted repos' files.
func checkAll(now time.Time) {
	files := []struct{ file, root string }{{GlobalFile(), ""}}
	for _, root := range Trusted() {
		files = append(files, struct{ file, root string }{LocalFile(root), root})
	}
	for _, f := range files {
		if err := Check(f.file, f.root, now, Fires); err != nil {
			fmt.Fprintln(os.Stderr, "schedule:", err)
		}
	}
}

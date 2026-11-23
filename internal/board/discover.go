package board

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"tmux-agent/internal/tasks"
	"tmux-agent/internal/tmux"
)

// Discover runs one dispatcher per scope of every agent pane plus extra dirs. One worker per shared scope, independent of browser polling. A held external
// lock is retried on the next discovery tick, allowing ownership to transfer.
func Discover(ctx context.Context, extra func() []string, fire Fire) {
	var workers sync.WaitGroup
	defer workers.Wait()
	running := map[string]bool{}
	finished := make(chan string, 32)
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		dirs := Registered()
		if extra != nil {
			dirs = extra()
		}
		for _, s := range tmux.QuerySessions() {
			for _, w := range s.Windows {
				for _, p := range w.Panes {
					if p.Agent != "" && p.Path != "" {
						dirs = append(dirs, p.Path)
					}
				}
			}
		}
		for _, d := range dirs {
			root := tasks.MainRoot(d)
			scope := tasks.Scope(root)
			if running[scope] {
				continue
			}
			running[scope] = true
			workers.Add(1)
			go func() {
				defer workers.Done()
				err := Run(ctx, root, fire)
				if err != nil && !errors.Is(err, ErrLocked) {
					log.Printf("board %s: %v", root, err)
				}
				select {
				case finished <- scope:
				case <-ctx.Done():
				}
			}()
		}
		select {
		case <-ctx.Done():
			return
		case scope := <-finished:
			delete(running, scope)
			select {
			case <-tick.C:
			case <-ctx.Done():
				return
			}
		case <-tick.C:
		}
	}
}

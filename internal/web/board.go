package web

import (
	"context"

	"tmux-agent/internal/board"
)

func (s *server) discoverBoards(ctx context.Context, fire board.Fire) {
	board.Discover(ctx, func() []string {
		dirs := []string{s.dir}
		s.boardDirs.Range(func(k, v any) bool { dirs = append(dirs, k.(string)); return true })
		return dirs
	}, fire)
}

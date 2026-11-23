package board

import (
	"os"
	"path/filepath"
	"tagent/internal/sharedctx"
	"tagent/internal/tasks"
)

func Register(dir string) error {
	dir = tasks.MainRoot(dir)
	base := tasks.BoardDir(tasks.Scope(dir))
	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	return tasks.AtomicWrite(filepath.Join(base, "repo"), []byte(dir))
}

func Registered() []string {
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(sharedctx.Dir("global")), "*", "tasks", "repo"))
	var dirs []string
	for _, f := range files {
		if b, e := os.ReadFile(f); e == nil {
			if st, e := os.Stat(string(b)); e == nil && st.IsDir() {
				dirs = append(dirs, string(b))
			}
		}
	}
	return dirs
}

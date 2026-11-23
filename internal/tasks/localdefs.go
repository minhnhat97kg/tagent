package tasks

import (
	"maps"
	"slices"

	"tagent/internal/roles"
)

// LocalDefinitions are the repo's agent definition files plus the roles declared
// in .tasks/board.yaml (pi has no definition files; its agents live there).
func LocalDefinitions(dir, harness string) []roles.Definition {
	defs := roles.ListIn(dir, harness)
	cfg, err := LoadBoardConfig(dir)
	if err != nil {
		return defs
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Roles)) {
		if !slices.ContainsFunc(defs, func(d roles.Definition) bool { return d.Name == name }) {
			defs = append(defs, roles.Definition{Name: name, Path: BoardConfigPath(dir), Harness: cfg.Roles[name].Agent})
		}
	}
	return defs
}

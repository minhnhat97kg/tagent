package roles

import (
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Profile is a global default for a role name: the board.yaml entry wins.
type Profile struct {
	Harness string `toml:"harness"`
	Model   string `toml:"model"`
}

func ProfileFor(name string) Profile {
	var profiles map[string]Profile
	_, _ = toml.DecodeFile(filepath.Join(GlobalDir(), "profiles.toml"), &profiles)
	return profiles[name]
}

package roles

import (
	"github.com/BurntSushi/toml"
	"os"
	"path/filepath"
)

type Profile struct {
	Harness string `toml:"harness"`
	Model   string `toml:"model"`
}

func ProfileFor(name string) Profile {
	var profiles map[string]Profile
	_, _ = toml.DecodeFile(filepath.Join(GlobalDir(), "profiles.toml"), &profiles)
	return profiles[name]
}
func Universal(name string) (Definition, bool) {
	if filepath.Base(name) != name {
		return Definition{}, false
	}
	path := filepath.Join(GlobalDir(), name+".md")
	b, err := os.ReadFile(path)
	return Definition{Name: name, Path: path, Instructions: string(b), Harness: "global"}, err == nil
}

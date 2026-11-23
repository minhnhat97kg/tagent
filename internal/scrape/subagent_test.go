package scrape

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMetaFinished(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(body), 0o600)
		return p
	}
	if !metaFinished(write("a.json", `{"agentType":"x","finished":true}`)) {
		t.Error("finished subagent not detected")
	}
	if metaFinished(write("b.json", `{"agentType":"x"}`)) || metaFinished(write("c.json", `{"finished":false}`)) || metaFinished(filepath.Join(dir, "none.json")) {
		t.Error("running or unknown subagent flagged as finished")
	}
}

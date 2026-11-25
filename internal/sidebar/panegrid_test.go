package sidebar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The rules are pinned by testdata/pane-layout.json, shared with the web port.
func TestPaneLayoutFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pane-layout.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string    `json:"name"`
		MinH     int       `json:"minH"`
		Saved    []paneBox `json:"saved"`
		Defaults []paneBox `json:"defaults"`
		Moved    string    `json:"moved"`
		Want     []paneBox `json:"want"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := mergeBoxes(c.Saved, c.Defaults, c.MinH, c.Moved); !slices.Equal(got, c.Want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.Name, got, c.Want)
		}
	}
}

func TestLayoutSaveKeepsTheOtherWidthClass(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	wide := []paneBox{{ID: "agents", X: 0, Y: 0, W: 6, H: 4}}
	narrow := []paneBox{{ID: "tasks", X: 0, Y: 2, W: 12, H: 6}}
	if err := saveLayout(true, wide); err != nil {
		t.Fatal(err)
	}
	if err := saveLayout(false, narrow); err != nil {
		t.Fatal(err)
	}
	if f := loadLayouts(); !slices.Equal(f.Wide, wide) || !slices.Equal(f.Narrow, narrow) {
		t.Fatalf("round trip: %+v", f)
	}
	if err := saveLayout(true, nil); err != nil { // reset deletes only its class
		t.Fatal(err)
	}
	if f := loadLayouts(); f.Wide != nil || !slices.Equal(f.Narrow, narrow) {
		t.Fatalf("after reset: %+v", f)
	}
}

func TestBadLayoutFileMeansNoLayout(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(layoutPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"{not json", `{"v":2,"wide":[{"id":"agents","w":6,"h":4}]}`} {
		if err := os.WriteFile(layoutPath(), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if f := loadLayouts(); f.Wide != nil || f.Narrow != nil {
			t.Fatalf("%q loaded as %+v", body, f)
		}
	}
}

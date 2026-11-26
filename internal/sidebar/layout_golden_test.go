package sidebar

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

var meterRe = regexp.MustCompile("[▰▱]{5} (?:[0-9]+%|–) *")

var brailleRe = regexp.MustCompile("[\u2800-\u28ff]")

// TestLayoutGolden pins the dashboard and its maximized views at the sizes that
// matter, for every focused card. UPDATE_GOLDEN=1 rewrites testdata/layout.golden.
func TestLayoutGolden(t *testing.T) {
	var b strings.Builder
	for _, s := range [][2]int{{32, 22}, {60, 40}, {60, 55}, {130, 45}} {
		for _, focus := range []int{0, 3, 2, 4} {
			m := dashSample()
			m.width, m.height, m.focus = s[0], s[1], focus
			fmt.Fprintf(&b, "=== dash %dx%d focus %d\n%s\n", s[0], s[1], focus, ansi.Strip(m.dashboardView()))
		}
		for _, view := range []string{"tasks", "workflows", "detail", "flow"} { // maximized cards and their sub-views
			m := boardTestModel()
			m.width, m.height = s[0], s[1]
			switch view {
			case "workflows":
				m.focus = cardWorkflows
			case "detail":
				m.boardDrawer = true
			case "flow":
				m.boardFlow = true
			}
			fmt.Fprintf(&b, "=== max %s %dx%d\n%s\n", view, s[0], s[1], ansi.Strip(m.dashboardView()))
		}
	}
	out := meterRe.ReplaceAllString(brailleRe.ReplaceAllString(b.String(), "~"), "~ ") // the companion avatar animates, quota meters are live
	path := filepath.Join("testdata", "layout.golden")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(path, []byte(out), 0o644)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != out {
		got := filepath.Join(os.TempDir(), "layout.got")
		os.WriteFile(got, []byte(out), 0o644)
		t.Fatalf("layout differs from %s: diff it against %s", path, got)
	}
}

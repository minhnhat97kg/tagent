package process

import "testing"

func TestParseCPUTimes(t *testing.T) {
	got := parseCPUTimes("  1 649:54.72\n 354   2:02.90\n 9 1:02:03\n bad x\n")
	if got[354] != 122.9 || got[9] != 3723 || got[1] < 38994 || len(got) != 3 {
		t.Fatalf("parseCPUTimes: %v", got)
	}
}

func TestHasToolChild(t *testing.T) {
	snap := FromPSOutput("10 1 claude claude\n11 10 node mcp-server\n12 10 zsh zsh -c make\n13 12 make make\n")
	if !snap.HasToolChild([]int{10}) {
		t.Fatal("a shell running make is a tool")
	}
	idle := FromPSOutput("10 1 claude claude\n11 10 node mcp-server\n12 10 zsh zsh\n")
	if idle.HasToolChild([]int{10}) {
		t.Fatal("an idle shell or MCP server is not a tool")
	}
}

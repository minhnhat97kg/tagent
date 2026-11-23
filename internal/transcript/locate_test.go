package transcript

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionDiscoveryBudget(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < discoveryEntries+1; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("%05d", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if path, exhausted := Locate(ctx, root, "missing.jsonl"); path != "" || !exhausted {
		t.Fatal("discovery entry budget", path, exhausted)
	}
	cancel()
	if _, exhausted := Locate(ctx, root, "also-missing.jsonl"); !exhausted {
		t.Fatal("discovery deadline")
	}
}

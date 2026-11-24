package sidebar

import (
	"os"
	"testing"

	"tagent/internal/quota"
)

// Tests never read the machine's quota cache or credentials.
func TestMain(m *testing.M) {
	quotaGet = func() quota.Snapshot { return quota.Snapshot{} }
	os.Exit(m.Run())
}

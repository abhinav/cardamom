package store

import (
	"crypto/rand"
	"encoding/hex"
)

// DefaultIDPrefix is used when Store is opened without an explicit prefix.
// CLI callers should load the per-project config and pass its prefix via
// the WithIDPrefix option; this default exists for direct programmatic
// use (tests) and as a safety net.
const DefaultIDPrefix = "clu-"

// newID returns a random ID with the given prefix, e.g. "clu-a3f8".
// The 16-bit hex suffix gives ~65k IDs before collision probability
// becomes notable; the create path retries on PK conflict.
func newID(prefix string) string {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b)
}

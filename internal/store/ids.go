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

// newID returns a random ID with the given prefix, e.g. "clu-a3f81b".
// The 24-bit hex suffix gives ~16.7M IDs before collision probability
// becomes notable; the create path retries on PK conflict. Widened from
// 16-bit (2-byte) so large `clu batch` graphs — a thousand+ issues in one
// shot, regenerated repeatedly — don't hit birthday collisions or exhaust
// the space as the table fills. Older 4-hex IDs (clu-a3f8) remain valid;
// IDs are opaque strings with no fixed-width assumption.
func newID(prefix string) string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b)
}

package store

import (
	"crypto/rand"
	"encoding/hex"
)

func newID() string {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "clu-" + hex.EncodeToString(b)
}

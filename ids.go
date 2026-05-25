package main

import (
	"crypto/rand"
	"encoding/hex"
)

func NewID() string {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "bd-" + hex.EncodeToString(b)
}

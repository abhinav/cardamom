package board

import (
	"encoding/hex"
	"fmt"
	"io"

	"go.abhg.dev/cardamom/internal/issue"
)

func newLogID(entropy io.Reader) (issue.LogID, error) {
	var body [16]byte
	if _, err := io.ReadFull(entropy, body[:]); err != nil {
		return "", fmt.Errorf("read log ID entropy: %w", err)
	}
	return issue.NewLogID("log_" + hex.EncodeToString(body[:]))
}

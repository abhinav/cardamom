package mail

import (
	"encoding/hex"
	"fmt"
	"io"

	"go.abhg.dev/cardamom/internal/mail"
)

func newMessageID(entropy io.Reader) (mail.ID, error) {
	var body [16]byte
	if _, err := io.ReadFull(entropy, body[:]); err != nil {
		return "", fmt.Errorf("read mail ID entropy: %w", err)
	}
	return mail.NewID("mail_" + hex.EncodeToString(body[:]))
}

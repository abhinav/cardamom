package attachment

import (
	"encoding/base32"
	"fmt"
	"io"
	"strings"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
)

func (r *Repository) newUploadID() (domainattachment.UploadID, error) {
	encoded, err := r.randomIdentity()
	if err != nil {
		return "", fmt.Errorf("generate attachment upload identity: %w", err)
	}
	return domainattachment.NewUploadID("upload_" + encoded)
}

func (r *Repository) newAttachmentID() (domainattachment.ID, error) {
	encoded, err := r.randomIdentity()
	if err != nil {
		return "", fmt.Errorf("generate attachment identity: %w", err)
	}
	return domainattachment.NewID("att_" + encoded)
}

func (r *Repository) randomIdentity() (string, error) {
	var body [16]byte
	if _, err := io.ReadFull(r.entropy, body[:]); err != nil {
		return "", err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(body[:])
	return strings.ToLower(encoded), nil
}

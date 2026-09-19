package boardhistory

import (
	"encoding/json/v2"
	"fmt"
)

// versionEnvelope is the common prefix used to select a retained wire format.
// Version dispatch must happen before a decoder applies a version's closed
// member set.
type versionEnvelope struct {
	Version uint8 `json:"version"`
}

func decodeVersion(encoded []byte, name string) (uint8, error) {
	var envelope versionEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return 0, fmt.Errorf("decode %s version: %w", name, err)
	}
	return envelope.Version, nil
}

package mail

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMessageIDUsesSuppliedEntropy(t *testing.T) {
	id, err := newMessageID(bytes.NewReader([]byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
	}))
	require.NoError(t, err)
	assert.Equal(t, "mail_000102030405060708090a0b0c0d0e0f", id.String())
}

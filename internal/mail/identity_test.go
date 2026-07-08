package mail

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewID(t *testing.T) {
	id, err := NewID("mail_0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	assert.Equal(t, "mail_0123456789abcdef0123456789abcdef", id.String())

	for _, value := range []string{
		"",
		"mail_0123456789abcdef0123456789abcde",
		"mail_0123456789abcdef0123456789abcdef0",
		"MAIL_0123456789abcdef0123456789abcdef",
		"mail_0123456789ABCDEF0123456789ABCDEF",
		"mail_0123456789abcdef0123456789abcdeg",
	} {
		_, err := NewID(value)
		assert.Error(t, err, value)
	}
}

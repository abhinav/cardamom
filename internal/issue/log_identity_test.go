package issue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogID(t *testing.T) {
	id, err := NewLogID("log_0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	assert.Equal(t, "log_0123456789abcdef0123456789abcdef", id.String())

	historical, err := NewLogID("cmt_0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	assert.Equal(t, "cmt_0123456789abcdef0123456789abcdef", historical.String())

	for _, value := range []string{
		"",
		"log_0123456789abcdef0123456789abcde",
		"log_0123456789abcdef0123456789abcdef0",
		"CMT_0123456789abcdef0123456789abcdef",
		"log_0123456789ABCDEF0123456789ABCDEF",
		"log_0123456789abcdef0123456789abcdeg",
	} {
		_, err := NewLogID(value)
		assert.Error(t, err, value)
	}
}

package issue

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplacementIDPreservesParticipantsAcrossCollisions(t *testing.T) {
	first, err := ParseUID(bytes.Repeat([]byte{1}, 16))
	require.NoError(t, err)
	second, err := ParseUID(bytes.Repeat([]byte{2}, 16))
	require.NoError(t, err)
	third, err := ParseUID(bytes.Repeat([]byte{3}, 16))
	require.NoError(t, err)
	retired := MustID("card-a")
	want := []ID{
		"card-a-01010101010101010101010101010101",
		"card-a-02020202020202020202020202020202",
		"card-a-03030303030303030303030303030303",
	}
	for index, uid := range []UID{first, second, third} {
		assert.Equal(t, want[index], ReplacementID(retired, uid))
	}
	// A later participant cannot change either earlier replacement, and a
	// collision at a replacement uses that retired name as its own input.
	assert.Equal(t, want[0], ReplacementID(retired, first))
	assert.Equal(t, ID(string(want[0])+"-02020202020202020202020202020202"),
		ReplacementID(want[0], second))
}

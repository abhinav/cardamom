package issue

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateUID(t *testing.T) {
	t.Parallel()

	want := bytes.Repeat([]byte{0x5a}, 16)
	uid, err := GenerateUID(bytes.NewReader(want))
	require.NoError(t, err)
	assert.Equal(t, want, uid.Bytes())

	returned := uid.Bytes()
	returned[0] = 0
	assert.Equal(t, want, uid.Bytes())
}

func TestGenerateUIDRejectsInvalidEntropy(t *testing.T) {
	t.Parallel()

	_, err := GenerateUID(bytes.NewReader(make([]byte, 15)))
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)

	_, err = GenerateUID(bytes.NewReader(make([]byte, 16)))
	assert.ErrorContains(t, err, "issue UID must not be zero")
}

func TestParseUID(t *testing.T) {
	t.Parallel()

	source := bytes.Repeat([]byte{0xa5}, 16)
	uid, err := ParseUID(source)
	require.NoError(t, err)
	source[0] = 0
	assert.Equal(t, bytes.Repeat([]byte{0xa5}, 16), uid.Bytes())

	_, err = ParseUID(make([]byte, 15))
	assert.ErrorContains(t, err, "issue UID must contain 16 bytes")

	_, err = ParseUID(make([]byte, 16))
	assert.ErrorContains(t, err, "issue UID must not be zero")
}

func TestStatePreservesUID(t *testing.T) {
	t.Parallel()

	uid := testUID(t, 0x2a)
	now := time.Unix(10, 0).UTC()
	state, err := Load(Snapshot{
		UID: uid, ID: MustID("an-1"), Title: "Private identity",
		Kind: KindTask, Lifecycle: LifecycleOpen, Priority: PriorityNormal,
		Created: now, Updated: now,
	})
	require.NoError(t, err)
	assert.Equal(t, uid, state.UID())
	assert.Equal(t, uid, state.Snapshot().UID)
	updated := state.WithRecoveryState(&RecoveryState{Body: "Still private"}, now.Add(time.Second))
	assert.Equal(t, uid, updated.UID())
	assert.Equal(t, uid, updated.Snapshot().UID)

	_, err = Load(Snapshot{
		ID: MustID("an-1"), Title: "Missing private identity",
		Kind: KindTask, Lifecycle: LifecycleOpen, Priority: PriorityNormal,
		Created: now, Updated: now,
	})
	assert.ErrorContains(t, err, "issue UID must not be zero")
}

func testUID(t *testing.T, fill byte) UID {
	t.Helper()
	uid, err := ParseUID(bytes.Repeat([]byte{fill}, 16))
	require.NoError(t, err)
	return uid
}

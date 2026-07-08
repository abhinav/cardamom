package attachment

import (
	"bytes"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestNewID(t *testing.T) {
	valid := "att_" + strings.Repeat("a", 26)

	id, err := NewID(valid)
	require.NoError(t, err)
	assert.Equal(t, valid, id.String())

	tests := []struct {
		name string
		give string
	}{
		{name: "Empty", give: ""},
		{name: "WrongPrefix", give: "attachment_" + strings.Repeat("a", 26)},
		{name: "Uppercase", give: "att_" + strings.Repeat("A", 26)},
		{name: "WrongLength", give: "att_" + strings.Repeat("a", 25)},
		{name: "InvalidAlphabet", give: "att_" + strings.Repeat("a", 25) + "0"},
		{name: "NonCanonicalPaddingBits", give: "att_" + strings.Repeat("a", 25) + "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewID(tt.give)
			assert.Error(t, err)
		})
	}
}

func TestNewIDFromEntropy(t *testing.T) {
	id, err := newID(bytes.NewReader(make([]byte, idEntropyBytes)))
	require.NoError(t, err)
	assert.Equal(t, "att_"+strings.Repeat("a", 26), id.String())

	_, err = newID(iotest.ErrReader(assert.AnError))
	assert.ErrorIs(t, err, assert.AnError)
}

func TestAssociation(t *testing.T) {
	boardID, err := board.NewID("board-1")
	require.NoError(t, err)
	issueID, err := issue.NewID("issue-1")
	require.NoError(t, err)

	boardAssociation, err := NewBoardAssociation(boardID)
	require.NoError(t, err)
	assert.Equal(t, boardID, boardAssociation.BoardID())
	_, associated := boardAssociation.OriginIssueID()
	assert.False(t, associated)

	issueAssociation, err := NewIssueAssociation(boardID, issueID)
	require.NoError(t, err)
	assert.Equal(t, boardID, issueAssociation.BoardID())
	gotIssueID, associated := issueAssociation.OriginIssueID()
	assert.True(t, associated)
	assert.Equal(t, issueID, gotIssueID)
}

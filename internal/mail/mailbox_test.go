package mail

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSelectionCarriesParsedMailboxInput(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	selection, err := NewSelection(SelectionRequest{
		Recipient: "bob", IncludeRead: true, MaxAge: time.Minute, Limit: 20,
	}, now)
	require.NoError(t, err)

	assert.Equal(t, "bob", selection.Recipient())
	assert.False(t, selection.AllRecipients())
	assert.True(t, selection.IncludeRead())
	assert.Equal(t, now.Add(-time.Minute), selection.Since())
	assert.Equal(t, 20, selection.Limit())
}

func TestNewSelectionRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name          string
		recipient     string
		allRecipients bool
		maxAge        time.Duration
		limit         int
		want          string
	}{
		{name: "MissingRecipient", want: "recipient required"},
		{name: "GlobalWithRecipient", recipient: "bob", allRecipients: true, want: "recipient must be empty"},
		{name: "NegativeMaxAge", recipient: "bob", maxAge: -time.Second, want: "maximum age must be greater than or equal to zero"},
		{name: "NegativeLimit", recipient: "bob", limit: -1, want: "limit must be greater than or equal to zero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSelection(SelectionRequest{
				Recipient: tt.recipient, AllRecipients: tt.allRecipients,
				MaxAge: tt.maxAge, Limit: tt.limit,
			}, time.Time{})
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestConsumptionRejectsAllRecipients(t *testing.T) {
	selection, err := NewSelection(SelectionRequest{AllRecipients: true}, time.Time{})
	require.NoError(t, err)

	_, err = Read(selection)
	assert.ErrorContains(t, err, "all-recipient mailbox reads cannot consume messages")
}

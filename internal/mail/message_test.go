package mail

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDirectSendCapsTTL(t *testing.T) {
	now := time.Date(2026, time.July, 13, 0, 0, 0, 0, time.UTC)
	send, err := NewDirectSend("alice", "bob", "status", 90*24*time.Hour)
	require.NoError(t, err)

	delivery := send.Delivery(now)
	assert.Equal(t, now.Add(MessageMaxTTL), delivery.Expires)
}

func TestNewDirectSendRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		sender    string
		recipient string
		body      string
		ttl       time.Duration
		want      string
	}{
		{name: "MissingSender", recipient: "bob", body: "status", want: "sender required"},
		{name: "MissingRecipient", sender: "alice", body: "status", want: "recipient required"},
		{name: "MissingBody", sender: "alice", recipient: "bob", want: "body required"},
		{name: "NegativeTTL", sender: "alice", recipient: "bob", body: "status", ttl: -time.Second, want: "ttl must be greater than or equal to zero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDirectSend(tt.sender, tt.recipient, tt.body, tt.ttl)
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

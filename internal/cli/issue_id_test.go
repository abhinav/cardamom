package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueID_UnmarshalText(t *testing.T) {
	tests := []struct {
		name    string
		give    string
		want    issueID
		wantErr string
	}{
		{name: "Bare", give: "cm-123", want: "cm-123"},
		{name: "Reference", give: "%cm-123", want: "cm-123"},
		{name: "Empty", wantErr: `issue ID "" must match`},
		{name: "MarkerOnly", give: "%", wantErr: `issue ID "" must match`},
		{name: "RepeatedMarker", give: "%%cm-123", wantErr: `issue ID "%cm-123" must match`},
		{name: "InvalidBareID", give: "cm_123", wantErr: `issue ID "cm_123" must match`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got issueID
			err := got.UnmarshalText([]byte(tt.give))

			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

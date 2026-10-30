package planning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestApplyRequiresResolvedOwnershipOnlyForSuppliedKey(t *testing.T) {
	for _, test := range []struct {
		name     string
		key      *string
		conflict bool
	}{
		{"disputed key", new("producer:shared"), true},
		{"unrelated key", new("producer:other"), false},
		{"no key", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := loadIssue(t, "an-1", issue.KindTask, issue.StatusReady)
			policy, err := LoadApply(ApplySnapshot{
				BoardID: "board", Revision: 1, Mode: ApplyModeCommit,
				IssueIDs:     []issue.ID{current.ID()},
				Issues:       map[issue.ID]ApplyIssueSnapshot{current.ID(): {State: current}},
				AllocatedIDs: []issue.ID{current.ID()}, AllocatedUIDs: []issue.UID{current.UID()},
				ExternalKeyConflicts: map[ExternalKey]string{"producer:shared": "conflict-key"},
				OccurredAt:           time.Unix(20, 0),
			})
			require.NoError(t, err)
			command, err := applyDocumentCommand(ApplyDocumentRequest{
				Version: 1, Mode: ApplyModeCommit, OnExisting: ApplyExistingUpdate,
				Issues: []ApplyIssue{{ID: new(current.ID().String()), Key: test.key, Details: new("More context")}},
			}, configuration.Defaults().Issue.Summary.MaxBytes)
			require.NoError(t, err)
			out, err := policy.ApplyDocument(command)
			if test.conflict {
				assert.Equal(t, errkind.Conflict, errkind.Of(err))
				assert.ErrorContains(t, err, "conflict-key")
				return
			}
			require.NoError(t, err)
			require.Len(t, out.Applied, 1)
			assert.Equal(t, "More context", out.Applied[0].Issue.Details())
		})
	}
}

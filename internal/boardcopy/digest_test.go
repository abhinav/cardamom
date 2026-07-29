package boardcopy

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/configuration"
)

func TestSnapshotDigestIsCanonicalAndUnambiguous(t *testing.T) {
	createdAt := time.Date(2026, time.July, 29, 9, 30, 0, 123, time.UTC)
	closedAt := createdAt.Add(time.Hour)
	summary := "Summary"
	author := "worker"
	digest, err := attachment.NewDigest(
		"sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
	)
	require.NoError(t, err)
	snapshot := CopySnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  12,
		Version:         CopySnapshotVersion,
		Digest:          "ignored",
		Board: CopyBoard{
			ID: "board-source", Name: "Source", CreatedAt: createdAt,
		},
		Configuration: configuration.Defaults(),
		Issues: []CopyIssue{
			{
				ID: "cm-2", Title: "Second", Kind: "task", Lifecycle: "closed",
				Priority: 3, CreatedAt: createdAt, UpdatedAt: closedAt,
				ClosedAt: &closedAt, Summary: &summary,
			},
			{
				ID: "cm-1", Title: "First", Kind: "workstream", Lifecycle: "open",
				Priority: 2, CreatedAt: createdAt, UpdatedAt: createdAt,
			},
		},
		Labels: []CopyLabel{
			{IssueID: "cm-2", Label: "beta"},
			{IssueID: "cm-1", Label: "alpha"},
		},
		Dependencies: []CopyDependency{{
			IssueID: "cm-2", PrerequisiteID: "cm-1",
		}},
		Containment: []CopyContainment{{
			ChildID: "cm-2", ParentID: "cm-1",
		}},
		ExternalKeys: []CopyExternalKey{{
			Key: "external-2", IssueID: "cm-2",
		}},
		LogEntries: []CopyLogEntry{
			{
				Order: 1, ID: "log_second", IssueID: "cm-2",
				Kind: "post", Author: &author, Committer: &author,
				Body: "Second Log", CreatedAt: &closedAt,
			},
			{
				Order: 0, ID: "log_first", IssueID: "cm-1",
				Kind: "state_snapshot", Body: "First Log",
			},
		},
		States: []CopyState{{
			IssueID: "cm-1", Body: "State", SnapshotLogEntryID: new("log_first"),
		}},
		Results: []CopyResultRecord{{
			IssueID: "cm-2", Body: "Result",
		}},
		Checkpoints: []CopyCheckpoint{{
			IssueID: "cm-2", Outcome: "approved", Reason: "Ready",
			DecidedAt: closedAt,
		}},
		Attachments: []CopyAttachment{{
			ID: "att_evidence", OriginIssueID: new("cm-2"),
			Blob:     attachment.BlobDescriptor{Digest: digest, SizeBytes: 4},
			Filename: "evidence.txt", MediaType: "text/plain",
			Lifecycle: "removed", CreatedActor: "worker", CreatedAt: createdAt,
			RemovedActor: &author, RemovedAt: &closedAt,
		}},
	}

	got := snapshotDigest(snapshot)
	assert.Equal(
		t,
		"sha256:133f6e53ab967b03fffcea1883a31b077656db0e9aa63e07f49dcfe06f91aeba",
		got,
	)

	reordered := snapshot
	reordered.Issues = slices.Clone(snapshot.Issues)
	reordered.Labels = slices.Clone(snapshot.Labels)
	reordered.LogEntries = slices.Clone(snapshot.LogEntries)
	slices.Reverse(reordered.Issues)
	slices.Reverse(reordered.Labels)
	slices.Reverse(reordered.LogEntries)
	assert.Equal(t, got, snapshotDigest(reordered))

	left := snapshot
	left.Board.ID = "ab"
	left.Board.Name = "c"
	right := snapshot
	right.Board.ID = "a"
	right.Board.Name = "bc"
	assert.NotEqual(t, snapshotDigest(left), snapshotDigest(right))

	empty := ""
	withEmpty := snapshot
	withEmpty.Board.Description = &empty
	assert.NotEqual(t, got, snapshotDigest(withEmpty))

	nextVersion := snapshot
	nextVersion.Version++
	assert.NotEqual(t, got, snapshotDigest(nextVersion))

	changedSourceMetadata := snapshot
	changedSourceMetadata.SourceLineageID = "store_ffffffffffffffffffffffffffffffff"
	changedSourceMetadata.SourceRevision++
	changedSourceMetadata.Digest = "another ignored value"
	assert.Equal(t, got, snapshotDigest(changedSourceMetadata))
}

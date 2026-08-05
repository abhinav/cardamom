package boardcopy

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"slices"
	"strconv"
	"time"
)

// PrepareSnapshot assigns the current version, canonical order, and semantic
// digest required for publication outside the source store.
func PrepareSnapshot(snapshot CopySnapshot) CopySnapshot {
	snapshot.Version = CopySnapshotVersion
	snapshot = canonicalCopySnapshot(snapshot)
	snapshot.Digest = snapshotDigest(snapshot)
	return snapshot
}

// VerifySnapshot checks the version and semantic digest of a prepared board
// publication.
func VerifySnapshot(snapshot CopySnapshot) error {
	if snapshot.Version != CopySnapshotVersion {
		return fmt.Errorf(
			"unsupported board snapshot version %d",
			snapshot.Version,
		)
	}
	if snapshot.Digest != snapshotDigest(snapshot) {
		return errors.New("board snapshot digest mismatch")
	}
	return nil
}

func canonicalCopySnapshot(snapshot CopySnapshot) CopySnapshot {
	snapshot.Issues = slices.Clone(snapshot.Issues)
	slices.SortFunc(snapshot.Issues, func(left, right CopyIssue) int {
		return cmp.Compare(left.ID, right.ID)
	})
	snapshot.Labels = slices.Clone(snapshot.Labels)
	slices.SortFunc(snapshot.Labels, func(left, right CopyLabel) int {
		if order := cmp.Compare(left.IssueID, right.IssueID); order != 0 {
			return order
		}
		return cmp.Compare(left.Label, right.Label)
	})
	snapshot.Dependencies = slices.Clone(snapshot.Dependencies)
	slices.SortFunc(snapshot.Dependencies, func(left, right CopyDependency) int {
		if order := cmp.Compare(left.IssueID, right.IssueID); order != 0 {
			return order
		}
		return cmp.Compare(left.PrerequisiteID, right.PrerequisiteID)
	})
	snapshot.Containment = slices.Clone(snapshot.Containment)
	slices.SortFunc(snapshot.Containment, func(left, right CopyContainment) int {
		if order := cmp.Compare(left.ChildID, right.ChildID); order != 0 {
			return order
		}
		return cmp.Compare(left.ParentID, right.ParentID)
	})
	snapshot.ExternalKeys = slices.Clone(snapshot.ExternalKeys)
	slices.SortFunc(snapshot.ExternalKeys, func(left, right CopyExternalKey) int {
		if order := cmp.Compare(left.Key, right.Key); order != 0 {
			return order
		}
		return cmp.Compare(left.IssueID, right.IssueID)
	})
	snapshot.LogEntries = slices.Clone(snapshot.LogEntries)
	slices.SortFunc(snapshot.LogEntries, func(left, right CopyLogEntry) int {
		if order := cmp.Compare(left.Order, right.Order); order != 0 {
			return order
		}
		return cmp.Compare(left.ID, right.ID)
	})
	snapshot.States = slices.Clone(snapshot.States)
	slices.SortFunc(snapshot.States, func(left, right CopyState) int {
		return cmp.Compare(left.IssueID, right.IssueID)
	})
	snapshot.Results = slices.Clone(snapshot.Results)
	slices.SortFunc(snapshot.Results, func(left, right CopyResultRecord) int {
		return cmp.Compare(left.IssueID, right.IssueID)
	})
	snapshot.Checkpoints = slices.Clone(snapshot.Checkpoints)
	slices.SortFunc(snapshot.Checkpoints, func(left, right CopyCheckpoint) int {
		return cmp.Compare(left.IssueID, right.IssueID)
	})
	snapshot.Attachments = slices.Clone(snapshot.Attachments)
	slices.SortFunc(snapshot.Attachments, func(left, right CopyAttachment) int {
		return cmp.Compare(left.ID, right.ID)
	})
	return snapshot
}

func snapshotDigest(snapshot CopySnapshot) string {
	snapshot = canonicalCopySnapshot(snapshot)
	encoder := newSnapshotDigestEncoder()

	// Version is the first value so a future encoding can define a new domain.
	encoder.integer("snapshot.version", snapshot.Version)
	encoder.string("board.id", snapshot.Board.ID)
	encoder.string("board.name", snapshot.Board.Name)
	encoder.optionalString("board.description", snapshot.Board.Description)
	encoder.timestamp("board.created_at", snapshot.Board.CreatedAt)
	encoder.string(
		"configuration.issue.id.prefix",
		snapshot.Configuration.Issue.ID.Prefix.String(),
	)
	encoder.string(
		"configuration.issue.id.strategy",
		snapshot.Configuration.Issue.ID.Strategy.String(),
	)
	encoder.unsigned(
		"configuration.issue.summary.max_bytes",
		snapshot.Configuration.Issue.Summary.MaxBytes.Uint64(),
	)
	encoder.unsigned(
		"configuration.attachment.max_bytes",
		snapshot.Configuration.Attachment.MaxBytes.Uint64(),
	)

	encoder.integer("issues.count", len(snapshot.Issues))
	for _, issue := range snapshot.Issues {
		encoder.marker("issues.item")
		encoder.string("issue.id", issue.ID)
		encoder.string("issue.title", issue.Title)
		encoder.string("issue.kind", issue.Kind)
		encoder.string("issue.lifecycle", issue.Lifecycle)
		encoder.signed("issue.priority", issue.Priority)
		encoder.timestamp("issue.created_at", issue.CreatedAt)
		encoder.timestamp("issue.updated_at", issue.UpdatedAt)
		encoder.optionalTimestamp("issue.closed_at", issue.ClosedAt)
		encoder.optionalString("issue.waiting_reason", issue.WaitingReason)
		encoder.optionalTimestamp("issue.waiting_since", issue.WaitingSince)
		encoder.optionalString("issue.summary", issue.Summary)
		encoder.optionalString("issue.details", issue.Details)
	}

	encoder.integer("labels.count", len(snapshot.Labels))
	for _, label := range snapshot.Labels {
		encoder.marker("labels.item")
		encoder.string("label.issue_id", label.IssueID)
		encoder.string("label.value", label.Label)
	}

	encoder.integer("dependencies.count", len(snapshot.Dependencies))
	for _, dependency := range snapshot.Dependencies {
		encoder.marker("dependencies.item")
		encoder.string("dependency.issue_id", dependency.IssueID)
		encoder.string("dependency.prerequisite_id", dependency.PrerequisiteID)
	}

	encoder.integer("containment.count", len(snapshot.Containment))
	for _, edge := range snapshot.Containment {
		encoder.marker("containment.item")
		encoder.string("containment.child_id", edge.ChildID)
		encoder.string("containment.parent_id", edge.ParentID)
	}

	encoder.integer("external_keys.count", len(snapshot.ExternalKeys))
	for _, key := range snapshot.ExternalKeys {
		encoder.marker("external_keys.item")
		encoder.string("external_key.value", key.Key)
		encoder.string("external_key.issue_id", key.IssueID)
	}

	encoder.integer("log_entries.count", len(snapshot.LogEntries))
	for _, entry := range snapshot.LogEntries {
		encoder.marker("log_entries.item")
		encoder.unsigned("log_entry.order", entry.Order)
		encoder.string("log_entry.id", entry.ID)
		encoder.string("log_entry.issue_id", entry.IssueID)
		encoder.string("log_entry.kind", entry.Kind)
		encoder.optionalString("log_entry.author", entry.Author)
		encoder.optionalString("log_entry.committer", entry.Committer)
		encoder.string("log_entry.body", entry.Body)
		encoder.optionalTimestamp("log_entry.created_at", entry.CreatedAt)
		encoder.optionalString("log_entry.next_action", entry.NextAction)
	}

	encoder.integer("states.count", len(snapshot.States))
	for _, state := range snapshot.States {
		encoder.marker("states.item")
		encoder.string("state.issue_id", state.IssueID)
		encoder.string("state.body", state.Body)
		encoder.optionalString("state.author", state.Author)
		encoder.optionalTimestamp("state.updated_at", state.UpdatedAt)
		encoder.optionalString(
			"state.snapshot_log_entry_id",
			state.SnapshotLogEntryID,
		)
		encoder.optionalString("state.next_action", state.NextAction)
	}

	encoder.integer("results.count", len(snapshot.Results))
	for _, result := range snapshot.Results {
		encoder.marker("results.item")
		encoder.string("result.issue_id", result.IssueID)
		encoder.string("result.body", result.Body)
	}

	encoder.integer("checkpoints.count", len(snapshot.Checkpoints))
	for _, checkpoint := range snapshot.Checkpoints {
		encoder.marker("checkpoints.item")
		encoder.string("checkpoint.issue_id", checkpoint.IssueID)
		encoder.string("checkpoint.outcome", checkpoint.Outcome)
		encoder.string("checkpoint.reason", checkpoint.Reason)
		encoder.timestamp("checkpoint.decided_at", checkpoint.DecidedAt)
	}

	encoder.integer("attachments.count", len(snapshot.Attachments))
	for _, value := range snapshot.Attachments {
		encoder.marker("attachments.item")
		encoder.string("attachment.id", value.ID)
		encoder.optionalString("attachment.origin_issue_id", value.OriginIssueID)
		encoder.string("attachment.blob.digest", value.Blob.Digest.String())
		encoder.unsigned("attachment.blob.size_bytes", value.Blob.SizeBytes)
		encoder.string("attachment.filename", value.Filename)
		encoder.string("attachment.media_type", value.MediaType)
		encoder.string("attachment.lifecycle", value.Lifecycle)
		encoder.string("attachment.created_actor", value.CreatedActor)
		encoder.timestamp("attachment.created_at", value.CreatedAt)
		encoder.optionalString("attachment.removed_actor", value.RemovedActor)
		encoder.optionalTimestamp("attachment.removed_at", value.RemovedAt)
	}

	return "sha256:" + hex.EncodeToString(encoder.sum())
}

type snapshotDigestEncoder struct {
	hash hash.Hash
}

func newSnapshotDigestEncoder() *snapshotDigestEncoder {
	return &snapshotDigestEncoder{hash: sha256.New()}
}

func (e *snapshotDigestEncoder) marker(tag string) {
	e.field(tag, nil)
}

func (e *snapshotDigestEncoder) string(tag string, value string) {
	e.field(tag, []byte(value))
}

func (e *snapshotDigestEncoder) optionalString(tag string, value *string) {
	if value == nil {
		e.field(tag, []byte{0})
		return
	}
	e.field(tag, append([]byte{1}, []byte(*value)...))
}

func (e *snapshotDigestEncoder) integer(tag string, value int) {
	e.field(tag, strconv.AppendInt(nil, int64(value), 10))
}

func (e *snapshotDigestEncoder) signed(tag string, value int64) {
	e.field(tag, strconv.AppendInt(nil, value, 10))
}

func (e *snapshotDigestEncoder) unsigned(tag string, value uint64) {
	e.field(tag, strconv.AppendUint(nil, value, 10))
}

func (e *snapshotDigestEncoder) timestamp(tag string, value time.Time) {
	e.field(tag, []byte(value.UTC().Format(time.RFC3339Nano)))
}

func (e *snapshotDigestEncoder) optionalTimestamp(tag string, value *time.Time) {
	if value == nil {
		e.field(tag, []byte{0})
		return
	}
	text := value.UTC().Format(time.RFC3339Nano)
	e.field(tag, append([]byte{1}, []byte(text)...))
}

func (e *snapshotDigestEncoder) field(tag string, value []byte) {
	// Tags and values are framed independently to prevent concatenation aliases.
	e.writePart([]byte(tag))
	e.writePart(value)
}

func (e *snapshotDigestEncoder) writePart(value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = e.hash.Write(size[:])
	_, _ = e.hash.Write(value)
}

func (e *snapshotDigestEncoder) sum() []byte {
	return e.hash.Sum(nil)
}

package boardcopy

import (
	"cmp"
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
)

const (
	// CopySnapshotVersion identifies the streamed semantic board-copy contract.
	CopySnapshotVersion = 2
)

// RecordType identifies one section of the canonical incremental board
// publication. The constant order is the canonical section order.
type RecordType uint8

const (
	// RecordTypeUnknown identifies a missing record.
	RecordTypeUnknown RecordType = iota

	// RecordTypeHeader identifies the required first record.
	RecordTypeHeader

	// RecordTypeIssue identifies an issue record.
	RecordTypeIssue

	// RecordTypeLabel identifies an issue-label record.
	RecordTypeLabel

	// RecordTypeDependency identifies a prerequisite edge record.
	RecordTypeDependency

	// RecordTypeContainment identifies a containment edge record.
	RecordTypeContainment

	// RecordTypeExternalKey identifies an external-key record.
	RecordTypeExternalKey

	// RecordTypeLogEntry identifies an immutable Log record.
	RecordTypeLogEntry

	// RecordTypeState identifies a mutable State record.
	RecordTypeState

	// RecordTypeResult identifies a durable Result record.
	RecordTypeResult

	// RecordTypeCheckpoint identifies a checkpoint-decision record.
	RecordTypeCheckpoint

	// RecordTypeAttachment identifies an attachment-metadata record.
	RecordTypeAttachment

	// RecordTypeTrailer identifies the required final record.
	RecordTypeTrailer
)

// Record is one bounded value in an incremental semantic board publication.
//
// Canonical streams contain one RecordHeader, records grouped by ascending
// RecordType and each payload's documented key order, then one RecordTrailer.
// Each record contributes its fields to the versioned semantic digest in the
// order implemented beside its payload definition.
type Record interface {
	recordType() RecordType
	compareRecordKey(Record) int
	addToRecordCounts(*RecordCounts) error
	encodeSemantic(*semanticEncoder)
}

// CopyBoard contains the portable board namespace fields.
type CopyBoard struct {
	// ID is the source board identity.
	ID string

	// Name is the source board display name.
	Name string

	// Description preserves absence separately from an empty description.
	Description *string

	// CreatedAt is the source board creation time.
	CreatedAt time.Time
}

// RecordHeader identifies the retained source view and board policy.
//
// The semantic digest includes Version, Board, and Configuration. It excludes
// SourceLineageID and SourceRevision so identical board semantics from another
// retained view have the same digest. The digest itself is stored outside the
// stream.
type RecordHeader struct {
	// Version identifies the semantic board-copy contract.
	Version int

	// SourceLineageID identifies the source store persistence history.
	SourceLineageID string

	// SourceRevision identifies the canonical revision retained by the reader.
	SourceRevision int64

	// Board contains the source board namespace and presentation.
	Board CopyBoard

	// Configuration contains the effective source board policy.
	Configuration configuration.Configuration
}

func (RecordHeader) recordType() RecordType { return RecordTypeHeader }

func (RecordHeader) compareRecordKey(Record) int { return 0 }

func (RecordHeader) addToRecordCounts(*RecordCounts) error { return nil }

func (r RecordHeader) encodeSemantic(encoder *semanticEncoder) {
	// Version begins the digest so future versions define a distinct domain.
	encoder.integer("snapshot.version", r.Version)
	encoder.string("board.id", r.Board.ID)
	encoder.string("board.name", r.Board.Name)
	encoder.optionalString("board.description", r.Board.Description)
	encoder.timestamp("board.created_at", r.Board.CreatedAt)
	encoder.string(
		"configuration.issue.id.prefix",
		r.Configuration.Issue.ID.Prefix.String(),
	)
	encoder.string(
		"configuration.issue.id.strategy",
		r.Configuration.Issue.ID.Strategy.String(),
	)
	encoder.unsigned(
		"configuration.issue.summary.max_bytes",
		r.Configuration.Issue.Summary.MaxBytes.Uint64(),
	)
	encoder.unsigned(
		"configuration.attachment.max_bytes",
		r.Configuration.Attachment.MaxBytes.Uint64(),
	)
}

// CopyIssue contains one portable issue projection.
// Canonical issue records are ordered by ID.
type CopyIssue struct {
	// ID is the source issue identity.
	ID string

	// Title is the issue's one-line identity.
	Title string

	// Kind is the issue type.
	Kind string

	// Lifecycle is the persisted open, closed, or cancelled state.
	Lifecycle string

	// Priority is the issue scheduling priority.
	Priority int64

	// CreatedAt is the issue creation time.
	CreatedAt time.Time

	// UpdatedAt is the most recent issue update time.
	UpdatedAt time.Time

	// ClosedAt is present for closed issues.
	ClosedAt *time.Time

	// WaitingReason explains the current directed wait when present.
	WaitingReason *string

	// WaitingSince records when the current directed wait began.
	WaitingSince *time.Time

	// Summary is the optional stable issue contract.
	Summary *string

	// Details is the optional expanded stable context.
	Details *string
}

func (CopyIssue) recordType() RecordType { return RecordTypeIssue }

func (r CopyIssue) compareRecordKey(other Record) int {
	return strings.Compare(r.ID, other.(CopyIssue).ID)
}

func (CopyIssue) addToRecordCounts(counts *RecordCounts) error {
	counts.Issues++
	return nil
}

func (r CopyIssue) encodeSemantic(encoder *semanticEncoder) {
	encoder.marker("issues.item")
	encoder.string("issue.id", r.ID)
	encoder.string("issue.title", r.Title)
	encoder.string("issue.kind", r.Kind)
	encoder.string("issue.lifecycle", r.Lifecycle)
	encoder.signed("issue.priority", r.Priority)
	encoder.timestamp("issue.created_at", r.CreatedAt)
	encoder.timestamp("issue.updated_at", r.UpdatedAt)
	encoder.optionalTimestamp("issue.closed_at", r.ClosedAt)
	encoder.optionalString("issue.waiting_reason", r.WaitingReason)
	encoder.optionalTimestamp("issue.waiting_since", r.WaitingSince)
	encoder.optionalString("issue.summary", r.Summary)
	encoder.optionalString("issue.details", r.Details)
}

// CopyLabel associates one inert label with an issue.
// Canonical label records are ordered by IssueID, then Label.
type CopyLabel struct {
	// IssueID identifies the labeled source issue.
	IssueID string

	// Label is the inert label value.
	Label string
}

func (CopyLabel) recordType() RecordType { return RecordTypeLabel }

func (r CopyLabel) compareRecordKey(other Record) int {
	value := other.(CopyLabel)
	return compareRecordKeyPair(r.IssueID, r.Label, value.IssueID, value.Label)
}

func (CopyLabel) addToRecordCounts(counts *RecordCounts) error {
	counts.Labels++
	return nil
}

func (r CopyLabel) encodeSemantic(encoder *semanticEncoder) {
	encoder.marker("labels.item")
	encoder.string("label.issue_id", r.IssueID)
	encoder.string("label.value", r.Label)
}

// CopyDependency records one issue prerequisite edge.
// Canonical dependency records are ordered by IssueID, then PrerequisiteID.
type CopyDependency struct {
	// IssueID identifies the dependent source issue.
	IssueID string

	// PrerequisiteID identifies the prerequisite source issue.
	PrerequisiteID string
}

func (CopyDependency) recordType() RecordType { return RecordTypeDependency }

func (r CopyDependency) compareRecordKey(other Record) int {
	value := other.(CopyDependency)
	return compareRecordKeyPair(
		r.IssueID,
		r.PrerequisiteID,
		value.IssueID,
		value.PrerequisiteID,
	)
}

func (CopyDependency) addToRecordCounts(counts *RecordCounts) error {
	counts.Dependencies++
	return nil
}

func (r CopyDependency) encodeSemantic(encoder *semanticEncoder) {
	encoder.marker("dependencies.item")
	encoder.string("dependency.issue_id", r.IssueID)
	encoder.string("dependency.prerequisite_id", r.PrerequisiteID)
}

// CopyContainment records one child-parent edge.
// Canonical containment records are ordered by ChildID.
type CopyContainment struct {
	// ChildID identifies the contained source issue.
	ChildID string

	// ParentID identifies the containing source issue.
	ParentID string
}

func (CopyContainment) recordType() RecordType { return RecordTypeContainment }

func (r CopyContainment) compareRecordKey(other Record) int {
	return strings.Compare(r.ChildID, other.(CopyContainment).ChildID)
}

func (CopyContainment) addToRecordCounts(counts *RecordCounts) error {
	counts.Containment++
	return nil
}

func (r CopyContainment) encodeSemantic(encoder *semanticEncoder) {
	encoder.marker("containment.item")
	encoder.string("containment.child_id", r.ChildID)
	encoder.string("containment.parent_id", r.ParentID)
}

// CopyExternalKey associates one board-scoped producer key with an issue.
// Canonical external-key records are ordered by Key, then IssueID.
type CopyExternalKey struct {
	// Key is the board-scoped producer key.
	Key string

	// IssueID identifies the associated source issue.
	IssueID string
}

func (CopyExternalKey) recordType() RecordType { return RecordTypeExternalKey }

func (r CopyExternalKey) compareRecordKey(other Record) int {
	value := other.(CopyExternalKey)
	return compareRecordKeyPair(r.Key, r.IssueID, value.Key, value.IssueID)
}

func (CopyExternalKey) addToRecordCounts(counts *RecordCounts) error {
	counts.ExternalKeys++
	return nil
}

func (r CopyExternalKey) encodeSemantic(encoder *semanticEncoder) {
	encoder.marker("external_keys.item")
	encoder.string("external_key.value", r.Key)
	encoder.string("external_key.issue_id", r.IssueID)
}

// CopyLogEntry contains one immutable Log record.
// Canonical Log records use contiguous zero-based Order values.
type CopyLogEntry struct {
	// Order is the stable source board Log sequence.
	Order uint64

	// ID is the source Log identity.
	ID string

	// IssueID identifies the source issue that owns the Log entry.
	IssueID string

	// Kind identifies a State snapshot or standalone post.
	Kind string

	// Author is the attributed author when present.
	Author *string

	// Committer is the persisted committer when present.
	Committer *string

	// Body is the durable Markdown record content.
	Body string

	// CreatedAt is the attributed creation time when present.
	CreatedAt *time.Time

	// NextAction is meaningful for State snapshot records.
	NextAction *string
}

func (CopyLogEntry) recordType() RecordType { return RecordTypeLogEntry }

func (r CopyLogEntry) compareRecordKey(other Record) int {
	return cmp.Compare(r.Order, other.(CopyLogEntry).Order)
}

func (r CopyLogEntry) addToRecordCounts(counts *RecordCounts) error {
	if r.Order != counts.LogEntries {
		return fmt.Errorf(
			"board Log record order is %d, expected %d",
			r.Order,
			counts.LogEntries,
		)
	}
	counts.LogEntries++
	return nil
}

func (r CopyLogEntry) encodeSemantic(encoder *semanticEncoder) {
	encoder.marker("log_entries.item")
	encoder.unsigned("log_entry.order", r.Order)
	encoder.string("log_entry.id", r.ID)
	encoder.string("log_entry.issue_id", r.IssueID)
	encoder.string("log_entry.kind", r.Kind)
	encoder.optionalString("log_entry.author", r.Author)
	encoder.optionalString("log_entry.committer", r.Committer)
	encoder.string("log_entry.body", r.Body)
	encoder.optionalTimestamp("log_entry.created_at", r.CreatedAt)
	encoder.optionalString("log_entry.next_action", r.NextAction)
}

// CopyState contains one issue's mutable recovery record.
// Canonical State records are ordered by IssueID.
type CopyState struct {
	// IssueID identifies the source issue that owns the State.
	IssueID string

	// Body is the current recovery content.
	Body string

	// Author is the current attributed author when present.
	Author *string

	// UpdatedAt is the most recent State update time when present.
	UpdatedAt *time.Time

	// SnapshotLogEntryID identifies the committed State snapshot when present.
	SnapshotLogEntryID *string

	// NextAction is the planned transition when present.
	NextAction *string
}

func (CopyState) recordType() RecordType { return RecordTypeState }

func (r CopyState) compareRecordKey(other Record) int {
	return strings.Compare(r.IssueID, other.(CopyState).IssueID)
}

func (CopyState) addToRecordCounts(counts *RecordCounts) error {
	counts.States++
	return nil
}

func (r CopyState) encodeSemantic(encoder *semanticEncoder) {
	encoder.marker("states.item")
	encoder.string("state.issue_id", r.IssueID)
	encoder.string("state.body", r.Body)
	encoder.optionalString("state.author", r.Author)
	encoder.optionalTimestamp("state.updated_at", r.UpdatedAt)
	encoder.optionalString("state.snapshot_log_entry_id", r.SnapshotLogEntryID)
	encoder.optionalString("state.next_action", r.NextAction)
}

// CopyResultRecord contains one issue's current durable outcome.
// Canonical Result records are ordered by IssueID.
type CopyResultRecord struct {
	// IssueID identifies the source issue that owns the Result.
	IssueID string

	// Body is the durable Markdown outcome.
	Body string
}

func (CopyResultRecord) recordType() RecordType { return RecordTypeResult }

func (r CopyResultRecord) compareRecordKey(other Record) int {
	return strings.Compare(r.IssueID, other.(CopyResultRecord).IssueID)
}

func (CopyResultRecord) addToRecordCounts(counts *RecordCounts) error {
	counts.Results++
	return nil
}

func (r CopyResultRecord) encodeSemantic(encoder *semanticEncoder) {
	encoder.marker("results.item")
	encoder.string("result.issue_id", r.IssueID)
	encoder.string("result.body", r.Body)
}

// CopyCheckpoint contains one immutable checkpoint decision.
// Canonical checkpoint records are ordered by IssueID.
type CopyCheckpoint struct {
	// IssueID identifies the source checkpoint issue.
	IssueID string

	// Outcome is the approved or denied checkpoint decision.
	Outcome string

	// Reason explains the checkpoint decision.
	Reason string

	// DecidedAt is the checkpoint decision time.
	DecidedAt time.Time
}

func (CopyCheckpoint) recordType() RecordType { return RecordTypeCheckpoint }

func (r CopyCheckpoint) compareRecordKey(other Record) int {
	return strings.Compare(r.IssueID, other.(CopyCheckpoint).IssueID)
}

func (CopyCheckpoint) addToRecordCounts(counts *RecordCounts) error {
	counts.Checkpoints++
	return nil
}

func (r CopyCheckpoint) encodeSemantic(encoder *semanticEncoder) {
	encoder.marker("checkpoints.item")
	encoder.string("checkpoint.issue_id", r.IssueID)
	encoder.string("checkpoint.outcome", r.Outcome)
	encoder.string("checkpoint.reason", r.Reason)
	encoder.timestamp("checkpoint.decided_at", r.DecidedAt)
}

// CopyAttachment contains active or removed board-scoped attachment metadata.
// Canonical attachment records are ordered by ID.
type CopyAttachment struct {
	// ID is the source attachment identity.
	ID string

	// OriginIssueID identifies the source issue that introduced the attachment.
	OriginIssueID *string

	// Blob identifies the immutable attachment body.
	Blob attachment.BlobDescriptor

	// Filename is the source attachment filename.
	Filename string

	// MediaType is the source attachment media type.
	MediaType string

	// Lifecycle is the active or removed attachment state.
	Lifecycle string

	// CreatedActor is the actor that created the attachment.
	CreatedActor string

	// CreatedAt is the attachment creation time.
	CreatedAt time.Time

	// RemovedActor is the actor that removed the attachment when present.
	RemovedActor *string

	// RemovedAt is the attachment removal time when present.
	RemovedAt *time.Time
}

func (CopyAttachment) recordType() RecordType { return RecordTypeAttachment }

func (r CopyAttachment) compareRecordKey(other Record) int {
	return strings.Compare(r.ID, other.(CopyAttachment).ID)
}

func (r CopyAttachment) addToRecordCounts(counts *RecordCounts) error {
	if err := r.Blob.Validate(); err != nil {
		return fmt.Errorf("board attachment %q: %w", r.ID, err)
	}
	counts.Attachments++
	return nil
}

func (r CopyAttachment) encodeSemantic(encoder *semanticEncoder) {
	encoder.marker("attachments.item")
	encoder.string("attachment.id", r.ID)
	encoder.optionalString("attachment.origin_issue_id", r.OriginIssueID)
	encoder.string("attachment.blob.digest", r.Blob.Digest.String())
	encoder.unsigned("attachment.blob.size_bytes", r.Blob.SizeBytes)
	encoder.string("attachment.filename", r.Filename)
	encoder.string("attachment.media_type", r.MediaType)
	encoder.string("attachment.lifecycle", r.Lifecycle)
	encoder.string("attachment.created_actor", r.CreatedActor)
	encoder.timestamp("attachment.created_at", r.CreatedAt)
	encoder.optionalString("attachment.removed_actor", r.RemovedActor)
	encoder.optionalTimestamp("attachment.removed_at", r.RemovedAt)
}

// RecordCounts reports the number of values in each canonical record section.
// The required RecordTrailer carries all fields and must equal the counts
// accumulated from its preceding payloads.
type RecordCounts struct {
	// Issues is the number of issue records.
	Issues uint64

	// Labels is the number of issue-label records.
	Labels uint64

	// Dependencies is the number of prerequisite-edge records.
	Dependencies uint64

	// Containment is the number of containment-edge records.
	Containment uint64

	// ExternalKeys is the number of external-key records.
	ExternalKeys uint64

	// LogEntries is the number of immutable Log records.
	LogEntries uint64

	// States is the number of mutable State records.
	States uint64

	// Results is the number of durable Result records.
	Results uint64

	// Checkpoints is the number of checkpoint-decision records.
	Checkpoints uint64

	// Attachments is the number of attachment-metadata records.
	Attachments uint64
}

// RecordTrailer terminates a complete record stream.
//
// Counts let readers distinguish a complete stream from one that ends cleanly
// at an intermediate record boundary.
type RecordTrailer struct {
	// Counts reports the completed section sizes.
	Counts RecordCounts
}

func (RecordTrailer) recordType() RecordType { return RecordTypeTrailer }

func (RecordTrailer) compareRecordKey(Record) int { return 0 }

func (r RecordTrailer) addToRecordCounts(counts *RecordCounts) error {
	if r.Counts != *counts {
		return fmt.Errorf(
			"board record trailer counts %+v do not match records %+v",
			r.Counts,
			*counts,
		)
	}
	return nil
}

func (r RecordTrailer) encodeSemantic(encoder *semanticEncoder) {
	encoder.marker("trailer")
	encoder.unsigned("issues.count", r.Counts.Issues)
	encoder.unsigned("labels.count", r.Counts.Labels)
	encoder.unsigned("dependencies.count", r.Counts.Dependencies)
	encoder.unsigned("containment.count", r.Counts.Containment)
	encoder.unsigned("external_keys.count", r.Counts.ExternalKeys)
	encoder.unsigned("log_entries.count", r.Counts.LogEntries)
	encoder.unsigned("states.count", r.Counts.States)
	encoder.unsigned("results.count", r.Counts.Results)
	encoder.unsigned("checkpoints.count", r.Counts.Checkpoints)
	encoder.unsigned("attachments.count", r.Counts.Attachments)
}

// RecordTypeOf reports the canonical section containing record.
func RecordTypeOf(record Record) RecordType {
	if record == nil {
		return RecordTypeUnknown
	}
	return record.recordType()
}

func compareRecordKeyPair(
	leftFirst string,
	leftSecond string,
	rightFirst string,
	rightSecond string,
) int {
	if compared := strings.Compare(leftFirst, rightFirst); compared != 0 {
		return compared
	}
	return strings.Compare(leftSecond, rightSecond)
}

// RecordSequence yields one canonical board publication incrementally.
//
// A non-nil error is terminal. Consumers may stop early without draining the
// sequence; an owned source must still release its retained view.
type RecordSequence = iter.Seq2[Record, error]

// RecordSource reads one retained semantic board view incrementally.
//
// Iteration owns the retained source view and releases it on completion or
// early termination.
type RecordSource interface {
	ReadCopyRecords(
		context.Context,
		board.ID,
		configuration.Overrides,
	) RecordSequence
}

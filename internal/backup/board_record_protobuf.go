package backup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	backupv1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/backup/v1"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/proto"
)

// BoardRecordEncoder writes deterministic length-delimited protobuf records.
//
// Callers may wrap destination with a hash and counting writer so the framed
// bytes determine both member integrity facts without buffering the member.
type BoardRecordEncoder struct {
	destination io.Writer // required
}

// NewBoardRecordEncoder constructs a board record encoder.
func NewBoardRecordEncoder(destination io.Writer) *BoardRecordEncoder {
	return &BoardRecordEncoder{destination: destination}
}

// Write appends one deterministic length-delimited board record.
func (e *BoardRecordEncoder) Write(record boardcopy.Record) error {
	encoded, err := boardRecordToProto(record)
	if err != nil {
		return err
	}
	_, err = (protodelim.MarshalOptions{MarshalOptions: proto.MarshalOptions{
		Deterministic: true,
	}}).MarshalTo(e.destination, encoded)
	if err != nil {
		return fmt.Errorf("write board record: %w", err)
	}
	return nil
}

// BoardRecordDecoder reads length-delimited protobuf records one at a time.
type BoardRecordDecoder struct {
	source       *bufio.Reader // required
	maxSizeBytes int64
}

// NewBoardRecordDecoder constructs a board record decoder bounded by the
// containing member's uncompressed size.
func NewBoardRecordDecoder(
	source io.Reader,
	maxSizeBytes uint64,
) *BoardRecordDecoder {
	return &BoardRecordDecoder{
		source:       bufio.NewReader(source),
		maxSizeBytes: int64(min(maxSizeBytes, uint64(math.MaxInt64))),
	}
}

// Read decodes the next length-delimited board record.
func (d *BoardRecordDecoder) Read() (boardcopy.Record, error) {
	var encoded backupv1.BoardRecord
	if err := (protodelim.UnmarshalOptions{MaxSize: d.maxSizeBytes}).UnmarshalFrom(
		d.source,
		&encoded,
	); err != nil {
		return nil, err
	}
	record, err := boardRecordFromProto(&encoded)
	if err != nil {
		return nil, fmt.Errorf("decode board record: %w", err)
	}
	return record, nil
}

func boardRecordToProto(record boardcopy.Record) (*backupv1.BoardRecord, error) {
	switch value := record.(type) {
	case boardcopy.RecordHeader:
		encoded, err := boardHeaderToProto(value)
		if err != nil {
			return nil, err
		}
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_Header{
			Header: encoded,
		}}, nil
	case boardcopy.CopyIssue:
		encoded, err := copyIssueToProto(value)
		if err != nil {
			return nil, err
		}
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_Issue{
			Issue: encoded,
		}}, nil
	case boardcopy.CopyLabel:
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_Label{
			Label: &backupv1.Label{IssueId: value.IssueID, Value: value.Label},
		}}, nil
	case boardcopy.CopyDependency:
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_Dependency{
			Dependency: &backupv1.Dependency{
				IssueId: value.IssueID, PrerequisiteId: value.PrerequisiteID,
			},
		}}, nil
	case boardcopy.CopyContainment:
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_Containment{
			Containment: &backupv1.Containment{
				ChildId: value.ChildID, ParentId: value.ParentID,
			},
		}}, nil
	case boardcopy.CopyExternalKey:
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_ExternalKey{
			ExternalKey: &backupv1.ExternalKey{
				Key: value.Key, IssueId: value.IssueID,
			},
		}}, nil
	case boardcopy.CopyLogEntry:
		encoded, err := copyLogEntryToProto(value)
		if err != nil {
			return nil, err
		}
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_LogEntry{
			LogEntry: encoded,
		}}, nil
	case boardcopy.CopyState:
		encoded, err := copyStateToProto(value)
		if err != nil {
			return nil, err
		}
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_State{
			State: encoded,
		}}, nil
	case boardcopy.CopyResultRecord:
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_Result{
			Result: &backupv1.Result{IssueId: value.IssueID, Body: value.Body},
		}}, nil
	case boardcopy.CopyCheckpoint:
		encoded, err := copyCheckpointToProto(value)
		if err != nil {
			return nil, err
		}
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_Checkpoint{
			Checkpoint: encoded,
		}}, nil
	case boardcopy.CopyAttachment:
		encoded, err := copyAttachmentToProto(value)
		if err != nil {
			return nil, err
		}
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_Attachment{
			Attachment: encoded,
		}}, nil
	case boardcopy.RecordTrailer:
		counts := value.Counts
		return &backupv1.BoardRecord{Value: &backupv1.BoardRecord_Trailer{
			Trailer: &backupv1.BoardTrailer{
				Issues: counts.Issues, Labels: counts.Labels,
				Dependencies: counts.Dependencies,
				Containment:  counts.Containment, ExternalKeys: counts.ExternalKeys,
				LogEntries: counts.LogEntries, States: counts.States,
				Results: counts.Results, Checkpoints: counts.Checkpoints,
				Attachments: counts.Attachments,
			},
		}}, nil
	default:
		return nil, errors.New("board record is required")
	}
}

func boardRecordFromProto(encoded *backupv1.BoardRecord) (boardcopy.Record, error) {
	switch value := encoded.GetValue().(type) {
	case *backupv1.BoardRecord_Header:
		return boardHeaderFromProto(value.Header)
	case *backupv1.BoardRecord_Issue:
		return copyIssueFromProto(value.Issue)
	case *backupv1.BoardRecord_Label:
		if value.Label == nil {
			return nil, errors.New("label is required")
		}
		return boardcopy.CopyLabel{
			IssueID: value.Label.GetIssueId(), Label: value.Label.GetValue(),
		}, nil
	case *backupv1.BoardRecord_Dependency:
		if value.Dependency == nil {
			return nil, errors.New("dependency is required")
		}
		return boardcopy.CopyDependency{
			IssueID:        value.Dependency.GetIssueId(),
			PrerequisiteID: value.Dependency.GetPrerequisiteId(),
		}, nil
	case *backupv1.BoardRecord_Containment:
		if value.Containment == nil {
			return nil, errors.New("containment edge is required")
		}
		return boardcopy.CopyContainment{
			ChildID:  value.Containment.GetChildId(),
			ParentID: value.Containment.GetParentId(),
		}, nil
	case *backupv1.BoardRecord_ExternalKey:
		if value.ExternalKey == nil {
			return nil, errors.New("external key is required")
		}
		return boardcopy.CopyExternalKey{
			Key: value.ExternalKey.GetKey(), IssueID: value.ExternalKey.GetIssueId(),
		}, nil
	case *backupv1.BoardRecord_LogEntry:
		return copyLogEntryFromProto(value.LogEntry)
	case *backupv1.BoardRecord_State:
		return copyStateFromProto(value.State)
	case *backupv1.BoardRecord_Result:
		if value.Result == nil {
			return nil, errors.New("result is required")
		}
		return boardcopy.CopyResultRecord{
			IssueID: value.Result.GetIssueId(), Body: value.Result.GetBody(),
		}, nil
	case *backupv1.BoardRecord_Checkpoint:
		return copyCheckpointFromProto(value.Checkpoint)
	case *backupv1.BoardRecord_Attachment:
		return copyAttachmentFromProto(value.Attachment)
	case *backupv1.BoardRecord_Trailer:
		if value.Trailer == nil {
			return nil, errors.New("board trailer is required")
		}
		return boardcopy.RecordTrailer{Counts: boardcopy.RecordCounts{
			Issues: value.Trailer.GetIssues(), Labels: value.Trailer.GetLabels(),
			Dependencies: value.Trailer.GetDependencies(),
			Containment:  value.Trailer.GetContainment(),
			ExternalKeys: value.Trailer.GetExternalKeys(),
			LogEntries:   value.Trailer.GetLogEntries(),
			States:       value.Trailer.GetStates(), Results: value.Trailer.GetResults(),
			Checkpoints: value.Trailer.GetCheckpoints(),
			Attachments: value.Trailer.GetAttachments(),
		}}, nil
	default:
		return nil, errors.New("board record value is required")
	}
}

func boardHeaderToProto(value boardcopy.RecordHeader) (*backupv1.BoardHeader, error) {
	createdAt, err := timestampToProto("board creation time", value.Board.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &backupv1.BoardHeader{
		Version: uint32(value.Version), SourceLineageId: value.SourceLineageID,
		SourceRevision: value.SourceRevision,
		Board: &backupv1.Board{
			Id: value.Board.ID, Name: value.Board.Name,
			Description: value.Board.Description, CreatedAt: createdAt,
		},
		Configuration: &backupv1.Configuration{
			IssueIdPrefix:        value.Configuration.Issue.ID.Prefix.String(),
			IssueIdStrategy:      value.Configuration.Issue.ID.Strategy.String(),
			IssueSummaryMaxBytes: value.Configuration.Issue.Summary.MaxBytes.Uint64(),
			AttachmentMaxBytes:   value.Configuration.Attachment.MaxBytes.Uint64(),
			BoardPinsMaxCount:    value.Configuration.Board.Pins.MaxCount.Uint64(),
		},
	}, nil
}

func boardHeaderFromProto(value *backupv1.BoardHeader) (boardcopy.Record, error) {
	if value == nil || value.GetBoard() == nil {
		return nil, errors.New("board header and board are required")
	}
	if value.GetConfiguration() == nil {
		return nil, errors.New("configuration is required")
	}
	createdAt, err := timestampFromProto("board creation time", value.GetBoard().GetCreatedAt())
	if err != nil {
		return nil, err
	}
	resolved, err := configurationFromProto(value.GetConfiguration())
	if err != nil {
		return nil, err
	}
	return boardcopy.RecordHeader{
		Version:         int(value.GetVersion()),
		SourceLineageID: value.GetSourceLineageId(),
		SourceRevision:  value.GetSourceRevision(),
		Board: boardcopy.CopyBoard{
			ID: value.GetBoard().GetId(), Name: value.GetBoard().GetName(),
			Description: cloneString(value.GetBoard().Description),
			CreatedAt:   createdAt,
		},
		Configuration: resolved,
	}, nil
}

func copyIssueToProto(value boardcopy.CopyIssue) (*backupv1.Issue, error) {
	createdAt, err := timestampToProto("issue creation time", value.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("issue %q: %w", value.ID, err)
	}
	updatedAt, err := timestampToProto("issue update time", value.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("issue %q: %w", value.ID, err)
	}
	closedAt, err := optionalTimestampToProto("issue closure time", value.ClosedAt)
	if err != nil {
		return nil, fmt.Errorf("issue %q: %w", value.ID, err)
	}
	waitingSince, err := optionalTimestampToProto("issue waiting time", value.WaitingSince)
	if err != nil {
		return nil, fmt.Errorf("issue %q: %w", value.ID, err)
	}
	return &backupv1.Issue{
		Id: value.ID, Title: value.Title, Kind: value.Kind,
		Lifecycle: value.Lifecycle, Priority: value.Priority,
		CreatedAt: createdAt, UpdatedAt: updatedAt, ClosedAt: closedAt,
		WaitingReason: value.WaitingReason, WaitingSince: waitingSince,
		Summary: value.Summary, Details: value.Details,
	}, nil
}

func copyIssueFromProto(value *backupv1.Issue) (boardcopy.Record, error) {
	if value == nil {
		return nil, errors.New("issue is required")
	}
	createdAt, err := timestampFromProto("issue creation time", value.GetCreatedAt())
	if err != nil {
		return nil, fmt.Errorf("issue %q: %w", value.GetId(), err)
	}
	updatedAt, err := timestampFromProto("issue update time", value.GetUpdatedAt())
	if err != nil {
		return nil, fmt.Errorf("issue %q: %w", value.GetId(), err)
	}
	closedAt, err := optionalTimestampFromProto("issue closure time", value.GetClosedAt())
	if err != nil {
		return nil, fmt.Errorf("issue %q: %w", value.GetId(), err)
	}
	waitingSince, err := optionalTimestampFromProto("issue waiting time", value.GetWaitingSince())
	if err != nil {
		return nil, fmt.Errorf("issue %q: %w", value.GetId(), err)
	}
	return boardcopy.CopyIssue{
		ID: value.GetId(), Title: value.GetTitle(), Kind: value.GetKind(),
		Lifecycle: value.GetLifecycle(), Priority: value.GetPriority(),
		CreatedAt: createdAt, UpdatedAt: updatedAt, ClosedAt: closedAt,
		WaitingReason: cloneString(value.WaitingReason), WaitingSince: waitingSince,
		Summary: cloneString(value.Summary), Details: cloneString(value.Details),
	}, nil
}

func copyLogEntryToProto(value boardcopy.CopyLogEntry) (*backupv1.LogEntry, error) {
	createdAt, err := optionalTimestampToProto("Log entry creation time", value.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("log entry %q: %w", value.ID, err)
	}
	return &backupv1.LogEntry{
		Order: value.Order, Id: value.ID, IssueId: value.IssueID,
		Kind: value.Kind, Author: value.Author, Committer: value.Committer,
		Body: value.Body, CreatedAt: createdAt, NextAction: value.NextAction,
	}, nil
}

func copyLogEntryFromProto(value *backupv1.LogEntry) (boardcopy.Record, error) {
	if value == nil {
		return nil, errors.New("log entry is required")
	}
	createdAt, err := optionalTimestampFromProto("Log entry creation time", value.GetCreatedAt())
	if err != nil {
		return nil, fmt.Errorf("log entry %q: %w", value.GetId(), err)
	}
	return boardcopy.CopyLogEntry{
		Order: value.GetOrder(), ID: value.GetId(), IssueID: value.GetIssueId(),
		Kind: value.GetKind(), Author: cloneString(value.Author),
		Committer: cloneString(value.Committer), Body: value.GetBody(),
		CreatedAt: createdAt, NextAction: cloneString(value.NextAction),
	}, nil
}

func copyStateToProto(value boardcopy.CopyState) (*backupv1.State, error) {
	updatedAt, err := optionalTimestampToProto("state update time", value.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("state for issue %q: %w", value.IssueID, err)
	}
	return &backupv1.State{
		IssueId: value.IssueID, Body: value.Body, Author: value.Author,
		UpdatedAt: updatedAt, SnapshotLogEntryId: value.SnapshotLogEntryID,
		NextAction: value.NextAction,
	}, nil
}

func copyStateFromProto(value *backupv1.State) (boardcopy.Record, error) {
	if value == nil {
		return nil, errors.New("state is required")
	}
	updatedAt, err := optionalTimestampFromProto("state update time", value.GetUpdatedAt())
	if err != nil {
		return nil, fmt.Errorf("state for issue %q: %w", value.GetIssueId(), err)
	}
	return boardcopy.CopyState{
		IssueID: value.GetIssueId(), Body: value.GetBody(),
		Author: cloneString(value.Author), UpdatedAt: updatedAt,
		SnapshotLogEntryID: cloneString(value.SnapshotLogEntryId),
		NextAction:         cloneString(value.NextAction),
	}, nil
}

func copyCheckpointToProto(value boardcopy.CopyCheckpoint) (*backupv1.Checkpoint, error) {
	decidedAt, err := timestampToProto("checkpoint decision time", value.DecidedAt)
	if err != nil {
		return nil, fmt.Errorf("checkpoint %q: %w", value.IssueID, err)
	}
	return &backupv1.Checkpoint{
		IssueId: value.IssueID, Outcome: value.Outcome,
		Reason: value.Reason, DecidedAt: decidedAt,
	}, nil
}

func copyCheckpointFromProto(value *backupv1.Checkpoint) (boardcopy.Record, error) {
	if value == nil {
		return nil, errors.New("checkpoint is required")
	}
	decidedAt, err := timestampFromProto("checkpoint decision time", value.GetDecidedAt())
	if err != nil {
		return nil, fmt.Errorf("checkpoint %q: %w", value.GetIssueId(), err)
	}
	return boardcopy.CopyCheckpoint{
		IssueID: value.GetIssueId(), Outcome: value.GetOutcome(),
		Reason: value.GetReason(), DecidedAt: decidedAt,
	}, nil
}

func copyAttachmentToProto(value boardcopy.CopyAttachment) (*backupv1.Attachment, error) {
	createdAt, err := timestampToProto("attachment creation time", value.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("attachment %q: %w", value.ID, err)
	}
	removedAt, err := optionalTimestampToProto("attachment removal time", value.RemovedAt)
	if err != nil {
		return nil, fmt.Errorf("attachment %q: %w", value.ID, err)
	}
	return &backupv1.Attachment{
		Id: value.ID, OriginIssueId: value.OriginIssueID,
		Blob: &backupv1.BlobDescriptor{
			Digest: value.Blob.Digest.String(), SizeBytes: value.Blob.SizeBytes,
		},
		Filename: value.Filename, MediaType: value.MediaType,
		Lifecycle: value.Lifecycle, CreatedActor: value.CreatedActor,
		CreatedAt: createdAt, RemovedActor: value.RemovedActor,
		RemovedAt: removedAt,
	}, nil
}

func copyAttachmentFromProto(value *backupv1.Attachment) (boardcopy.Record, error) {
	if value == nil || value.GetBlob() == nil {
		return nil, errors.New("attachment and blob are required")
	}
	digest, err := attachment.NewDigest(value.GetBlob().GetDigest())
	if err != nil {
		return nil, fmt.Errorf("attachment %q: %w", value.GetId(), err)
	}
	descriptor := attachment.BlobDescriptor{
		Digest: digest, SizeBytes: value.GetBlob().GetSizeBytes(),
	}
	if err := descriptor.Validate(); err != nil {
		return nil, fmt.Errorf("attachment %q: %w", value.GetId(), err)
	}
	createdAt, err := timestampFromProto("attachment creation time", value.GetCreatedAt())
	if err != nil {
		return nil, fmt.Errorf("attachment %q: %w", value.GetId(), err)
	}
	removedAt, err := optionalTimestampFromProto("attachment removal time", value.GetRemovedAt())
	if err != nil {
		return nil, fmt.Errorf("attachment %q: %w", value.GetId(), err)
	}
	return boardcopy.CopyAttachment{
		ID: value.GetId(), OriginIssueID: cloneString(value.OriginIssueId),
		Blob: descriptor, Filename: value.GetFilename(),
		MediaType: value.GetMediaType(), Lifecycle: value.GetLifecycle(),
		CreatedActor: value.GetCreatedActor(), CreatedAt: createdAt,
		RemovedActor: cloneString(value.RemovedActor), RemovedAt: removedAt,
	}, nil
}

func configurationFromProto(
	encoded *backupv1.Configuration,
) (configuration.Configuration, error) {
	prefix, err := configuration.NewPrefix(encoded.GetIssueIdPrefix())
	if err != nil {
		return configuration.Configuration{}, err
	}
	strategy, err := configuration.NewIDStrategy(encoded.GetIssueIdStrategy())
	if err != nil {
		return configuration.Configuration{}, err
	}
	summaryMaxBytes, err := configuration.NewByteLimit(encoded.GetIssueSummaryMaxBytes())
	if err != nil {
		return configuration.Configuration{}, err
	}
	attachmentMaxBytes, err := configuration.NewByteLimit(encoded.GetAttachmentMaxBytes())
	if err != nil {
		return configuration.Configuration{}, err
	}
	pinMaxCount, err := board.NewPinLimit(encoded.GetBoardPinsMaxCount())
	if err != nil {
		return configuration.Configuration{}, err
	}
	return configuration.Configuration{
		Issue: configuration.IssueConfiguration{
			ID: configuration.IssueIDConfiguration{
				Prefix: prefix, Strategy: strategy,
			},
			Summary: configuration.SummaryConfiguration{MaxBytes: summaryMaxBytes},
		},
		Attachment: configuration.AttachmentConfiguration{MaxBytes: attachmentMaxBytes},
		Board: configuration.BoardConfiguration{
			Pins: configuration.PinConfiguration{MaxCount: pinMaxCount},
		},
	}, nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	return new(*value)
}

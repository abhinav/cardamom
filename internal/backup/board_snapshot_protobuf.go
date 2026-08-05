package backup

import (
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	backupv1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/backup/v1"
)

func snapshotToProto(snapshot boardcopy.CopySnapshot) (*backupv1.BoardSnapshot, error) {
	boardCreatedAt, err := timestampToProto("board creation time", snapshot.Board.CreatedAt)
	if err != nil {
		return nil, err
	}
	encoded := &backupv1.BoardSnapshot{
		SourceLineageId: snapshot.SourceLineageID,
		SourceRevision:  snapshot.SourceRevision,
		Version:         uint32(snapshot.Version),
		Digest:          snapshot.Digest,
		Board: &backupv1.Board{
			Id: snapshot.Board.ID, Name: snapshot.Board.Name,
			Description: snapshot.Board.Description, CreatedAt: boardCreatedAt,
		},
		Configuration: &backupv1.Configuration{
			IssueIdPrefix:        snapshot.Configuration.Issue.ID.Prefix.String(),
			IssueIdStrategy:      snapshot.Configuration.Issue.ID.Strategy.String(),
			IssueSummaryMaxBytes: snapshot.Configuration.Issue.Summary.MaxBytes.Uint64(),
			AttachmentMaxBytes:   snapshot.Configuration.Attachment.MaxBytes.Uint64(),
		},
		Labels:       make([]*backupv1.Label, 0, len(snapshot.Labels)),
		Dependencies: make([]*backupv1.Dependency, 0, len(snapshot.Dependencies)),
		Containment:  make([]*backupv1.Containment, 0, len(snapshot.Containment)),
		ExternalKeys: make([]*backupv1.ExternalKey, 0, len(snapshot.ExternalKeys)),
		Results:      make([]*backupv1.Result, 0, len(snapshot.Results)),
	}

	for _, value := range snapshot.Issues {
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
		encoded.Issues = append(encoded.Issues, &backupv1.Issue{
			Id: value.ID, Title: value.Title, Kind: value.Kind,
			Lifecycle: value.Lifecycle, Priority: value.Priority,
			CreatedAt: createdAt, UpdatedAt: updatedAt, ClosedAt: closedAt,
			WaitingReason: value.WaitingReason, WaitingSince: waitingSince,
			Summary: value.Summary, Details: value.Details,
		})
	}
	for _, value := range snapshot.Labels {
		encoded.Labels = append(encoded.Labels, &backupv1.Label{
			IssueId: value.IssueID, Value: value.Label,
		})
	}
	for _, value := range snapshot.Dependencies {
		encoded.Dependencies = append(encoded.Dependencies, &backupv1.Dependency{
			IssueId: value.IssueID, PrerequisiteId: value.PrerequisiteID,
		})
	}
	for _, value := range snapshot.Containment {
		encoded.Containment = append(encoded.Containment, &backupv1.Containment{
			ChildId: value.ChildID, ParentId: value.ParentID,
		})
	}
	for _, value := range snapshot.ExternalKeys {
		encoded.ExternalKeys = append(encoded.ExternalKeys, &backupv1.ExternalKey{
			Key: value.Key, IssueId: value.IssueID,
		})
	}
	for _, value := range snapshot.LogEntries {
		createdAt, err := optionalTimestampToProto("Log entry creation time", value.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("log entry %q: %w", value.ID, err)
		}
		encoded.LogEntries = append(encoded.LogEntries, &backupv1.LogEntry{
			Order: value.Order, Id: value.ID, IssueId: value.IssueID,
			Kind: value.Kind, Author: value.Author, Committer: value.Committer,
			Body: value.Body, CreatedAt: createdAt, NextAction: value.NextAction,
		})
	}
	for _, value := range snapshot.States {
		updatedAt, err := optionalTimestampToProto("state update time", value.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("state for issue %q: %w", value.IssueID, err)
		}
		encoded.States = append(encoded.States, &backupv1.State{
			IssueId: value.IssueID, Body: value.Body, Author: value.Author,
			UpdatedAt: updatedAt, SnapshotLogEntryId: value.SnapshotLogEntryID,
			NextAction: value.NextAction,
		})
	}
	for _, value := range snapshot.Results {
		encoded.Results = append(encoded.Results, &backupv1.Result{
			IssueId: value.IssueID, Body: value.Body,
		})
	}
	for _, value := range snapshot.Checkpoints {
		decidedAt, err := timestampToProto("checkpoint decision time", value.DecidedAt)
		if err != nil {
			return nil, fmt.Errorf("checkpoint %q: %w", value.IssueID, err)
		}
		encoded.Checkpoints = append(encoded.Checkpoints, &backupv1.Checkpoint{
			IssueId: value.IssueID, Outcome: value.Outcome,
			Reason: value.Reason, DecidedAt: decidedAt,
		})
	}
	for _, value := range snapshot.Attachments {
		createdAt, err := timestampToProto("attachment creation time", value.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("attachment %q: %w", value.ID, err)
		}
		removedAt, err := optionalTimestampToProto("attachment removal time", value.RemovedAt)
		if err != nil {
			return nil, fmt.Errorf("attachment %q: %w", value.ID, err)
		}
		encoded.Attachments = append(encoded.Attachments, &backupv1.Attachment{
			Id: value.ID, OriginIssueId: value.OriginIssueID,
			Blob: &backupv1.BlobDescriptor{
				Digest: value.Blob.Digest.String(), SizeBytes: value.Blob.SizeBytes,
			},
			Filename: value.Filename, MediaType: value.MediaType,
			Lifecycle: value.Lifecycle, CreatedActor: value.CreatedActor,
			CreatedAt: createdAt, RemovedActor: value.RemovedActor,
			RemovedAt: removedAt,
		})
	}
	return encoded, nil
}

func snapshotFromProto(encoded *backupv1.BoardSnapshot) (boardcopy.CopySnapshot, error) {
	if encoded.GetBoard() == nil {
		return boardcopy.CopySnapshot{}, errors.New("board is required")
	}
	if encoded.GetConfiguration() == nil {
		return boardcopy.CopySnapshot{}, errors.New("configuration is required")
	}
	boardCreatedAt, err := timestampFromProto("board creation time", encoded.GetBoard().GetCreatedAt())
	if err != nil {
		return boardcopy.CopySnapshot{}, err
	}
	configuration, err := configurationFromProto(encoded.GetConfiguration())
	if err != nil {
		return boardcopy.CopySnapshot{}, err
	}
	snapshot := boardcopy.CopySnapshot{
		SourceLineageID: encoded.GetSourceLineageId(),
		SourceRevision:  encoded.GetSourceRevision(),
		Version:         int(encoded.GetVersion()),
		Digest:          encoded.GetDigest(),
		Board: boardcopy.CopyBoard{
			ID: encoded.GetBoard().GetId(), Name: encoded.GetBoard().GetName(),
			Description: cloneString(encoded.GetBoard().Description),
			CreatedAt:   boardCreatedAt,
		},
		Configuration: configuration,
	}

	for _, value := range encoded.GetIssues() {
		if value == nil {
			return boardcopy.CopySnapshot{}, errors.New("issue is required")
		}
		createdAt, err := timestampFromProto("issue creation time", value.GetCreatedAt())
		if err != nil {
			return boardcopy.CopySnapshot{}, fmt.Errorf("issue %q: %w", value.GetId(), err)
		}
		updatedAt, err := timestampFromProto("issue update time", value.GetUpdatedAt())
		if err != nil {
			return boardcopy.CopySnapshot{}, fmt.Errorf("issue %q: %w", value.GetId(), err)
		}
		closedAt, err := optionalTimestampFromProto("issue closure time", value.GetClosedAt())
		if err != nil {
			return boardcopy.CopySnapshot{}, fmt.Errorf("issue %q: %w", value.GetId(), err)
		}
		waitingSince, err := optionalTimestampFromProto("issue waiting time", value.GetWaitingSince())
		if err != nil {
			return boardcopy.CopySnapshot{}, fmt.Errorf("issue %q: %w", value.GetId(), err)
		}
		snapshot.Issues = append(snapshot.Issues, boardcopy.CopyIssue{
			ID: value.GetId(), Title: value.GetTitle(), Kind: value.GetKind(),
			Lifecycle: value.GetLifecycle(), Priority: value.GetPriority(),
			CreatedAt: createdAt, UpdatedAt: updatedAt, ClosedAt: closedAt,
			WaitingReason: cloneString(value.WaitingReason), WaitingSince: waitingSince,
			Summary: cloneString(value.Summary), Details: cloneString(value.Details),
		})
	}
	for _, value := range encoded.GetLabels() {
		if value == nil {
			return boardcopy.CopySnapshot{}, errors.New("label is required")
		}
		snapshot.Labels = append(snapshot.Labels, boardcopy.CopyLabel{
			IssueID: value.GetIssueId(), Label: value.GetValue(),
		})
	}
	for _, value := range encoded.GetDependencies() {
		if value == nil {
			return boardcopy.CopySnapshot{}, errors.New("dependency is required")
		}
		snapshot.Dependencies = append(snapshot.Dependencies, boardcopy.CopyDependency{
			IssueID: value.GetIssueId(), PrerequisiteID: value.GetPrerequisiteId(),
		})
	}
	for _, value := range encoded.GetContainment() {
		if value == nil {
			return boardcopy.CopySnapshot{}, errors.New("containment edge is required")
		}
		snapshot.Containment = append(snapshot.Containment, boardcopy.CopyContainment{
			ChildID: value.GetChildId(), ParentID: value.GetParentId(),
		})
	}
	for _, value := range encoded.GetExternalKeys() {
		if value == nil {
			return boardcopy.CopySnapshot{}, errors.New("external key is required")
		}
		snapshot.ExternalKeys = append(snapshot.ExternalKeys, boardcopy.CopyExternalKey{
			Key: value.GetKey(), IssueID: value.GetIssueId(),
		})
	}
	for _, value := range encoded.GetLogEntries() {
		if value == nil {
			return boardcopy.CopySnapshot{}, errors.New("log entry is required")
		}
		createdAt, err := optionalTimestampFromProto("Log entry creation time", value.GetCreatedAt())
		if err != nil {
			return boardcopy.CopySnapshot{}, fmt.Errorf("log entry %q: %w", value.GetId(), err)
		}
		snapshot.LogEntries = append(snapshot.LogEntries, boardcopy.CopyLogEntry{
			Order: value.GetOrder(), ID: value.GetId(), IssueID: value.GetIssueId(),
			Kind: value.GetKind(), Author: cloneString(value.Author),
			Committer: cloneString(value.Committer), Body: value.GetBody(),
			CreatedAt: createdAt, NextAction: cloneString(value.NextAction),
		})
	}
	for _, value := range encoded.GetStates() {
		if value == nil {
			return boardcopy.CopySnapshot{}, errors.New("state is required")
		}
		updatedAt, err := optionalTimestampFromProto("state update time", value.GetUpdatedAt())
		if err != nil {
			return boardcopy.CopySnapshot{}, fmt.Errorf("state for issue %q: %w", value.GetIssueId(), err)
		}
		snapshot.States = append(snapshot.States, boardcopy.CopyState{
			IssueID: value.GetIssueId(), Body: value.GetBody(),
			Author: cloneString(value.Author), UpdatedAt: updatedAt,
			SnapshotLogEntryID: cloneString(value.SnapshotLogEntryId),
			NextAction:         cloneString(value.NextAction),
		})
	}
	for _, value := range encoded.GetResults() {
		if value == nil {
			return boardcopy.CopySnapshot{}, errors.New("result is required")
		}
		snapshot.Results = append(snapshot.Results, boardcopy.CopyResultRecord{
			IssueID: value.GetIssueId(), Body: value.GetBody(),
		})
	}
	for _, value := range encoded.GetCheckpoints() {
		if value == nil {
			return boardcopy.CopySnapshot{}, errors.New("checkpoint is required")
		}
		decidedAt, err := timestampFromProto("checkpoint decision time", value.GetDecidedAt())
		if err != nil {
			return boardcopy.CopySnapshot{}, fmt.Errorf("checkpoint %q: %w", value.GetIssueId(), err)
		}
		snapshot.Checkpoints = append(snapshot.Checkpoints, boardcopy.CopyCheckpoint{
			IssueID: value.GetIssueId(), Outcome: value.GetOutcome(),
			Reason: value.GetReason(), DecidedAt: decidedAt,
		})
	}
	for _, value := range encoded.GetAttachments() {
		if value == nil || value.GetBlob() == nil {
			return boardcopy.CopySnapshot{}, errors.New("attachment and blob are required")
		}
		digest, err := attachment.NewDigest(value.GetBlob().GetDigest())
		if err != nil {
			return boardcopy.CopySnapshot{}, fmt.Errorf("attachment %q: %w", value.GetId(), err)
		}
		descriptor := attachment.BlobDescriptor{
			Digest: digest, SizeBytes: value.GetBlob().GetSizeBytes(),
		}
		if err := descriptor.Validate(); err != nil {
			return boardcopy.CopySnapshot{}, fmt.Errorf("attachment %q: %w", value.GetId(), err)
		}
		createdAt, err := timestampFromProto("attachment creation time", value.GetCreatedAt())
		if err != nil {
			return boardcopy.CopySnapshot{}, fmt.Errorf("attachment %q: %w", value.GetId(), err)
		}
		removedAt, err := optionalTimestampFromProto("attachment removal time", value.GetRemovedAt())
		if err != nil {
			return boardcopy.CopySnapshot{}, fmt.Errorf("attachment %q: %w", value.GetId(), err)
		}
		snapshot.Attachments = append(snapshot.Attachments, boardcopy.CopyAttachment{
			ID: value.GetId(), OriginIssueID: cloneString(value.OriginIssueId),
			Blob: descriptor, Filename: value.GetFilename(),
			MediaType: value.GetMediaType(), Lifecycle: value.GetLifecycle(),
			CreatedActor: value.GetCreatedActor(), CreatedAt: createdAt,
			RemovedActor: cloneString(value.RemovedActor), RemovedAt: removedAt,
		})
	}
	return snapshot, nil
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
	return configuration.Configuration{
		Issue: configuration.IssueConfiguration{
			ID: configuration.IssueIDConfiguration{
				Prefix: prefix, Strategy: strategy,
			},
			Summary: configuration.SummaryConfiguration{MaxBytes: summaryMaxBytes},
		},
		Attachment: configuration.AttachmentConfiguration{MaxBytes: attachmentMaxBytes},
	}, nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	return new(*value)
}

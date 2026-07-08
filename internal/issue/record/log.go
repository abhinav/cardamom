package record

import (
	"context"
	"time"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// AddLogEntry appends one immutable attributed log entry.
type AddLogEntry struct {
	// IssueID identifies the issue receiving the log entry.
	IssueID issue.ID
	// Author is the normalized log entry author.
	Author issue.Actor
	// Body is the immutable Markdown log entry source.
	Body string
}

// LogEntry is an immutable attributed record prepared for persistence.
type LogEntry struct {
	// ID is assigned by persistence before insertion.
	ID issue.LogID
	// IssueID identifies the issue receiving the log entry.
	IssueID issue.ID
	// Kind selects the finite immutable payload contract.
	Kind issue.LogEntryKind
	// Author is empty when the event has no body attribution.
	Author issue.Actor
	// Committer is empty when the preserving actor is unknown.
	Committer issue.Actor
	// Body is the immutable Markdown log entry source.
	Body string
	// NextAction is the transition preserved with a State snapshot.
	NextAction string
	// Created is absent when the event has no recorded time.
	Created *time.Time
}

// LogEntryAdded is the semantic outcome of appending one log entry.
type LogEntryAdded struct {
	// LogEntry is the prepared log entry. Persistence assigns its durable ID.
	LogEntry LogEntry
	// CommittedRevision is populated after persistence publishes the change.
	CommittedRevision
}

// AddLogEntryResult reports the committed caller-facing log entry.
type AddLogEntryResult struct {
	// LogEntry is the persisted caller-facing projection.
	LogEntry issue.LogEntry
}

// AddLogEntryRequest supplies caller input for one log entry.
type AddLogEntryRequest struct {
	// IssueID identifies the issue receiving the log entry.
	IssueID string
	// Body is the immutable Markdown log entry source.
	Body string
}

// AddLogEntry applies log entry policy to the loaded snapshot.
func (p *Policy) AddLogEntry(command AddLogEntry) (LogEntryAdded, error) {
	if command.IssueID != p.snapshot.Issue.ID() {
		return LogEntryAdded{}, ErrIncompleteSnapshot
	}
	if command.Author == "" {
		return LogEntryAdded{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: log entry author required",
		)
	}
	if command.Body == "" {
		return LogEntryAdded{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: log entry body required",
		)
	}
	entry := LogEntry{
		IssueID:    command.IssueID,
		Kind:       issue.LogEntryKindPost,
		Author:     command.Author,
		Committer:  command.Author,
		Body:       command.Body,
		NextAction: "",
		Created:    new(p.snapshot.OccurredAt),
	}
	return LogEntryAdded{LogEntry: entry}, nil
}

// AddLogEntry validates caller input and appends one log entry.
func (r *Recorder) AddLogEntry(
	ctx context.Context,
	invocation issue.Invocation,
	request AddLogEntryRequest,
) (AddLogEntryResult, error) {
	id, err := issue.NewID(request.IssueID)
	if err != nil {
		return AddLogEntryResult{}, err
	}
	outcome, err := r.changes.AddLogEntry(
		ctx,
		AddLogEntry{IssueID: id, Author: issue.NewActor(invocation.Actor()), Body: request.Body},
	)
	if err != nil {
		return AddLogEntryResult{}, err
	}
	return AddLogEntryResult{LogEntry: logEntryProjection(outcome.LogEntry)}, nil
}

func logEntryProjection(entry LogEntry) issue.LogEntry {
	return issue.LogEntry{
		ID:         entry.ID,
		IssueID:    entry.IssueID.String(),
		Kind:       entry.Kind.String(),
		Author:     optionalActor(entry.Author),
		Committer:  optionalActor(entry.Committer),
		Body:       entry.Body,
		NextAction: optionalText(entry.NextAction),
		Created:    optionalUnix(entry.Created),
	}
}

func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return new(value)
}

func optionalActor(actor issue.Actor) *string {
	if actor == "" {
		return nil
	}
	return new(actor.String())
}

func optionalUnix(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	return new(value.Unix())
}

package board

import (
	"context"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
)

// PinRepository persists one board's ordered pinned-issue collection.
type PinRepository interface {
	// ListPins returns current issue references in insertion order.
	ListPins(context.Context) ([]issue.Reference, error)

	// PinIssue atomically admits one issue under maxCount.
	PinIssue(context.Context, issue.ID, PinLimit) (PinMutation, error)

	// UnpinIssue atomically removes one issue when present.
	UnpinIssue(context.Context, issue.ID) (PinMutation, error)
}

// PinConfiguration resolves the current effective policy for one board.
type PinConfiguration interface {
	// ResolveBoardPinLimit returns the freshly resolved admission limit.
	ResolveBoardPinLimit(context.Context, ID) (PinLimit, error)
}

// PinsConfig supplies the dependencies for one board's pin operations.
type PinsConfig struct {
	// BoardID identifies the board whose pin collection is owned.
	BoardID ID // required

	// Repository persists the ordered collection.
	Repository PinRepository // required

	// Configuration supplies the effective admission limit per invocation.
	Configuration PinConfiguration // required
}

// Pins owns finite pinned-issue operations for one board.
type Pins struct {
	boardID       ID
	repository    PinRepository
	configuration PinConfiguration
}

// NewPins constructs board pin operations.
func NewPins(cfg PinsConfig) *Pins {
	_, err := NewID(cfg.BoardID.String())
	must.NotErrorf(err, "board pin BoardID must be valid")
	must.NotBeNilf(cfg.Repository, "board pin Repository is required")
	must.NotBeNilf(cfg.Configuration, "board pin Configuration is required")
	return &Pins{
		boardID: cfg.BoardID, repository: cfg.Repository,
		configuration: cfg.Configuration,
	}
}

// List returns current issue references in insertion order.
func (p *Pins) List(ctx context.Context) ([]issue.Reference, error) {
	return p.repository.ListPins(ctx)
}

// Pin adds one issue to the end of the collection when absent.
func (p *Pins) Pin(
	ctx context.Context,
	_ Invocation,
	issueID string,
) (PinMutation, error) {
	id, err := issue.NewID(issueID)
	if err != nil {
		return PinMutation{}, err
	}
	limit, err := p.configuration.ResolveBoardPinLimit(ctx, p.boardID)
	if err != nil {
		return PinMutation{}, err
	}
	return p.repository.PinIssue(ctx, id, limit)
}

// Unpin removes one issue from the collection when present.
func (p *Pins) Unpin(
	ctx context.Context,
	_ Invocation,
	issueID string,
) (PinMutation, error) {
	id, err := issue.NewID(issueID)
	if err != nil {
		return PinMutation{}, err
	}
	return p.repository.UnpinIssue(ctx, id)
}

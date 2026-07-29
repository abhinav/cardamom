package dump

import (
	"context"
	"errors"
	"fmt"
)

// Renderer produces one complete deterministic dump artifact.
type Renderer interface {
	// Render selects and renders one coherent artifact.
	Render(context.Context, RenderRequest) (RenderedDump, error)
}

// Publisher commits one complete rendered dump to its destination.
type Publisher interface {
	// Publish commits the complete rendered dump to its destination.
	Publish(context.Context, Publication) (PublicationResult, error)
}

// PublicationService owns local dump publication from rendering through atomic
// filesystem replacement.
type PublicationService struct {
	renderer  Renderer
	publisher Publisher
}

// NewPublicationService constructs the local publication operation.
func NewPublicationService(
	renderer Renderer,
	publisher Publisher,
) (*PublicationService, error) {
	if renderer == nil {
		return nil, errors.New("dump renderer is required")
	}
	if publisher == nil {
		return nil, errors.New("dump publisher is required")
	}
	return &PublicationService{renderer: renderer, publisher: publisher}, nil
}

// Execute renders and publishes one coherent dump.
func (s *PublicationService) Execute(
	ctx context.Context,
	request Request,
) (ExecutionResult, error) {
	rendered, err := s.renderer.Render(ctx, RenderRequest{Selection: request.Selection})
	if err != nil {
		return ExecutionResult{}, err
	}
	published, err := s.publisher.Publish(ctx, Publication{
		Destination: request.Destination,
		Rendered:    rendered,
		Force:       request.Force,
	})
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("publish dump: %w", err)
	}
	return ExecutionResult{
		Destination: request.Destination,
		BoardID:     rendered.Provenance.BoardID,
		Revision:    rendered.Revision,
		Selection:   rendered.Selection,
		Issues:      rendered.IssueCount,
		Written:     published.Written,
		Unchanged:   published.Unchanged,
		Removed:     published.Removed,
	}, nil
}

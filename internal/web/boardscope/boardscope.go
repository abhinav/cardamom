// Package boardscope resolves protocol board scopes to domain board identities.
package boardscope

import (
	"context"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
)

// Catalog supplies the board reads needed to resolve protocol scopes.
type Catalog interface {
	// Get returns the current board with the requested stable identity.
	Get(context.Context, board.ID) (*board.State, error)

	// List returns every current board in catalog order.
	List(context.Context) ([]*board.State, error)
}

// IssueLocator finds the board that owns a store-global issue identity.
type IssueLocator interface {
	// BoardForIssue returns the board that owns issueID.
	BoardForIssue(context.Context, string) (board.ID, error)
}

// Resolver validates protocol scopes and resolves issue ownership without
// opening board repositories or command services.
type Resolver struct {
	catalog Catalog
	issues  IssueLocator
}

// New constructs a board-scope resolver from catalog and issue ownership
// collaborators.
func New(catalog Catalog, issues IssueLocator) *Resolver {
	must.NotBeNilf(catalog, "boardscope: catalog is required")
	must.NotBeNilf(issues, "boardscope: issue locator is required")
	return &Resolver{catalog: catalog, issues: issues}
}

// Boards validates scope and returns its current catalog boards.
func (r *Resolver) Boards(ctx context.Context, scope *privatev1.BoardScope) ([]*board.State, error) {
	if scope == nil || scope.Selection == nil {
		return nil, errkind.Errorf(errkind.InvalidInput, "invalid input: board scope is required")
	}
	switch selection := scope.Selection.(type) {
	case *privatev1.BoardScope_BoardId:
		boardID, err := board.NewID(selection.BoardId)
		if err != nil {
			return nil, err
		}
		state, err := r.catalog.Get(ctx, boardID)
		if err != nil {
			return nil, err
		}
		return []*board.State{state}, nil
	case *privatev1.BoardScope_AllBoards:
		if selection.AllBoards == nil {
			return nil, errkind.Errorf(errkind.InvalidInput, "invalid input: all-boards scope is required")
		}
		return r.catalog.List(ctx)
	default:
		return nil, errkind.Errorf(errkind.InvalidInput, "invalid input: unknown board scope")
	}
}

// BoardForIssue validates issueID and returns its current catalog board.
func (r *Resolver) BoardForIssue(ctx context.Context, issueID string) (*board.State, error) {
	if _, err := issue.NewID(issueID); err != nil {
		return nil, err
	}
	boardID, err := r.issues.BoardForIssue(ctx, issueID)
	if err != nil {
		return nil, err
	}
	return r.catalog.Get(ctx, boardID)
}

// RequireIssueBoard verifies that an issue belongs to the selected board.
func (r *Resolver) RequireIssueBoard(
	ctx context.Context,
	issueID string,
	boardID board.ID,
) error {
	if _, err := issue.NewID(issueID); err != nil {
		return err
	}
	owner, err := r.issues.BoardForIssue(ctx, issueID)
	if err != nil {
		return err
	}
	if owner != boardID {
		return errkind.Errorf(
			errkind.Conflict,
			"issue belongs to another board: issue %q belongs to board %q",
			issueID,
			owner,
		)
	}
	return nil
}

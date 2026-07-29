// Package selection resolves explicit, issue-derived, and checkout-bound
// board scope for one process invocation.
//
// The package owns selection precedence and the checkout binding contract.
// Callers supply the persistence implementations for those boundaries.
package selection

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/must"
)

// ErrBindingNotFound reports that the checkout has no selected board.
var ErrBindingNotFound = errors.New("board binding not found")

// ErrIssueNotFound reports that an issue selector has no owning board.
var ErrIssueNotFound = errkind.Errorf(errkind.NotFound, "issue not found")

// Catalog supplies the board reads required for selection.
type Catalog interface {
	// List returns every board in deterministic order.
	List(context.Context) ([]*board.State, error)

	// Get returns the board with the requested stable identity.
	Get(context.Context, board.ID) (*board.State, error)

	// Sole returns the store's only board or an ambiguity error.
	Sole(context.Context) (*board.State, error)
}

// Binding persists the board selected for one local checkout.
type Binding interface {
	// Read returns the board selected for the checkout.
	// It returns ErrBindingNotFound when no board is bound.
	Read() (board.ID, error)

	// Write replaces the board selected for the checkout.
	Write(board.ID) error
}

// IssueLocator finds the board that owns a store-global issue identity.
type IssueLocator interface {
	// BoardForIssue returns the board that owns issueID.
	// It returns ErrIssueNotFound when issueID does not exist.
	BoardForIssue(context.Context, string) (board.ID, error)
}

// Resolver selects boards from explicit, issue-derived, and checkout context.
type Resolver struct {
	// catalog supplies board namespace reads.
	catalog Catalog

	// binding persists checkout-local board selection.
	binding Binding

	// issues resolves store-global issue identities to boards.
	issues IssueLocator
}

// NewResolver constructs a board resolver from its selection boundaries.
func NewResolver(catalog Catalog, binding Binding, issues IssueLocator) *Resolver {
	must.NotBeNilf(catalog, "selection Catalog is required")
	must.NotBeNilf(binding, "selection Binding is required")
	must.NotBeNilf(issues, "selection IssueLocator is required")
	return &Resolver{
		catalog: catalog,
		binding: binding,
		issues:  issues,
	}
}

// Request describes explicit, ambient, or issue-derived board selection.
type Request struct {
	// Selector is the explicit flag or environment selection, when present.
	Selector *board.Selector

	// IssueIDs infer one owning board and bypass the checkout binding.
	IssueIDs []string
}

// Resolve applies issue-derived, explicit, binding, and sole-board rules.
func (r *Resolver) Resolve(ctx context.Context, request Request) (*board.State, error) {
	if len(request.IssueIDs) > 0 {
		return r.resolveIssueBoard(ctx, request)
	}
	if request.Selector != nil {
		return r.resolveExplicit(ctx, *request.Selector)
	}
	boardID, err := r.binding.Read()
	if err == nil {
		return r.catalog.Get(ctx, boardID)
	}
	if !errors.Is(err, ErrBindingNotFound) {
		return nil, err
	}
	return r.catalog.Sole(ctx)
}

// Use resolves selector and persists it for the current checkout.
func (r *Resolver) Use(ctx context.Context, selector board.Selector) (*board.State, error) {
	state, err := r.resolveExplicit(ctx, selector)
	if err != nil {
		return nil, err
	}
	if err := r.binding.Write(state.ID()); err != nil {
		return nil, err
	}
	return state, nil
}

// resolveExplicit selects one board by stable identity or exact name.
func (r *Resolver) resolveExplicit(
	ctx context.Context,
	selector board.Selector,
) (*board.State, error) {
	boards, err := r.catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	var matches []*board.State
	for _, candidate := range boards {
		if selector.Matches(candidate) {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return nil, errkind.Errorf(errkind.NotFound, "board %q not found", selector.String())
	case 1:
		return matches[0], nil
	default:
		return nil, errkind.Errorf(
			errkind.Conflict,
			"board %q is ambiguous; use an ID",
			selector.String(),
		)
	}
}

// resolveIssueBoard selects the common owning board for explicit issue IDs.
func (r *Resolver) resolveIssueBoard(ctx context.Context, request Request) (*board.State, error) {
	firstIssue := request.IssueIDs[0]
	boardID, err := r.issues.BoardForIssue(ctx, firstIssue)
	if err != nil {
		if errors.Is(err, ErrIssueNotFound) {
			return nil, ErrIssueNotFound
		}
		return nil, err
	}
	for _, issueID := range request.IssueIDs[1:] {
		owner, err := r.issues.BoardForIssue(ctx, issueID)
		if err != nil {
			if errors.Is(err, ErrIssueNotFound) {
				return nil, ErrIssueNotFound
			}
			return nil, err
		}
		if owner != boardID {
			return nil, errkind.Errorf(
				errkind.Conflict,
				"issue IDs belong to multiple boards: %q and %q",
				boardID,
				owner,
			)
		}
	}
	if request.Selector != nil {
		selected, err := r.resolveExplicit(ctx, *request.Selector)
		if err != nil {
			return nil, err
		}
		if selected.ID() != boardID {
			return nil, errkind.Errorf(
				errkind.Conflict,
				"issue %q belongs to board %q, not selected board %q",
				firstIssue,
				boardID,
				selected.ID(),
			)
		}
	}
	return r.catalog.Get(ctx, boardID)
}

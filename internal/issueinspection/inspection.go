// Package issueinspection opens public issue references for CLI and browser
// reads. A retired ID returns its candidates without selecting an owning
// board. Ordinary IDs and producer keys delegate to the caller's scoped issue
// reader, which decides whether the view includes attachment associations.
package issueinspection

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

// Inspector distinguishes retired references from ordinary issue reads.
// It never selects a candidate on the caller's behalf.
type Inspector struct {
	// Markers reads the store-wide retired-ID namespace.
	Markers issue.MarkerReader // required

	// Issues opens ordinary issue reads only after marker lookup completes.
	Issues Reader // required
}

// Result contains either an ordinary issue or an ambiguous reference marker.
// Successful inspection sets one pointer; neither is populated on error.
type Result struct {
	// Issue is present for an ordinary issue ID or producer key.
	Issue *LocatedIssue

	// Marker is present when the public ID was permanently retired.
	Marker *issue.ReferenceMarker
}

// ReadIssue returns collision candidates for retired IDs and delegates all
// other reads. Producer keys retain their ordinary board-scoped meaning even
// when their spelling resembles a retired public ID.
func (i *Inspector) ReadIssue(ctx context.Context, request issue.ReadRequest) (Result, error) {
	if request.Key == "" {
		id, err := issue.NewID(request.IssueID)
		if err != nil {
			return Result{}, err
		}
		marker, err := i.Markers.ReadMarker(ctx, id)
		if err != nil {
			return Result{}, fmt.Errorf("inspect issue reference: %w", err)
		}
		if marker != nil {
			return Result{Marker: marker}, nil
		}
	}
	view, err := i.Issues.ReadIssue(ctx, request)
	if err != nil {
		return Result{}, err
	}
	return Result{Issue: &view}, nil
}

// Reader supplies a located issue through the caller's normal scope rules.
type Reader interface {
	// ReadIssue returns the issue or a selection/read error.
	ReadIssue(context.Context, issue.ReadRequest) (LocatedIssue, error)
}

// LocatedIssue combines an ordinary issue view with its logical board.
type LocatedIssue struct {
	// BoardID identifies the board that owns View.
	BoardID board.ID // required

	// View contains the requested issue and any requested attachments.
	View attachment.IssueView // required
}

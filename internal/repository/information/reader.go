// Package information composes store and board information from one retained
// persistence snapshot.
package information

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/information"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/repository/board"
	"go.abhg.dev/cardamom/internal/repository/store"
)

var _ information.Reader = (*Reader)(nil)

// Reader assembles store-owned and board-owned information through one read
// snapshot.
type Reader struct {
	store *store.Store   // required
	board boardInventory // required
}

// boardInventory is the board-owned read capability used by Reader.
type boardInventory interface {
	ReadIssueInventory(
		context.Context,
		*store.View,
	) (board.IssueInventory, error)
}

// New binds the information reader to one store and board repository.
func New(persistence *store.Store, boardRepository *board.Repository) *Reader {
	must.NotBeNilf(persistence, "information Store is required")
	must.NotBeNilf(boardRepository, "information board repository is required")
	return &Reader{store: persistence, board: boardRepository}
}

// Read returns schema, revision, and issue inventory from one retained read
// snapshot.
func (r *Reader) Read(
	ctx context.Context,
) (out information.Snapshot, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	storeInformation, err := view.ReadInformation(ctx)
	if err != nil {
		return out, err
	}
	boardInventory, err := r.board.ReadIssueInventory(ctx, view)
	if err != nil {
		return out, err
	}
	statusCounts := make(
		[]information.IssueStatusCount,
		len(boardInventory.ByStatus),
	)
	for index, count := range boardInventory.ByStatus {
		statusCounts[index] = information.IssueStatusCount{
			Status: count.Status,
			Count:  count.Count,
		}
	}
	return information.Snapshot{
		Schema: information.Schema{
			DatabaseVersion: storeInformation.DatabaseSchemaVersion,
			CodeVersion:     storeInformation.CodeSchemaVersion,
		},
		Revision: information.Revision{Current: storeInformation.Revision},
		Issues: information.IssueInventory{
			Total: boardInventory.Total, ByStatus: statusCounts,
		},
	}, nil
}

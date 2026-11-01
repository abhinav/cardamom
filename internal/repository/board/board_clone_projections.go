package board

import (
	"context"

	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// cloneIssueProjections preserves absent current fields directly. Portable
// backup records require resolved values and cannot express this projection;
// clone carries the identities, history, and conflicts that explain absence.
func cloneIssueProjections(ctx context.Context, reader, writer *query.Queries, importer *copyRecordImporter) error {
	var after string
	for {
		rows, err := reader.BoardListCopyIssuePage(ctx, query.BoardListCopyIssuePageParams{
			BoardID: importer.boardID, AfterID: after, PageSize: copyRecordPageSize,
		})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			uid := importer.issueUIDs[row.ID]
			if err := writer.BoardInsertCopiedIssue(ctx, query.BoardInsertCopiedIssueParams{
				Uid: uid.Bytes(), ID: row.ID, BoardID: importer.boardID,
				Title: row.Title, Kind: row.Kind, Lifecycle: row.Lifecycle, Priority: row.Priority,
				CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ClosedAt: row.ClosedAt,
				WaitingReason: row.WaitingReason, WaitingSince: row.WaitingSince,
				Summary: row.Summary, Details: row.Details, Revision: importer.lastRevision,
			}); err != nil {
				return err
			}
			after = row.ID
		}
	}
}

// clonePinProjections retains positions because journal assignments refer to
// them. Backup pin records instead describe a contiguous export order.
func clonePinProjections(ctx context.Context, reader, writer *query.Queries, boardID string) error {
	var afterPosition int64
	var afterUID []byte
	for {
		rows, err := reader.BoardListCopyPinPage(ctx, query.BoardListCopyPinPageParams{
			BoardID: boardID, AfterPosition: afterPosition, AfterIssueUid: afterUID,
			PageSize: copyRecordPageSize,
		})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			if err := writer.BoardInsertPinAtPosition(ctx, query.BoardInsertPinAtPositionParams{
				BoardID: boardID, IssueUid: row.IssueUid, Position: row.Position,
			}); err != nil {
				return err
			}
			afterPosition, afterUID = row.Position, row.IssueUid
		}
	}
}

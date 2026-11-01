package board

import (
	"context"

	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// cloneConflicts transfers compact projection metadata unchanged. CloneV1
// retains the journal bodies and event identities referenced by this metadata.
func cloneConflicts(ctx context.Context, reader, writer *query.Queries, boardID string, revision int64) error {
	var after string
	for {
		rows, err := reader.BoardListConflicts(ctx, query.BoardListConflictsParams{BoardID: boardID, AfterID: after, PageLimit: copyRecordPageSize})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			if err := writer.BoardInsertConflict(ctx, query.BoardInsertConflictParams{ID: row.ID, BoardID: boardID, Kind: row.Kind, Category: row.Category, IssueUid: row.IssueUid, RelatedUid: row.RelatedUid, MemberKey: row.MemberKey, Revision: revision}); err != nil {
				return err
			}
			alternatives, err := reader.BoardReadConflictAlternatives(ctx, row.ID)
			if err != nil {
				return err
			}
			for _, value := range alternatives {
				if err := writer.BoardInsertConflictAlternative(ctx, query.BoardInsertConflictAlternativeParams{ConflictID: row.ID, AlternativeID: value.AlternativeID, Kind: value.Kind, IssueUid: value.IssueUid, RelatedUid: value.RelatedUid, MemberKey: value.MemberKey, WaitingPresent: value.WaitingPresent}); err != nil {
					return err
				}
			}
			origins, err := reader.BoardReadConflictOrigins(ctx, row.ID)
			if err != nil {
				return err
			}
			for _, origin := range origins {
				if err := writer.BoardInsertConflictOrigin(ctx, query.BoardInsertConflictOriginParams{ConflictID: row.ID, AlternativeID: origin.AlternativeID, WriterUid: origin.WriterUid, Sequence: origin.Sequence, Ordinal: origin.Ordinal}); err != nil {
					return err
				}
			}
			affected, err := reader.BoardReadConflictAffectedIssues(ctx, row.ID)
			if err != nil {
				return err
			}
			for _, value := range affected {
				if err := writer.BoardInsertConflictIssue(ctx, query.BoardInsertConflictIssueParams{ConflictID: row.ID, IssueUid: value.IssueUid, Field: value.Field}); err != nil {
					return err
				}
			}
			after = row.ID
		}
	}
}

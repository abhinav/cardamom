package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

func verifyBoardMutationHistory(ctx context.Context, queries *query.Queries) error {
	missingHistory, err := queries.StoreCountBoardsWithoutMutationHistory(ctx)
	if err != nil {
		return fmt.Errorf("verify board mutation history: find boards without history: %w", err)
	}
	if missingHistory != 0 {
		return fmt.Errorf(
			"verify board mutation history: found %d boards without history",
			missingHistory,
		)
	}
	invalidTopology, err := queries.StoreCountInvalidMutationTopology(ctx)
	if err != nil {
		return fmt.Errorf("verify board mutation history topology: %w", err)
	}
	if invalidTopology != 0 {
		return fmt.Errorf(
			"verify board mutation history: found %d invalid chain rows",
			invalidTopology,
		)
	}
	return verifyBoardMutationPayloads(ctx, queries)
}

func verifyBoardMutationPayloads(ctx context.Context, queries *query.Queries) error {
	var afterBoardID string
	var afterWriterUID []byte
	var afterSequence int64
	for {
		row, err := queries.StoreNextBoardMutationCommit(
			ctx,
			query.StoreNextBoardMutationCommitParams{
				AfterBoardID:   afterBoardID,
				AfterWriterUid: afterWriterUID,
				AfterSequence:  afterSequence,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("verify board mutation history: read marker: %w", err)
		}
		if err := verifyBoardMutation(ctx, queries, row); err != nil {
			return fmt.Errorf(
				"verify board mutation history for board %q writer %x sequence %d: %w",
				row.BoardID,
				row.WriterUid,
				row.Sequence,
				err,
			)
		}
		afterBoardID = row.BoardID
		afterWriterUID = append(afterWriterUID[:0], row.WriterUid...)
		afterSequence = row.Sequence
	}
}

func verifyBoardMutation(
	ctx context.Context,
	queries *query.Queries,
	row query.BoardMutationCommit,
) error {
	// The canonical marker bytes are the authoritative observed frontier.
	// DecodeMarker validates ordering, uniqueness, and the writer predecessor.
	marker, err := boardhistory.DecodeMarker(row.MarkerBytes)
	if err != nil {
		return err
	}
	boardID, err := board.NewID(row.BoardID)
	if err != nil {
		return err
	}
	writer, err := board.ParseWriterUID(row.WriterUid)
	if err != nil {
		return err
	}
	previous, err := boardhistory.ParseDigest(row.PreviousHash)
	if err != nil {
		return err
	}
	payloadHash, err := boardhistory.ParseDigest(row.PayloadHash)
	if err != nil {
		return err
	}
	markerHash, err := boardhistory.ParseDigest(row.MarkerHash)
	if err != nil {
		return err
	}
	header := marker.Header()
	if header.Board != boardID ||
		header.Writer != writer ||
		header.Sequence != uint64(row.Sequence) || header.Previous != previous ||
		marker.PartCount() != row.PartCount ||
		marker.EventCount() != uint64(row.EventCount) ||
		marker.PayloadHash() != payloadHash || marker.Hash() != markerHash {
		return errors.New("marker columns do not match retained marker bytes")
	}
	for _, tip := range header.Frontier {
		count, err := queries.StoreCountBoardMutationFrontierCommit(
			ctx,
			query.StoreCountBoardMutationFrontierCommitParams{
				BoardID: row.BoardID, WriterUid: tip.Writer.Bytes(),
				Sequence: int64(tip.Sequence), MarkerHash: tip.Hash.Bytes(),
			},
		)
		if err != nil {
			return fmt.Errorf("verify observed frontier tip: %w", err)
		}
		if count != 1 {
			return errors.New("observed frontier tip does not identify retained history")
		}
	}
	// Blob references protect CAS content from collection. Rebuild the expected
	// set from canonical events instead of trusting that derived index.
	expectedBlobSizes := make(map[string]int64)
	for retained, err := range boardhistory.MutationEvents(
		marker,
		func(partIndex int64) ([]byte, error) {
			return queries.StoreGetBoardMutationPart(
				ctx,
				query.StoreGetBoardMutationPartParams{
					BoardID:   row.BoardID,
					WriterUid: row.WriterUid,
					Sequence:  row.Sequence,
					PartIndex: partIndex,
				},
			)
		},
	) {
		if err != nil {
			return err
		}
		if err := retainExpectedMutationBlob(expectedBlobSizes, retained.Event); err != nil {
			return fmt.Errorf("retain mutation event %d blob reference: %w", retained.ID.Ordinal, err)
		}
	}
	return verifyBoardMutationBlobReferences(ctx, queries, row, expectedBlobSizes)
}

func retainExpectedMutationBlob(
	expected map[string]int64,
	event boardhistory.Event,
) error {
	var digest string
	var sizeBytes int64
	switch value := event.Payload.(type) {
	case *boardhistory.AttachmentAdd:
		digest, sizeBytes = value.Digest, value.SizeBytes
	default:
		return nil
	}
	if retainedSize, found := expected[digest]; found && retainedSize != sizeBytes {
		return errors.New("attachment digest has conflicting sizes")
	}
	expected[digest] = sizeBytes
	return nil
}

func verifyBoardMutationBlobReferences(
	ctx context.Context,
	queries *query.Queries,
	row query.BoardMutationCommit,
	expected map[string]int64,
) error {
	references, err := queries.StoreListBoardMutationBlobReferences(
		ctx,
		query.StoreListBoardMutationBlobReferencesParams{
			BoardID: row.BoardID, WriterUid: row.WriterUid, Sequence: row.Sequence,
		},
	)
	if err != nil {
		return fmt.Errorf("read attachment blob references: %w", err)
	}
	for _, reference := range references {
		expectedSize, found := expected[reference.Digest]
		if !found {
			return fmt.Errorf(
				"attachment blob reference %q has no attachment add event",
				reference.Digest,
			)
		}
		if reference.SizeBytes != expectedSize {
			return fmt.Errorf(
				"attachment blob reference %q has size %d; want %d",
				reference.Digest,
				reference.SizeBytes,
				expectedSize,
			)
		}
		delete(expected, reference.Digest)
	}
	missing := slices.Sorted(maps.Keys(expected))
	if len(missing) != 0 {
		return fmt.Errorf("attachment blob reference %q is missing", missing[0])
	}
	return nil
}

package board

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/markdown"
	"go.abhg.dev/cardamom/internal/markdown/reference"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// CopyRepositoryConfig supplies destination publication dependencies.
type CopyRepositoryConfig struct {
	Clock   Clock
	Entropy io.Reader
}

// CopyRepository owns atomic destination metadata publication for board copy.
type CopyRepository struct {
	store   *store.Store
	clock   Clock
	entropy io.Reader
}

// NewCopyRepository constructs destination board-copy persistence.
func NewCopyRepository(
	persistence *store.Store,
	cfg CopyRepositoryConfig,
) (*CopyRepository, error) {
	if persistence == nil {
		return nil, errors.New("board copy store is required")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	entropy := cfg.Entropy
	if entropy == nil {
		entropy = rand.Reader
	}
	return &CopyRepository{
		store: persistence, clock: clock, entropy: entropy,
	}, nil
}

// ReadCopyReceipt returns persisted publication facts when present.
func (r *CopyRepository) ReadCopyReceipt(
	ctx context.Context,
	key boardcopy.CopyReceiptKey,
) (receipt boardcopy.CopyReceipt, found bool, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return receipt, false, fmt.Errorf("begin destination receipt read: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	return r.readCopyReceipt(ctx, query.New(view), key)
}

// ImportCopyRecords atomically creates one destination board and receipt from
// an incremental record sequence.
func (r *CopyRepository) ImportCopyRecords(
	ctx context.Context,
	index boardcopy.RecordIndex,
	records boardcopy.RecordSequence,
	options boardcopy.CopyOptions,
) (result boardcopy.CopyImportResult, err error) {
	return r.importCopyRecords(ctx, index, records, options)
}

func (r *CopyRepository) importCopyRecords(
	ctx context.Context,
	index boardcopy.RecordIndex,
	records boardcopy.RecordSequence,
	options boardcopy.CopyOptions,
) (result boardcopy.CopyImportResult, err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return result, fmt.Errorf("begin destination board copy: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	queries := query.New(change)

	name := index.Header.Board.Name
	if options.Name != nil {
		name = *options.Name
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return result, errkind.Errorf(
			errkind.InvalidInput,
			"invalid project namespace: board name required",
		)
	}
	receipt, found, err := r.readCopyReceipt(
		ctx,
		queries,
		boardcopy.CopyReceiptKey{
			SourceLineageID: index.Header.SourceLineageID,
			SourceBoardID:   index.Header.Board.ID,
			SnapshotVersion: index.Header.Version,
		},
	)
	if err != nil {
		return result, err
	}
	if found {
		if err := change.Commit(); err != nil {
			return result, err
		}
		return boardcopy.NewConcurrentCopyImport(receipt), nil
	}
	if err := requireCopyProject(ctx, queries, options.ProjectID); err != nil {
		return result, err
	}

	nameExists, err := queries.ProjectCopyBoardNameExists(
		ctx,
		query.ProjectCopyBoardNameExistsParams{
			ProjectID: options.ProjectID,
			Name:      name,
		},
	)
	if err != nil {
		return result, fmt.Errorf("inspect destination board name: %w", err)
	}
	if nameExists {
		return result, errkind.Errorf(
			errkind.Conflict,
			"destination board name %q already exists; use --name",
			name,
		)
	}

	boardID, err := r.allocateCopyBoardID(ctx, queries, index.Header.Board.ID)
	if err != nil {
		return result, err
	}
	issueMappings, err := r.allocateCopyIssueIDs(
		ctx,
		change,
		queries,
		index.IssueIDs,
		index.Header.Configuration.Issue.ID,
	)
	if err != nil {
		return result, err
	}
	logMappings, err := r.allocateCopyLogIDs(ctx, queries, index.LogEntryIDs)
	if err != nil {
		return result, err
	}
	attachmentMappings := identityMappings(index.AttachmentIDs)

	revisionCount := int64(1)
	if index.RemovedAttachments {
		revisionCount = 2
	}
	revisions, err := change.ReserveRevisions(ctx, revisionCount)
	if err != nil {
		return result, fmt.Errorf("reserve destination revisions: %w", err)
	}

	issueIDs := mappingIndex(issueMappings)
	logIDs := mappingIndex(logMappings)
	importer := copyRecordImporter{
		ctx: ctx, queries: queries, projectID: options.ProjectID,
		name: name, boardID: boardID,
		issueIDs: issueIDs, logIDs: logIDs,
		rewrite:       copyReferenceRewriter(issueIDs, logIDs),
		firstRevision: revisions.FirstRevision(),
		lastRevision:  revisions.LastRevision(),
	}
	indexer := boardcopy.NewRecordIndexer()
	for record, recordErr := range records {
		if recordErr != nil {
			return result, recordErr
		}
		if err := indexer.Add(record); err != nil {
			return result, fmt.Errorf("validate destination board records: %w", err)
		}
		if err := importer.Import(record); err != nil {
			return result, err
		}
	}
	actual, err := indexer.Finish()
	if err != nil {
		return result, fmt.Errorf("validate destination board records: %w", err)
	}
	if !sameRecordIndex(index, actual) {
		return result, errkind.Errorf(
			errkind.Conflict,
			"source board changed while copying records",
		)
	}
	receiptID, err := insertCopyReceipt(
		ctx,
		queries,
		index,
		options.ProjectID,
		name,
		boardID,
		revisions.LastRevision(),
		r.clock.Now(),
	)
	if err != nil {
		return result, err
	}
	if err := insertCopyMappings(
		ctx,
		queries,
		receiptID,
		issueMappings,
		logMappings,
		attachmentMappings,
	); err != nil {
		return result, err
	}
	if err := advanceCopyIssueAllocator(
		ctx,
		change,
		index.Header.Configuration,
		issueMappings,
	); err != nil {
		return result, err
	}
	if err := change.PublishRevisions(ctx, revisions); err != nil {
		return result, fmt.Errorf("publish destination revisions: %w", err)
	}
	if err := change.Commit(); err != nil {
		return result, err
	}

	outcome := copyOutcome(
		index,
		options.ProjectID,
		name,
		boardID,
		revisions.LastRevision(),
		false,
		issueMappings,
		logMappings,
		attachmentMappings,
	)
	return boardcopy.NewPublishedCopyImport(outcome), nil
}

func sameRecordIndex(
	expected boardcopy.RecordIndex,
	actual boardcopy.RecordIndex,
) bool {
	expectedHeader := expected.Header
	expectedHeader.SourceRevision = 0
	actualHeader := actual.Header
	actualHeader.SourceRevision = 0
	return expected.Digest == actual.Digest &&
		reflect.DeepEqual(expectedHeader, actualHeader) &&
		expected.Counts == actual.Counts &&
		slices.Equal(expected.IssueIDs, actual.IssueIDs) &&
		slices.Equal(expected.LogEntryIDs, actual.LogEntryIDs) &&
		slices.Equal(expected.AttachmentIDs, actual.AttachmentIDs) &&
		slices.Equal(expected.Blobs, actual.Blobs) &&
		expected.RemovedAttachments == actual.RemovedAttachments
}

func requireCopyProject(
	ctx context.Context,
	queries *query.Queries,
	projectID string,
) error {
	exists, err := queries.ProjectExists(ctx, projectID)
	if err != nil {
		return fmt.Errorf("inspect destination project: %w", err)
	}
	if !exists {
		return errkind.Errorf(
			errkind.NotFound,
			"project not found: project %q",
			projectID,
		)
	}
	return nil
}

func (r *CopyRepository) readCopyReceipt(
	ctx context.Context,
	queries *query.Queries,
	key boardcopy.CopyReceiptKey,
) (boardcopy.CopyReceipt, bool, error) {
	row, err := queries.BoardGetCopyReceipt(
		ctx,
		query.BoardGetCopyReceiptParams{
			SourceLineageID: key.SourceLineageID,
			SourceBoardID:   key.SourceBoardID,
			SnapshotVersion: int64(key.SnapshotVersion),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return boardcopy.CopyReceipt{}, false, nil
	}
	if err != nil {
		return boardcopy.CopyReceipt{}, false, fmt.Errorf(
			"read destination copy receipts: %w",
			err,
		)
	}
	issueRows, err := queries.BoardListCopyIssueMappings(ctx, row.ID)
	if err != nil {
		return boardcopy.CopyReceipt{}, false, fmt.Errorf(
			"read destination issue mappings: %w",
			err,
		)
	}
	issueMappings := make([]boardcopy.CopyIdentityMap, 0, len(issueRows))
	for _, row := range issueRows {
		issueMappings = append(issueMappings, boardcopy.CopyIdentityMap{
			Source: row.SourceID, Destination: row.DestinationID,
		})
	}
	logRows, err := queries.BoardListCopyLogMappings(ctx, row.ID)
	if err != nil {
		return boardcopy.CopyReceipt{}, false, fmt.Errorf(
			"read destination Log mappings: %w",
			err,
		)
	}
	logMappings := make([]boardcopy.CopyIdentityMap, 0, len(logRows))
	for _, row := range logRows {
		logMappings = append(logMappings, boardcopy.CopyIdentityMap{
			Source: row.SourceID, Destination: row.DestinationID,
		})
	}
	attachmentRows, err := queries.BoardListCopyAttachmentMappings(
		ctx,
		row.ID,
	)
	if err != nil {
		return boardcopy.CopyReceipt{}, false, fmt.Errorf(
			"read destination attachment mappings: %w",
			err,
		)
	}
	attachmentMappings := make(
		[]boardcopy.CopyIdentityMap,
		0,
		len(attachmentRows),
	)
	for _, row := range attachmentRows {
		attachmentMappings = append(
			attachmentMappings,
			boardcopy.CopyIdentityMap{
				Source: row.SourceID, Destination: row.DestinationID,
			},
		)
	}
	return boardcopy.CopyReceipt{
		SourceLineageID:      key.SourceLineageID,
		SourceBoardID:        key.SourceBoardID,
		SourceRevision:       row.SourceRevision,
		SnapshotVersion:      key.SnapshotVersion,
		SnapshotDigest:       row.SnapshotDigest,
		DestinationProjectID: row.DestinationProjectID,
		DestinationBoardID:   row.DestinationBoardID,
		DestinationName:      row.DestinationName,
		DestinationRevision:  row.DestinationRevision,
		Mappings: boardcopy.CopyMappings{
			Board: boardcopy.CopyIdentityMap{
				Source:      key.SourceBoardID,
				Destination: row.DestinationBoardID,
			},
			Issues:      issueMappings,
			LogEntries:  logMappings,
			Attachments: attachmentMappings,
		},
	}, true, nil
}

func (r *CopyRepository) allocateCopyBoardID(
	ctx context.Context,
	queries *query.Queries,
	sourceID string,
) (string, error) {
	exists, err := queries.ProjectCopyBoardIDExists(ctx, sourceID)
	if err != nil {
		return "", fmt.Errorf("inspect destination board identity: %w", err)
	}
	if !exists {
		return sourceID, nil
	}
	for range 32 {
		var body [16]byte
		if _, err := io.ReadFull(r.entropy, body[:]); err != nil {
			return "", fmt.Errorf("generate destination board identity: %w", err)
		}
		digest := sha256.Sum256(body[:])
		candidate := "board_" + hex.EncodeToString(digest[:10])
		exists, err := queries.ProjectCopyBoardIDExists(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("inspect destination board identity: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", errors.New("allocate destination board identity: collision limit reached")
}

func (r *CopyRepository) allocateCopyIssueIDs(
	ctx context.Context,
	change *store.Change,
	queries *query.Queries,
	sourceIDs []string,
	policy configuration.IssueIDConfiguration,
) ([]boardcopy.CopyIdentityMap, error) {
	issueCount, err := queries.BoardCountAllIssues(ctx)
	if err != nil {
		return nil, fmt.Errorf("count destination issues: %w", err)
	}
	reserved := make(map[string]struct{}, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		reserved[sourceID] = struct{}{}
	}
	out := make([]boardcopy.CopyIdentityMap, 0, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		destination := sourceID
		exists, err := queries.BoardIssueIDExists(ctx, destination)
		if err != nil {
			return nil, fmt.Errorf("inspect destination issue identity: %w", err)
		}
		if exists {
			destination, err = r.allocateCopyIssueID(
				ctx,
				change,
				queries,
				policy,
				issueCount,
				reserved,
			)
			if err != nil {
				return nil, err
			}
		}
		reserved[destination] = struct{}{}
		out = append(out, boardcopy.CopyIdentityMap{
			Source: sourceID, Destination: destination,
		})
		issueCount++
	}
	return out, nil
}

func (r *CopyRepository) allocateCopyIssueID(
	ctx context.Context,
	change *store.Change,
	queries *query.Queries,
	policy configuration.IssueIDConfiguration,
	issueCount int64,
	reserved map[string]struct{},
) (string, error) {
	if policy.Strategy == configuration.IDStrategySequential {
		for range 32 {
			number, err := change.ReserveIssueNumber(ctx)
			if err != nil {
				return "", err
			}
			candidate := policy.Prefix.String() + strconv.FormatInt(number, 10)
			if _, ok := reserved[candidate]; ok {
				continue
			}
			exists, err := queries.BoardIssueIDExists(ctx, candidate)
			if err != nil {
				return "", fmt.Errorf(
					"inspect destination issue identity: %w",
					err,
				)
			}
			if !exists {
				return candidate, nil
			}
		}
		return "", errors.New("allocate sequential destination issue identity: collision limit reached")
	}

	length := randomSuffixLength(issueCount)
	for range 32 {
		suffix, err := randomIssueSuffix(r.entropy, length)
		if err != nil {
			return "", err
		}
		candidate := policy.Prefix.String() + suffix
		if _, ok := reserved[candidate]; ok {
			continue
		}
		exists, err := queries.BoardIssueIDExists(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf(
				"inspect destination issue identity: %w",
				err,
			)
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", errors.New("allocate random destination issue identity: collision limit reached")
}

func (r *CopyRepository) allocateCopyLogIDs(
	ctx context.Context,
	queries *query.Queries,
	sourceIDs []string,
) ([]boardcopy.CopyIdentityMap, error) {
	reserved := make(map[string]struct{}, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		reserved[sourceID] = struct{}{}
	}
	out := make([]boardcopy.CopyIdentityMap, 0, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		destination := sourceID
		exists, err := queries.BoardCopyLogIDExists(ctx, destination)
		if err != nil {
			return nil, fmt.Errorf("inspect destination Log identity: %w", err)
		}
		if exists {
			allocated := false
			for range 32 {
				candidate, err := newLogID(r.entropy)
				if err != nil {
					return nil, err
				}
				destination = candidate.String()
				if _, ok := reserved[destination]; ok {
					continue
				}
				exists, err := queries.BoardCopyLogIDExists(ctx, destination)
				if err != nil {
					return nil, fmt.Errorf(
						"inspect destination Log identity: %w",
						err,
					)
				}
				if !exists {
					allocated = true
					break
				}
			}
			if !allocated {
				return nil, errors.New(
					"allocate destination Log identity: collision limit reached",
				)
			}
		}
		reserved[destination] = struct{}{}
		out = append(out, boardcopy.CopyIdentityMap{
			Source: sourceID, Destination: destination,
		})
	}
	return out, nil
}

func identityMappings(values []string) []boardcopy.CopyIdentityMap {
	out := make([]boardcopy.CopyIdentityMap, 0, len(values))
	for _, value := range values {
		out = append(out, boardcopy.CopyIdentityMap{
			Source: value, Destination: value,
		})
	}
	return out
}

func mappingIndex(values []boardcopy.CopyIdentityMap) map[string]string {
	out := make(map[string]string, len(values))
	for _, value := range values {
		out[value.Source] = value.Destination
	}
	return out
}

func copyReferenceRewriter(
	issueIDs map[string]string,
	logIDs map[string]string,
) func(string) string {
	return func(source string) string {
		return markdown.RewriteReferences(
			source,
			func(identity reference.Identity) string {
				switch identity.Kind {
				case reference.KindIssue:
					if mapped, ok := issueIDs[identity.ID]; ok {
						return "%" + mapped
					}
				case reference.KindLog:
					if mapped, ok := logIDs[identity.ID]; ok {
						return "%" + mapped
					}
				case reference.KindAttachment:
					return "%" + identity.ID
				}
				return "%" + identity.ID
			},
		)
	}
}

func rewriteOptionalCopyMarkdown(
	value *string,
	rewrite func(string) string,
) *string {
	if value == nil {
		return nil
	}
	rewritten := rewrite(*value)
	return &rewritten
}

func insertCopyReceipt(
	ctx context.Context,
	queries *query.Queries,
	index boardcopy.RecordIndex,
	projectID string,
	name string,
	boardID string,
	revision int64,
	createdAt time.Time,
) (int64, error) {
	receiptID, err := queries.BoardInsertCopyReceipt(
		ctx,
		query.BoardInsertCopyReceiptParams{
			SourceLineageID:      index.Header.SourceLineageID,
			SourceBoardID:        index.Header.Board.ID,
			SnapshotVersion:      int64(index.Header.Version),
			SnapshotDigest:       index.Digest,
			SourceRevision:       index.Header.SourceRevision,
			DestinationProjectID: projectID,
			DestinationName:      name,
			DestinationBoardID:   boardID,
			DestinationRevision:  revision,
			CreatedAt:            createdAt,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("create board copy receipt: %w", err)
	}
	return receiptID, nil
}

func insertCopyMappings(
	ctx context.Context,
	queries *query.Queries,
	receiptID int64,
	issueMappings []boardcopy.CopyIdentityMap,
	logMappings []boardcopy.CopyIdentityMap,
	attachmentMappings []boardcopy.CopyIdentityMap,
) error {
	for _, value := range issueMappings {
		if err := queries.BoardInsertCopyIssueMapping(
			ctx,
			query.BoardInsertCopyIssueMappingParams{
				ReceiptID: receiptID, SourceID: value.Source,
				DestinationID: value.Destination,
			},
		); err != nil {
			return fmt.Errorf("create board copy issue mapping: %w", err)
		}
	}
	for _, value := range logMappings {
		if err := queries.BoardInsertCopyLogMapping(
			ctx,
			query.BoardInsertCopyLogMappingParams{
				ReceiptID: receiptID, SourceID: value.Source,
				DestinationID: value.Destination,
			},
		); err != nil {
			return fmt.Errorf("create board copy Log mapping: %w", err)
		}
	}
	for _, value := range attachmentMappings {
		if err := queries.BoardInsertCopyAttachmentMapping(
			ctx,
			query.BoardInsertCopyAttachmentMappingParams{
				ReceiptID: receiptID, SourceID: value.Source,
				DestinationID: value.Destination,
			},
		); err != nil {
			return fmt.Errorf("create board copy attachment mapping: %w", err)
		}
	}
	return nil
}

func advanceCopyIssueAllocator(
	ctx context.Context,
	change *store.Change,
	cfg configuration.Configuration,
	mappings []boardcopy.CopyIdentityMap,
) error {
	if cfg.Issue.ID.Strategy != configuration.IDStrategySequential {
		return nil
	}
	var next int64
	prefix := cfg.Issue.ID.Prefix.String()
	for _, mapping := range mappings {
		if mapping.Source != mapping.Destination ||
			!strings.HasPrefix(mapping.Destination, prefix) {
			continue
		}
		number, err := strconv.ParseInt(
			strings.TrimPrefix(mapping.Destination, prefix),
			10,
			64,
		)
		if err == nil && number >= next {
			if number == math.MaxInt64 {
				return errors.New(
					"destination sequential issue ID space exhausted",
				)
			}
			next = number + 1
		}
	}
	if next == 0 {
		return nil
	}
	if err := change.AdvanceIssueNumber(ctx, next); err != nil {
		return fmt.Errorf("advance destination issue allocator: %w", err)
	}
	return nil
}

func copyOutcome(
	index boardcopy.RecordIndex,
	projectID string,
	name string,
	boardID string,
	revision int64,
	alreadyCompleted bool,
	issueMappings []boardcopy.CopyIdentityMap,
	logMappings []boardcopy.CopyIdentityMap,
	attachmentMappings []boardcopy.CopyIdentityMap,
) boardcopy.CopyOutcome {
	return boardcopy.CopyOutcome{
		SourceLineageID:      index.Header.SourceLineageID,
		SourceBoardID:        index.Header.Board.ID,
		SourceRevision:       index.Header.SourceRevision,
		SnapshotVersion:      index.Header.Version,
		SnapshotDigest:       index.Digest,
		DestinationProjectID: projectID,
		DestinationBoardID:   boardID,
		DestinationName:      name,
		DestinationRevision:  revision,
		AlreadyCompleted:     alreadyCompleted,
		Counts: boardcopy.CopyCounts{
			Issues: len(index.IssueIDs), LogEntries: len(index.LogEntryIDs),
			Attachments: len(index.AttachmentIDs),
		},
		Mappings: boardcopy.CopyMappings{
			Board: boardcopy.CopyIdentityMap{
				Source: index.Header.Board.ID, Destination: boardID,
			},
			Issues:      issueMappings,
			LogEntries:  logMappings,
			Attachments: attachmentMappings,
		},
	}
}

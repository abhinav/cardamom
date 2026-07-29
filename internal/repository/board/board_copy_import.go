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
	"strconv"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/attachment"
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

// ImportCopySnapshot atomically creates one destination board and receipt.
func (r *CopyRepository) ImportCopySnapshot(
	ctx context.Context,
	snapshot boardcopy.CopySnapshot,
	options boardcopy.CopyOptions,
) (result boardcopy.CopyImportResult, err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return result, fmt.Errorf("begin destination board copy: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	queries := query.New(change)

	name := snapshot.Board.Name
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
			SourceLineageID: snapshot.SourceLineageID,
			SourceBoardID:   snapshot.Board.ID,
			SnapshotVersion: snapshot.Version,
		},
	)
	if err != nil {
		return result, err
	}
	if found {
		if err := change.Commit(); err != nil {
			return result, err
		}
		return boardcopy.NewCompetingCopyImport(receipt), nil
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

	boardID, err := r.allocateCopyBoardID(ctx, queries, snapshot.Board.ID)
	if err != nil {
		return result, err
	}
	issueMappings, err := r.allocateCopyIssueIDs(
		ctx,
		change,
		queries,
		snapshot,
	)
	if err != nil {
		return result, err
	}
	logMappings, err := r.allocateCopyLogIDs(ctx, queries, snapshot)
	if err != nil {
		return result, err
	}
	attachmentMappings := identityMappingsForAttachments(snapshot.Attachments)

	revisionCount := int64(1)
	if hasRemovedCopyAttachment(snapshot.Attachments) {
		revisionCount = 2
	}
	revisions, err := change.ReserveRevisions(ctx, revisionCount)
	if err != nil {
		return result, fmt.Errorf("reserve destination revisions: %w", err)
	}

	if err := importCopyMetadata(
		ctx,
		queries,
		snapshot,
		options.ProjectID,
		name,
		boardID,
		issueMappings,
		logMappings,
		revisions.FirstRevision(),
		revisions.LastRevision(),
	); err != nil {
		return result, err
	}
	receiptID, err := insertCopyReceipt(
		ctx,
		queries,
		snapshot,
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
		snapshot.Configuration,
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
		snapshot,
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
	snapshot boardcopy.CopySnapshot,
) ([]boardcopy.CopyIdentityMap, error) {
	issueCount, err := queries.BoardCountAllIssues(ctx)
	if err != nil {
		return nil, fmt.Errorf("count destination issues: %w", err)
	}
	reserved := make(map[string]struct{}, len(snapshot.Issues))
	for _, source := range snapshot.Issues {
		reserved[source.ID] = struct{}{}
	}
	out := make([]boardcopy.CopyIdentityMap, 0, len(snapshot.Issues))
	for _, source := range snapshot.Issues {
		destination := source.ID
		exists, err := queries.BoardIssueIDExists(ctx, destination)
		if err != nil {
			return nil, fmt.Errorf("inspect destination issue identity: %w", err)
		}
		if exists {
			destination, err = r.allocateCopyIssueID(
				ctx,
				change,
				queries,
				snapshot.Configuration.Issue.ID,
				issueCount,
				reserved,
			)
			if err != nil {
				return nil, err
			}
		}
		reserved[destination] = struct{}{}
		out = append(out, boardcopy.CopyIdentityMap{
			Source: source.ID, Destination: destination,
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
	snapshot boardcopy.CopySnapshot,
) ([]boardcopy.CopyIdentityMap, error) {
	reserved := make(map[string]struct{}, len(snapshot.LogEntries))
	for _, source := range snapshot.LogEntries {
		reserved[source.ID] = struct{}{}
	}
	out := make([]boardcopy.CopyIdentityMap, 0, len(snapshot.LogEntries))
	for _, source := range snapshot.LogEntries {
		destination := source.ID
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
			Source: source.ID, Destination: destination,
		})
	}
	return out, nil
}

func identityMappingsForAttachments(
	values []boardcopy.CopyAttachment,
) []boardcopy.CopyIdentityMap {
	out := make([]boardcopy.CopyIdentityMap, 0, len(values))
	for _, value := range values {
		out = append(out, boardcopy.CopyIdentityMap{
			Source: value.ID, Destination: value.ID,
		})
	}
	return out
}

func hasRemovedCopyAttachment(values []boardcopy.CopyAttachment) bool {
	for _, value := range values {
		if value.Lifecycle == attachment.LifecycleRemoved.String() {
			return true
		}
	}
	return false
}

func importCopyMetadata(
	ctx context.Context,
	queries *query.Queries,
	snapshot boardcopy.CopySnapshot,
	projectID string,
	name string,
	boardID string,
	issueMappings []boardcopy.CopyIdentityMap,
	logMappings []boardcopy.CopyIdentityMap,
	firstRevision int64,
	lastRevision int64,
) error {
	issueIDs := mappingIndex(issueMappings)
	logIDs := mappingIndex(logMappings)
	rewrite := copyReferenceRewriter(issueIDs, logIDs)

	description := rewriteOptionalCopyMarkdown(snapshot.Board.Description, rewrite)
	prefix := snapshot.Configuration.Issue.ID.Prefix.String()
	strategy := snapshot.Configuration.Issue.ID.Strategy.String()
	summaryMaxBytes := int64(
		snapshot.Configuration.Issue.Summary.MaxBytes.Uint64(),
	)
	attachmentMaxBytes := int64(
		snapshot.Configuration.Attachment.MaxBytes.Uint64(),
	)
	if err := queries.ProjectInsertCopiedBoard(
		ctx,
		query.ProjectInsertCopiedBoardParams{
			ID:                   boardID,
			ProjectID:            projectID,
			Name:                 name,
			Description:          description,
			CreatedAt:            snapshot.Board.CreatedAt,
			IssueIDPrefix:        &prefix,
			IssueIDStrategy:      &strategy,
			IssueSummaryMaxBytes: &summaryMaxBytes,
			AttachmentMaxBytes:   &attachmentMaxBytes,
			Revision:             lastRevision,
		},
	); err != nil {
		return fmt.Errorf("create destination board: %w", err)
	}

	for _, value := range snapshot.Issues {
		err := queries.BoardInsertCopiedIssue(
			ctx,
			query.BoardInsertCopiedIssueParams{
				ID:            issueIDs[value.ID],
				BoardID:       boardID,
				Title:         value.Title,
				Kind:          value.Kind,
				Lifecycle:     value.Lifecycle,
				Priority:      value.Priority,
				CreatedAt:     value.CreatedAt,
				UpdatedAt:     value.UpdatedAt,
				ClosedAt:      value.ClosedAt,
				WaitingReason: value.WaitingReason,
				WaitingSince:  value.WaitingSince,
				Summary: rewriteOptionalCopyMarkdown(
					value.Summary,
					rewrite,
				),
				Details: rewriteOptionalCopyMarkdown(
					value.Details,
					rewrite,
				),
				Revision: lastRevision,
			},
		)
		if err != nil {
			return fmt.Errorf("create destination issue %q: %w", value.ID, err)
		}
	}
	for _, value := range snapshot.Labels {
		if err := queries.BoardInsertIssueLabel(
			ctx,
			query.BoardInsertIssueLabelParams{
				BoardID: boardID,
				IssueID: issueIDs[value.IssueID],
				Label:   value.Label,
			},
		); err != nil {
			return fmt.Errorf("create destination issue label: %w", err)
		}
	}
	for _, value := range snapshot.Dependencies {
		if err := queries.BoardInsertIssueDependency(
			ctx,
			query.BoardInsertIssueDependencyParams{
				BoardID:        boardID,
				IssueID:        issueIDs[value.IssueID],
				PrerequisiteID: issueIDs[value.PrerequisiteID],
			},
		); err != nil {
			return fmt.Errorf("create destination dependency: %w", err)
		}
	}
	for _, value := range snapshot.Containment {
		if err := queries.BoardInsertIssueParent(
			ctx,
			query.BoardInsertIssueParentParams{
				BoardID:  boardID,
				ChildID:  issueIDs[value.ChildID],
				ParentID: issueIDs[value.ParentID],
			},
		); err != nil {
			return fmt.Errorf("create destination containment: %w", err)
		}
	}
	for _, value := range snapshot.ExternalKeys {
		if err := queries.BoardInsertIssueExternalKey(
			ctx,
			query.BoardInsertIssueExternalKeyParams{
				BoardID:     boardID,
				ExternalKey: value.Key,
				IssueID:     issueIDs[value.IssueID],
			},
		); err != nil {
			return fmt.Errorf("create destination external key: %w", err)
		}
	}
	for _, value := range snapshot.LogEntries {
		if err := queries.BoardInsertIssueLogEntry(
			ctx,
			query.BoardInsertIssueLogEntryParams{
				ID:         logIDs[value.ID],
				BoardID:    boardID,
				IssueID:    issueIDs[value.IssueID],
				Kind:       value.Kind,
				Author:     value.Author,
				Committer:  value.Committer,
				Body:       rewrite(value.Body),
				CreatedAt:  value.CreatedAt,
				NextAction: rewriteOptionalCopyMarkdown(value.NextAction, rewrite),
			},
		); err != nil {
			return fmt.Errorf("create destination Log entry %q: %w", value.ID, err)
		}
	}
	for _, value := range snapshot.States {
		var snapshotID *string
		if value.SnapshotLogEntryID != nil {
			mapped := logIDs[*value.SnapshotLogEntryID]
			snapshotID = &mapped
		}
		if err := queries.BoardUpsertIssueState(
			ctx,
			query.BoardUpsertIssueStateParams{
				IssueID:            issueIDs[value.IssueID],
				BoardID:            boardID,
				Body:               rewrite(value.Body),
				Author:             value.Author,
				UpdatedAt:          value.UpdatedAt,
				SnapshotLogEntryID: snapshotID,
				NextAction: rewriteOptionalCopyMarkdown(
					value.NextAction,
					rewrite,
				),
			},
		); err != nil {
			return fmt.Errorf("create destination State: %w", err)
		}
	}
	for _, value := range snapshot.Results {
		if err := queries.BoardUpsertIssueResult(
			ctx,
			query.BoardUpsertIssueResultParams{
				IssueID: issueIDs[value.IssueID],
				BoardID: boardID,
				Body:    rewrite(value.Body),
			},
		); err != nil {
			return fmt.Errorf("create destination Result: %w", err)
		}
	}
	for _, value := range snapshot.Checkpoints {
		if err := queries.BoardInsertCheckpointDecision(
			ctx,
			query.BoardInsertCheckpointDecisionParams{
				IssueID:   issueIDs[value.IssueID],
				BoardID:   boardID,
				Outcome:   value.Outcome,
				Reason:    rewrite(value.Reason),
				DecidedAt: value.DecidedAt,
				Revision:  lastRevision,
			},
		); err != nil {
			return fmt.Errorf("create destination checkpoint decision: %w", err)
		}
	}
	for _, value := range snapshot.Attachments {
		if err := queries.AttachmentRetainBlob(
			ctx,
			query.AttachmentRetainBlobParams{
				Digest:    value.Blob.Digest.String(),
				SizeBytes: int64(value.Blob.SizeBytes),
			},
		); err != nil {
			return fmt.Errorf("create destination blob descriptor: %w", err)
		}
		var originIssueID *string
		if value.OriginIssueID != nil {
			mapped := issueIDs[*value.OriginIssueID]
			originIssueID = &mapped
		}
		createdRevision := lastRevision
		var removedRevision *int64
		if value.Lifecycle == attachment.LifecycleRemoved.String() {
			createdRevision = firstRevision
			removedRevision = &lastRevision
		}
		if err := queries.AttachmentInsertCopiedMetadata(
			ctx,
			query.AttachmentInsertCopiedMetadataParams{
				BoardID:         boardID,
				ID:              value.ID,
				OriginIssueID:   originIssueID,
				BlobDigest:      value.Blob.Digest.String(),
				BlobSizeBytes:   int64(value.Blob.SizeBytes),
				Filename:        value.Filename,
				MediaType:       value.MediaType,
				Lifecycle:       value.Lifecycle,
				CreatedActor:    value.CreatedActor,
				CreatedAt:       value.CreatedAt,
				CreatedRevision: createdRevision,
				RemovedActor:    value.RemovedActor,
				RemovedAt:       value.RemovedAt,
				RemovedRevision: removedRevision,
			},
		); err != nil {
			return fmt.Errorf("create destination attachment %q: %w", value.ID, err)
		}
	}
	return nil
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
	snapshot boardcopy.CopySnapshot,
	projectID string,
	name string,
	boardID string,
	revision int64,
	createdAt time.Time,
) (int64, error) {
	receiptID, err := queries.BoardInsertCopyReceipt(
		ctx,
		query.BoardInsertCopyReceiptParams{
			SourceLineageID:      snapshot.SourceLineageID,
			SourceBoardID:        snapshot.Board.ID,
			SnapshotVersion:      int64(snapshot.Version),
			SnapshotDigest:       snapshot.Digest,
			SourceRevision:       snapshot.SourceRevision,
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
	snapshot boardcopy.CopySnapshot,
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
		SourceLineageID:      snapshot.SourceLineageID,
		SourceBoardID:        snapshot.Board.ID,
		SourceRevision:       snapshot.SourceRevision,
		SnapshotVersion:      snapshot.Version,
		SnapshotDigest:       snapshot.Digest,
		DestinationProjectID: projectID,
		DestinationBoardID:   boardID,
		DestinationName:      name,
		DestinationRevision:  revision,
		AlreadyCompleted:     alreadyCompleted,
		Counts:               copyCounts(snapshot),
		Mappings: boardcopy.CopyMappings{
			Board: boardcopy.CopyIdentityMap{
				Source: snapshot.Board.ID, Destination: boardID,
			},
			Issues:      issueMappings,
			LogEntries:  logMappings,
			Attachments: attachmentMappings,
		},
	}
}

func copyCounts(snapshot boardcopy.CopySnapshot) boardcopy.CopyCounts {
	return boardcopy.CopyCounts{
		Issues:      len(snapshot.Issues),
		LogEntries:  len(snapshot.LogEntries),
		Attachments: len(snapshot.Attachments),
	}
}

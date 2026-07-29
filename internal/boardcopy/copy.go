// Package boardcopy owns non-destructive semantic board transfer between
// physical Cardamom stores.
package boardcopy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/must"
)

const (
	// CopySnapshotVersion identifies the first semantic board-copy schema.
	CopySnapshotVersion = 1
)

// CopySnapshot is one complete, schema-independent board publication.
//
// SourceLineageID and SourceRevision identify the retained source view.
// Digest covers the portable semantic fields and is assigned by Service.
type CopySnapshot struct {
	SourceLineageID string
	SourceRevision  int64
	Version         int
	Digest          string
	Board           CopyBoard
	Configuration   configuration.Configuration
	Issues          []CopyIssue
	Labels          []CopyLabel
	Dependencies    []CopyDependency
	Containment     []CopyContainment
	ExternalKeys    []CopyExternalKey
	LogEntries      []CopyLogEntry
	States          []CopyState
	Results         []CopyResultRecord
	Checkpoints     []CopyCheckpoint
	Attachments     []CopyAttachment
}

// CopyBoard contains the portable board namespace fields.
type CopyBoard struct {
	ID          string
	Name        string
	Description *string
	CreatedAt   time.Time
}

// CopyIssue contains one portable issue projection.
type CopyIssue struct {
	ID            string
	Title         string
	Kind          string
	Lifecycle     string
	Priority      int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ClosedAt      *time.Time
	WaitingReason *string
	WaitingSince  *time.Time
	Summary       *string
	Details       *string
}

// CopyLabel associates one inert label with an issue.
type CopyLabel struct {
	IssueID string
	Label   string
}

// CopyDependency records one issue prerequisite edge.
type CopyDependency struct {
	IssueID        string
	PrerequisiteID string
}

// CopyContainment records one child-parent edge.
type CopyContainment struct {
	ChildID  string
	ParentID string
}

// CopyExternalKey associates one board-scoped producer key with an issue.
type CopyExternalKey struct {
	Key     string
	IssueID string
}

// CopyLogEntry contains one immutable Log record in source order.
type CopyLogEntry struct {
	Order      uint64
	ID         string
	IssueID    string
	Kind       string
	Author     *string
	Committer  *string
	Body       string
	CreatedAt  *time.Time
	NextAction *string
}

// CopyState contains one issue's mutable recovery record.
type CopyState struct {
	IssueID            string
	Body               string
	Author             *string
	UpdatedAt          *time.Time
	SnapshotLogEntryID *string
	NextAction         *string
}

// CopyResultRecord contains one issue's current durable outcome.
type CopyResultRecord struct {
	IssueID string
	Body    string
}

// CopyCheckpoint contains one immutable checkpoint decision.
type CopyCheckpoint struct {
	IssueID   string
	Outcome   string
	Reason    string
	DecidedAt time.Time
}

// CopyAttachment contains active or removed board-scoped attachment metadata.
type CopyAttachment struct {
	ID            string
	OriginIssueID *string
	Blob          attachment.BlobDescriptor
	Filename      string
	MediaType     string
	Lifecycle     string
	CreatedActor  string
	CreatedAt     time.Time
	RemovedActor  *string
	RemovedAt     *time.Time
}

// CopyOptions selects the destination namespace and optional board name.
type CopyOptions struct {
	ProjectID string
	Name      *string
}

// CopyRequest selects the source board and destination options.
type CopyRequest struct {
	SourceBoardID board.ID
	Options       CopyOptions
}

// CopyIdentityMap records one source-to-destination identity decision.
type CopyIdentityMap struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// CopyMappings contains every persisted identity mapping.
type CopyMappings struct {
	Board       CopyIdentityMap   `json:"board"`
	Issues      []CopyIdentityMap `json:"issues"`
	LogEntries  []CopyIdentityMap `json:"log_entries"`
	Attachments []CopyIdentityMap `json:"attachments"`
}

// CopyCounts summarizes the semantic records and unique blobs transferred.
type CopyCounts struct {
	Issues      int `json:"issues"`
	LogEntries  int `json:"log_entries"`
	Attachments int `json:"attachments"`
	Blobs       int `json:"blobs"`
}

// CopyOutcome reports one new or previously completed board copy.
type CopyOutcome struct {
	SourceLineageID      string       `json:"source_lineage_id"`
	SourceBoardID        string       `json:"source_board_id"`
	SourceRevision       int64        `json:"source_revision"`
	SnapshotVersion      int          `json:"snapshot_version"`
	SnapshotDigest       string       `json:"snapshot_digest"`
	DestinationProjectID string       `json:"destination_project_id"`
	DestinationBoardID   string       `json:"destination_board_id"`
	DestinationName      string       `json:"destination_name"`
	DestinationRevision  int64        `json:"destination_revision"`
	AlreadyCompleted     bool         `json:"already_completed"`
	Counts               CopyCounts   `json:"counts"`
	Mappings             CopyMappings `json:"mappings"`
}

// CopyReceiptKey identifies one source publication series in a destination
// store.
type CopyReceiptKey struct {
	SourceLineageID string
	SourceBoardID   string
	SnapshotVersion int
}

// CopyReceipt contains the persisted facts from one completed publication.
type CopyReceipt struct {
	SourceLineageID      string
	SourceBoardID        string
	SourceRevision       int64
	SnapshotVersion      int
	SnapshotDigest       string
	DestinationProjectID string
	DestinationBoardID   string
	DestinationName      string
	DestinationRevision  int64
	Mappings             CopyMappings
}

// CopyImportResult reports either a new publication or a receipt that won a
// concurrent import race.
type CopyImportResult struct {
	// Published is set only when the import created the destination board.
	Published *CopyOutcome

	// Competing is set only when the import found and committed a no-op around
	// an existing receipt.
	Competing *CopyReceipt
}

// NewPublishedCopyImport reports a newly published destination board.
func NewPublishedCopyImport(outcome CopyOutcome) CopyImportResult {
	return CopyImportResult{Published: &outcome}
}

// NewCompetingCopyImport reports persisted receipt facts found during import.
func NewCompetingCopyImport(receipt CopyReceipt) CopyImportResult {
	return CopyImportResult{Competing: &receipt}
}

// CopySnapshotSource reads one retained semantic board view.
type CopySnapshotSource interface {
	ReadCopySnapshot(
		context.Context,
		board.ID,
		configuration.Overrides,
	) (CopySnapshot, error)
}

// CopySnapshotDestination atomically imports one semantic board snapshot.
type CopySnapshotDestination interface {
	// ReadCopyReceipt returns persisted publication facts when present.
	ReadCopyReceipt(
		context.Context,
		CopyReceiptKey,
	) (CopyReceipt, bool, error)

	// ImportCopySnapshot atomically publishes metadata or returns a competing
	// receipt after committing its no-op transaction.
	ImportCopySnapshot(
		context.Context,
		CopySnapshot,
		CopyOptions,
	) (CopyImportResult, error)
}

// CopyBlobSource opens verified content without exposing its private path.
type CopyBlobSource interface {
	OpenCopyBlob(
		context.Context,
		attachment.BlobDescriptor,
	) (io.ReadCloser, error)
}

// CopyBlobDestination publishes verified content before metadata references it.
type CopyBlobDestination interface {
	PublishCopyBlob(
		context.Context,
		attachment.BlobDescriptor,
		io.Reader,
	) error
}

// CopyConfiguration reads the source store's physical configuration layer.
type CopyConfiguration interface {
	ReadStoreConfiguration(context.Context) (configuration.Overrides, error)
}

// CopyServiceConfig supplies the finite collaborators for one board copy.
type CopyServiceConfig struct {
	Source           CopySnapshotSource      // required
	Destination      CopySnapshotDestination // required
	SourceBlobs      CopyBlobSource          // required
	DestinationBlobs CopyBlobDestination     // required
	Configuration    CopyConfiguration       // required
}

// CopyService owns one non-destructive board copy between physical stores.
type CopyService struct {
	source           CopySnapshotSource
	destination      CopySnapshotDestination
	sourceBlobs      CopyBlobSource
	destinationBlobs CopyBlobDestination
	configuration    CopyConfiguration
}

// NewCopyService constructs the finite board-copy operation.
func NewCopyService(cfg CopyServiceConfig) *CopyService {
	must.NotBeNilf(cfg.Source, "board copy source is required")
	must.NotBeNilf(cfg.Destination, "board copy destination is required")
	must.NotBeNilf(cfg.SourceBlobs, "board copy source blobs are required")
	must.NotBeNilf(cfg.DestinationBlobs, "board copy destination blobs are required")
	must.NotBeNilf(cfg.Configuration, "board copy configuration is required")
	return &CopyService{
		source: cfg.Source, destination: cfg.Destination,
		sourceBlobs: cfg.SourceBlobs, destinationBlobs: cfg.DestinationBlobs,
		configuration: cfg.Configuration,
	}
}

// Copy creates one destination board or returns an identical prior receipt.
func (s *CopyService) Copy(
	ctx context.Context,
	request CopyRequest,
) (CopyOutcome, error) {
	if _, err := board.NewID(request.SourceBoardID.String()); err != nil {
		return CopyOutcome{}, err
	}
	if strings.TrimSpace(request.Options.ProjectID) == "" {
		return CopyOutcome{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid project namespace: destination project required",
		)
	}
	if request.Options.Name != nil && strings.TrimSpace(*request.Options.Name) == "" {
		return CopyOutcome{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid project namespace: board name required",
		)
	}

	before, err := s.configuration.ReadStoreConfiguration(ctx)
	if err != nil {
		return CopyOutcome{}, fmt.Errorf("read source store configuration: %w", err)
	}
	snapshot, err := s.source.ReadCopySnapshot(
		ctx,
		request.SourceBoardID,
		before,
	)
	if err != nil {
		return CopyOutcome{}, err
	}
	after, err := s.configuration.ReadStoreConfiguration(ctx)
	if err != nil {
		return CopyOutcome{}, fmt.Errorf("reread source store configuration: %w", err)
	}
	if !before.Equal(after) {
		return CopyOutcome{}, errkind.Errorf(
			errkind.Conflict,
			"source store configuration changed while reading the board snapshot",
		)
	}

	snapshot.Version = CopySnapshotVersion
	snapshot = canonicalCopySnapshot(snapshot)
	snapshot.Digest = snapshotDigest(snapshot)
	descriptors := uniqueBlobDescriptors(snapshot.Attachments)
	receipt, found, err := s.destination.ReadCopyReceipt(
		ctx,
		copyReceiptKey(snapshot),
	)
	if err != nil {
		return CopyOutcome{}, err
	}
	if found {
		outcome, err := EvaluateCopyReceipt(
			snapshot,
			request.Options,
			receipt,
		)
		if err != nil {
			return CopyOutcome{}, err
		}
		outcome.Counts.Blobs = len(descriptors)
		return outcome, nil
	}
	for _, descriptor := range descriptors {
		reader, err := s.sourceBlobs.OpenCopyBlob(ctx, descriptor)
		if err != nil {
			return CopyOutcome{}, fmt.Errorf(
				"open source blob %s: %w",
				descriptor.Digest,
				err,
			)
		}
		publishErr := s.destinationBlobs.PublishCopyBlob(ctx, descriptor, reader)
		closeErr := reader.Close()
		if err := errors.Join(publishErr, closeErr); err != nil {
			return CopyOutcome{}, fmt.Errorf(
				"publish destination blob %s: %w",
				descriptor.Digest,
				err,
			)
		}
	}

	importResult, err := s.destination.ImportCopySnapshot(
		ctx,
		snapshot,
		request.Options,
	)
	if err != nil {
		return CopyOutcome{}, err
	}
	var outcome CopyOutcome
	switch {
	case importResult.Published != nil && importResult.Competing == nil:
		outcome = *importResult.Published
	case importResult.Published == nil && importResult.Competing != nil:
		outcome, err = EvaluateCopyReceipt(
			snapshot,
			request.Options,
			*importResult.Competing,
		)
		if err != nil {
			return CopyOutcome{}, err
		}
	default:
		return CopyOutcome{}, errors.New(
			"destination board copy returned an invalid import result",
		)
	}
	outcome.Counts.Blobs = len(descriptors)
	return outcome, nil
}

// EvaluateCopyReceipt applies retry policy to persisted receipt facts.
func EvaluateCopyReceipt(
	snapshot CopySnapshot,
	options CopyOptions,
	receipt CopyReceipt,
) (CopyOutcome, error) {
	if receipt.SnapshotDigest != snapshot.Digest {
		return CopyOutcome{}, errkind.Errorf(
			errkind.Conflict,
			"source board has changed since its previous copy; incremental synchronization is not supported",
		)
	}
	name := copyDestinationName(snapshot, options)
	if receipt.DestinationProjectID != options.ProjectID ||
		receipt.DestinationName != name {
		return CopyOutcome{}, errkind.Errorf(
			errkind.Conflict,
			"board snapshot was already copied with different destination options",
		)
	}
	return CopyOutcome{
		SourceLineageID:      receipt.SourceLineageID,
		SourceBoardID:        receipt.SourceBoardID,
		SourceRevision:       receipt.SourceRevision,
		SnapshotVersion:      receipt.SnapshotVersion,
		SnapshotDigest:       receipt.SnapshotDigest,
		DestinationProjectID: receipt.DestinationProjectID,
		DestinationBoardID:   receipt.DestinationBoardID,
		DestinationName:      receipt.DestinationName,
		DestinationRevision:  receipt.DestinationRevision,
		AlreadyCompleted:     true,
		Counts:               copyCounts(snapshot),
		Mappings:             receipt.Mappings,
	}, nil
}

func copyReceiptKey(snapshot CopySnapshot) CopyReceiptKey {
	return CopyReceiptKey{
		SourceLineageID: snapshot.SourceLineageID,
		SourceBoardID:   snapshot.Board.ID,
		SnapshotVersion: snapshot.Version,
	}
}

func copyDestinationName(
	snapshot CopySnapshot,
	options CopyOptions,
) string {
	name := snapshot.Board.Name
	if options.Name != nil {
		name = *options.Name
	}
	return strings.TrimSpace(name)
}

func copyCounts(snapshot CopySnapshot) CopyCounts {
	return CopyCounts{
		Issues:      len(snapshot.Issues),
		LogEntries:  len(snapshot.LogEntries),
		Attachments: len(snapshot.Attachments),
	}
}

func uniqueBlobDescriptors(values []CopyAttachment) []attachment.BlobDescriptor {
	byDigest := make(map[attachment.Digest]attachment.BlobDescriptor)
	for _, value := range values {
		byDigest[value.Blob.Digest] = value.Blob
	}
	out := make([]attachment.BlobDescriptor, 0, len(byDigest))
	for _, descriptor := range byDigest {
		out = append(out, descriptor)
	}
	slices.SortFunc(out, func(left, right attachment.BlobDescriptor) int {
		return strings.Compare(left.Digest.String(), right.Digest.String())
	})
	return out
}

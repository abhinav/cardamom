// Package boardcopy owns non-destructive semantic board transfer between
// physical Cardamom stores.
package boardcopy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/must"
)

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

// CopyImportResult is one successful atomic repository import outcome.
//
// The sealed result distinguishes a newly published board from a concurrent
// import that found the winning persisted receipt.
type CopyImportResult interface {
	evaluateCopyImport(RecordIndex, CopyOptions) (CopyOutcome, error)
}

// publishedCopyImport carries the outcome committed by a new publication.
type publishedCopyImport struct {
	outcome CopyOutcome
}

// NewPublishedCopyImport reports a newly published destination board.
func NewPublishedCopyImport(outcome CopyOutcome) CopyImportResult {
	return publishedCopyImport{outcome: outcome}
}

func (r publishedCopyImport) evaluateCopyImport(
	RecordIndex,
	CopyOptions,
) (CopyOutcome, error) {
	return r.outcome, nil
}

// concurrentCopyImport carries the receipt that won a concurrent import.
type concurrentCopyImport struct {
	receipt CopyReceipt
}

// NewConcurrentCopyImport reports the receipt found during a concurrent
// import.
func NewConcurrentCopyImport(receipt CopyReceipt) CopyImportResult {
	return concurrentCopyImport{receipt: receipt}
}

func (r concurrentCopyImport) evaluateCopyImport(
	index RecordIndex,
	options CopyOptions,
) (CopyOutcome, error) {
	return EvaluateRecordReceipt(index, options, r.receipt)
}

// EvaluateCopyImport applies boardcopy policy to a repository import result.
func EvaluateCopyImport(
	index RecordIndex,
	options CopyOptions,
	result CopyImportResult,
) (CopyOutcome, error) {
	return result.evaluateCopyImport(index, options)
}

// CopyReceiptDestination reads durable destination publication receipts.
type CopyReceiptDestination interface {
	// ReadCopyReceipt returns persisted publication facts when present.
	ReadCopyReceipt(
		context.Context,
		CopyReceiptKey,
	) (CopyReceipt, bool, error)
}

// RecordDestination atomically imports one indexed semantic record sequence.
type RecordDestination interface {
	CopyReceiptDestination

	// ImportCopyRecords atomically publishes metadata or returns a competing
	// receipt after committing its no-op transaction.
	ImportCopyRecords(
		context.Context,
		RecordIndex,
		RecordSequence,
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
	Source           RecordSource        // required
	Destination      RecordDestination   // required
	SourceBlobs      CopyBlobSource      // required
	DestinationBlobs CopyBlobDestination // required
	Configuration    CopyConfiguration   // required
}

// CopyService owns one non-destructive board copy between physical stores.
type CopyService struct {
	source           RecordSource
	destination      RecordDestination
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
	index, err := IndexRecords(
		s.source.ReadCopyRecords(ctx, request.SourceBoardID, before),
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

	receipt, found, err := s.destination.ReadCopyReceipt(
		ctx,
		copyReceiptKey(index),
	)
	if err != nil {
		return CopyOutcome{}, err
	}
	if found {
		outcome, err := EvaluateRecordReceipt(
			index,
			request.Options,
			receipt,
		)
		if err != nil {
			return CopyOutcome{}, err
		}
		outcome.Counts.Blobs = len(index.Blobs)
		return outcome, nil
	}
	for _, descriptor := range index.Blobs {
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

	importResult, err := s.destination.ImportCopyRecords(
		ctx,
		index,
		s.source.ReadCopyRecords(ctx, request.SourceBoardID, before),
		request.Options,
	)
	if err != nil {
		return CopyOutcome{}, err
	}
	outcome, err := EvaluateCopyImport(index, request.Options, importResult)
	if err != nil {
		return CopyOutcome{}, err
	}
	outcome.Counts.Blobs = len(index.Blobs)
	return outcome, nil
}

// EvaluateRecordReceipt applies retry policy to an indexed record publication.
func EvaluateRecordReceipt(
	index RecordIndex,
	options CopyOptions,
	receipt CopyReceipt,
) (CopyOutcome, error) {
	return evaluateCopyReceipt(
		index.Digest,
		copyDestinationName(index, options),
		copyCounts(index),
		options,
		receipt,
	)
}

func evaluateCopyReceipt(
	digest string,
	name string,
	counts CopyCounts,
	options CopyOptions,
	receipt CopyReceipt,
) (CopyOutcome, error) {
	if receipt.SnapshotDigest != digest {
		return CopyOutcome{}, errkind.Errorf(
			errkind.Conflict,
			"source board has changed since its previous copy; incremental synchronization is not supported",
		)
	}
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
		Counts:               counts,
		Mappings:             receipt.Mappings,
	}, nil
}

func copyReceiptKey(index RecordIndex) CopyReceiptKey {
	return CopyReceiptKey{
		SourceLineageID: index.Header.SourceLineageID,
		SourceBoardID:   index.Header.Board.ID,
		SnapshotVersion: index.Header.Version,
	}
}

func copyDestinationName(
	index RecordIndex,
	options CopyOptions,
) string {
	name := index.Header.Board.Name
	if options.Name != nil {
		name = *options.Name
	}
	return strings.TrimSpace(name)
}

func copyCounts(index RecordIndex) CopyCounts {
	return CopyCounts{
		Issues:      len(index.IssueIDs),
		LogEntries:  len(index.LogEntryIDs),
		Attachments: len(index.AttachmentIDs),
	}
}

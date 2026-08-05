package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/project"
)

// Selection identifies every source board or an explicit set of complete
// source boards.
type Selection struct {
	all      bool
	boardIDs []board.ID
}

// AllBoards selects every board retained by the source view.
func AllBoards() Selection {
	return Selection{all: true}
}

// SelectBoards selects one or more distinct complete boards by stable ID.
func SelectBoards(ids ...board.ID) (Selection, error) {
	if len(ids) == 0 {
		return Selection{}, errkind.Errorf(
			errkind.InvalidInput,
			"backup board selection is required",
		)
	}
	selected := make([]board.ID, len(ids))
	seen := make(map[board.ID]struct{}, len(ids))
	for index, id := range ids {
		parsed, err := board.NewID(id.String())
		if err != nil {
			return Selection{}, err
		}
		if _, duplicate := seen[parsed]; duplicate {
			return Selection{}, errkind.Errorf(
				errkind.InvalidInput,
				"backup board %q is selected more than once",
				parsed,
			)
		}
		seen[parsed] = struct{}{}
		selected[index] = parsed
	}
	return Selection{boardIDs: selected}, nil
}

// IsAll reports whether every source board is selected.
func (s Selection) IsAll() bool { return s.all }

// BoardIDs returns the explicit stable board IDs in caller order.
func (s Selection) BoardIDs() []board.ID { return slices.Clone(s.boardIDs) }

func (s Selection) validate() error {
	if s.all && len(s.boardIDs) == 0 {
		return nil
	}
	if !s.all && len(s.boardIDs) > 0 {
		return nil
	}
	return errkind.Errorf(errkind.InvalidInput, "backup board selection is required")
}

// CapturedBoard associates one complete semantic snapshot with its source
// project.
type CapturedBoard struct {
	// ProjectID identifies the source project recorded in the archive.
	ProjectID project.ID // required

	// Snapshot is the complete semantic board state from the retained source view.
	Snapshot boardcopy.CopySnapshot // required
}

// Capture contains project metadata and board snapshots from one retained
// source revision.
type Capture struct {
	// SourceLineageID identifies the source store persistence history.
	SourceLineageID string // required

	// SourceRevision is the canonical revision retained during capture.
	SourceRevision int64

	// Projects contains metadata for the projects referenced by Boards.
	Projects []project.Snapshot

	// Boards contains every selected complete semantic board snapshot.
	Boards []CapturedBoard
}

// Source captures selected semantic board state from one retained store view.
type Source interface {
	// Capture returns source metadata and semantic snapshots from one revision.
	Capture(
		context.Context,
		Selection,
		configuration.Overrides,
	) (Capture, error)
}

// BlobSource opens verified committed attachment content.
type BlobSource interface {
	// OpenCopyBlob returns a verified reader owned by the caller.
	OpenCopyBlob(
		context.Context,
		attachment.BlobDescriptor,
	) (io.ReadCloser, error)
}

// StoreConfiguration reads the physical source store configuration layer.
type StoreConfiguration interface {
	// ReadStoreConfiguration returns the current typed physical overrides.
	ReadStoreConfiguration(context.Context) (configuration.Overrides, error)
}

// ArchiveWrite streams a complete archive body to a staged destination.
type ArchiveWrite func(io.Writer) error

// Publisher atomically publishes a generated archive to its destination.
type Publisher interface {
	// Publish stages write output and replaces the destination after success.
	Publish(context.Context, string, ArchiveWrite) error
}

// OperationConfig supplies the retained-view, blob, configuration, and
// filesystem boundaries for backup generation.
type OperationConfig struct {
	Source        Source             // required
	Blobs         BlobSource         // required
	Configuration StoreConfiguration // required
	Publisher     Publisher          // required
}

// Operation writes selected complete board snapshots to portable archives.
type Operation struct {
	source        Source             // required
	blobs         BlobSource         // required
	configuration StoreConfiguration // required
	publisher     Publisher          // required
}

// NewOperation constructs one portable backup write operation.
func NewOperation(cfg OperationConfig) *Operation {
	must.NotBeNilf(cfg.Source, "backup source is required")
	must.NotBeNilf(cfg.Blobs, "backup blob source is required")
	must.NotBeNilf(cfg.Configuration, "backup configuration is required")
	must.NotBeNilf(cfg.Publisher, "backup publisher is required")
	return &Operation{
		source:        cfg.Source,
		blobs:         cfg.Blobs,
		configuration: cfg.Configuration,
		publisher:     cfg.Publisher,
	}
}

// WriteRequest selects the source boards and destination file for one backup.
type WriteRequest struct {
	// Destination is the output file replaced only after complete generation.
	Destination string // required

	// Selection identifies every board or explicit complete board IDs.
	Selection Selection // required
}

// WriteResult reports the retained source revision and archive population.
type WriteResult struct {
	// Destination is the output path supplied in the request.
	Destination string

	// SourceRevision is the canonical source revision represented by every board.
	SourceRevision int64

	// Boards is the number of complete board snapshots in the archive.
	Boards int

	// Blobs is the number of unique attachment bodies in the archive.
	Blobs int
}

// Write captures one source revision and atomically publishes its archive.
func (o *Operation) Write(
	ctx context.Context,
	request WriteRequest,
) (WriteResult, error) {
	if strings.TrimSpace(request.Destination) == "" {
		return WriteResult{}, errkind.Errorf(
			errkind.InvalidInput,
			"backup destination is required",
		)
	}
	if err := request.Selection.validate(); err != nil {
		return WriteResult{}, err
	}

	before, err := o.configuration.ReadStoreConfiguration(ctx)
	if err != nil {
		return WriteResult{}, fmt.Errorf("read source store configuration: %w", err)
	}
	if err := before.Validate(); err != nil {
		return WriteResult{}, fmt.Errorf("source store configuration: %w", err)
	}
	captured, err := o.source.Capture(ctx, request.Selection, before)
	if err != nil {
		return WriteResult{}, fmt.Errorf("capture source boards: %w", err)
	}
	after, err := o.configuration.ReadStoreConfiguration(ctx)
	if err != nil {
		return WriteResult{}, fmt.Errorf("reread source store configuration: %w", err)
	}
	if err := after.Validate(); err != nil {
		return WriteResult{}, fmt.Errorf("source store configuration after capture: %w", err)
	}
	if !before.Equal(after) {
		return WriteResult{}, errkind.Errorf(
			errkind.Conflict,
			"source store configuration changed while capturing the backup",
		)
	}

	descriptors, err := validateCapture(captured)
	if err != nil {
		return WriteResult{}, err
	}
	err = o.publisher.Publish(
		ctx,
		request.Destination,
		func(destination io.Writer) error {
			return o.writeArchive(ctx, destination, captured, descriptors)
		},
	)
	if err != nil {
		return WriteResult{}, fmt.Errorf("publish backup: %w", err)
	}
	return WriteResult{
		Destination:    request.Destination,
		SourceRevision: captured.SourceRevision,
		Boards:         len(captured.Boards),
		Blobs:          len(descriptors),
	}, nil
}

func validateCapture(captured Capture) ([]attachment.BlobDescriptor, error) {
	if strings.TrimSpace(captured.SourceLineageID) == "" {
		return nil, errors.New("backup source lineage is required")
	}
	if captured.SourceRevision < 0 {
		return nil, errors.New("backup source revision cannot be negative")
	}
	byDigest := make(map[attachment.Digest]attachment.BlobDescriptor)
	for _, value := range captured.Boards {
		if value.Snapshot.SourceLineageID != captured.SourceLineageID ||
			value.Snapshot.SourceRevision != captured.SourceRevision {
			return nil, fmt.Errorf(
				"backup board %q does not match retained source revision",
				value.Snapshot.Board.ID,
			)
		}
		for _, metadata := range value.Snapshot.Attachments {
			descriptor := metadata.Blob
			if err := descriptor.Validate(); err != nil {
				return nil, fmt.Errorf(
					"backup board %q attachment %q: %w",
					value.Snapshot.Board.ID,
					metadata.ID,
					err,
				)
			}
			prior, found := byDigest[descriptor.Digest]
			if found && prior != descriptor {
				return nil, fmt.Errorf(
					"backup blob %s has conflicting descriptors",
					descriptor.Digest,
				)
			}
			byDigest[descriptor.Digest] = descriptor
		}
	}
	descriptors := make([]attachment.BlobDescriptor, 0, len(byDigest))
	for _, descriptor := range byDigest {
		descriptors = append(descriptors, descriptor)
	}
	slices.SortFunc(descriptors, func(left, right attachment.BlobDescriptor) int {
		return strings.Compare(left.Digest.String(), right.Digest.String())
	})
	return descriptors, nil
}

func (o *Operation) writeArchive(
	ctx context.Context,
	destination io.Writer,
	captured Capture,
	descriptors []attachment.BlobDescriptor,
) (err error) {
	archive := NewWriter(destination)
	defer func() { err = errors.Join(err, archive.Close()) }()
	for _, sourceProject := range captured.Projects {
		if err := archive.AddProject(sourceProject); err != nil {
			return err
		}
	}
	for _, capturedBoard := range captured.Boards {
		if err := archive.AddBoard(
			capturedBoard.ProjectID,
			capturedBoard.Snapshot,
		); err != nil {
			return err
		}
	}
	for _, descriptor := range descriptors {
		if err := ctx.Err(); err != nil {
			return err
		}
		reader, err := o.blobs.OpenCopyBlob(ctx, descriptor)
		if err != nil {
			return fmt.Errorf("open source blob %s: %w", descriptor.Digest, err)
		}
		writeErr := archive.AddBlob(descriptor, reader)
		closeErr := reader.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return fmt.Errorf("write source blob %s: %w", descriptor.Digest, err)
		}
	}
	return nil
}

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

// CaptureDestination receives selected backup state while its source view is
// retained.
type CaptureDestination interface {
	// AddProject adds one project referenced by a selected board.
	AddProject(project.Snapshot) error

	// AddBoard consumes one selected board record stream before returning.
	AddBoard(project.ID, board.ID, boardcopy.RecordSequence) error
}

// CaptureResult identifies the retained source revision and captured
// population.
type CaptureResult struct {
	// SourceLineageID identifies the source store persistence history.
	SourceLineageID string

	// SourceRevision is the canonical revision retained during capture.
	SourceRevision int64

	// Projects is the number of source projects sent to the destination.
	Projects int

	// Boards is the number of selected boards sent to the destination.
	Boards int
}

// Source captures selected semantic board state from one retained store view.
type Source interface {
	// Capture streams selected state to destination and releases its persistence
	// view before returning.
	Capture(
		context.Context,
		Selection,
		configuration.Overrides,
		CaptureDestination,
	) (CaptureResult, error)
}

// BlobSource opens verified committed attachment content.
type BlobSource interface {
	// OpenCopyBlob returns a verified reader owned by the caller.
	OpenCopyBlob(
		context.Context,
		attachment.BlobDescriptor,
	) (io.ReadCloser, error)
}

//go:generate go tool mockgen -destination mocks_test.go -package backup -typed -write_package_comment=false . StoreConfiguration

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

	// Projects is the number of project records in the archive.
	Projects int

	// Boards is the number of complete board publications in the archive.
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
	var written WriteResult
	err = o.publisher.Publish(
		ctx,
		request.Destination,
		func(destination io.Writer) error {
			var err error
			written, err = o.writeArchive(
				ctx,
				destination,
				request.Selection,
				before,
			)
			return err
		},
	)
	if err != nil {
		return WriteResult{}, fmt.Errorf("publish backup: %w", err)
	}
	written.Destination = request.Destination
	return written, nil
}

func (o *Operation) writeArchive(
	ctx context.Context,
	destination io.Writer,
	selection Selection,
	storeOverrides configuration.Overrides,
) (result WriteResult, err error) {
	archive := NewWriter(destination)
	defer func() { err = errors.Join(err, archive.Close()) }()
	captured, err := o.source.Capture(
		ctx,
		selection,
		storeOverrides,
		archive,
	)
	if err != nil {
		return WriteResult{}, fmt.Errorf("capture source boards: %w", err)
	}
	if err := validateCapture(captured, archive); err != nil {
		return WriteResult{}, fmt.Errorf("capture source boards: %w", err)
	}

	after, err := o.configuration.ReadStoreConfiguration(ctx)
	if err != nil {
		return WriteResult{}, fmt.Errorf(
			"capture source boards: reread source store configuration: %w",
			err,
		)
	}
	if err := after.Validate(); err != nil {
		return WriteResult{}, fmt.Errorf(
			"capture source boards: source store configuration after capture: %w",
			err,
		)
	}
	if !storeOverrides.Equal(after) {
		return WriteResult{}, fmt.Errorf(
			"capture source boards: %w",
			errkind.Errorf(
				errkind.Conflict,
				"source store configuration changed while capturing the backup",
			),
		)
	}

	descriptors := archive.ReferencedBlobs()
	for _, descriptor := range descriptors {
		if err := ctx.Err(); err != nil {
			return WriteResult{}, fmt.Errorf("capture source boards: %w", err)
		}
		reader, err := o.blobs.OpenCopyBlob(ctx, descriptor)
		if err != nil {
			return WriteResult{}, fmt.Errorf(
				"capture source boards: open source blob %s: %w",
				descriptor.Digest,
				err,
			)
		}
		writeErr := archive.AddBlob(descriptor, reader)
		closeErr := reader.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return WriteResult{}, fmt.Errorf(
				"capture source boards: write source blob %s: %w",
				descriptor.Digest,
				err,
			)
		}
	}
	result = WriteResult{
		SourceRevision: captured.SourceRevision,
		Projects:       captured.Projects,
		Boards:         captured.Boards,
		Blobs:          len(descriptors),
	}
	return result, nil
}

func validateCapture(captured CaptureResult, archive *Writer) error {
	if strings.TrimSpace(captured.SourceLineageID) == "" {
		return errors.New("backup source lineage is required")
	}
	if captured.SourceRevision < 0 {
		return errors.New("backup source revision cannot be negative")
	}
	if captured.Projects != len(archive.manifest.Projects) {
		return fmt.Errorf(
			"backup source reported %d projects after capturing %d",
			captured.Projects,
			len(archive.manifest.Projects),
		)
	}
	if captured.Boards != len(archive.manifest.Boards) {
		return fmt.Errorf(
			"backup source reported %d boards after capturing %d",
			captured.Boards,
			len(archive.manifest.Boards),
		)
	}
	for _, publication := range archive.manifest.Boards {
		if publication.GetSourceLineageId() != captured.SourceLineageID ||
			publication.GetSourceRevision() != captured.SourceRevision {
			return fmt.Errorf(
				"backup board %q does not match retained source revision",
				publication.GetSourceBoardId(),
			)
		}
	}
	return nil
}

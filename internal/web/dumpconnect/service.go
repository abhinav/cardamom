// Package dumpconnect exposes deterministic dump artifacts through Connect.
package dumpconnect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/dump"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web"
)

const dumpFileChunkSize = 64 * 1024

//go:generate go tool mockgen -destination mocks_test.go -package dumpconnect -typed -write_package_comment=false . Renderer,RendererFactory

// Renderer produces one complete deterministic dump artifact.
type Renderer interface {
	// Render selects and renders one coherent artifact.
	Render(context.Context, dump.RenderRequest) (dump.RenderedDump, error)
}

// RendererFactory opens render operations for explicitly selected boards.
type RendererFactory interface {
	// Renderer returns the render operation for boardID.
	Renderer(context.Context, board.ID) (Renderer, error)
}

// Config supplies DumpService collaborators.
type Config struct {
	// Renderers opens board-scoped render operations.
	Renderers RendererFactory // required
}

// Service adapts deterministic rendering to generated DumpService streams.
type Service struct {
	privatev1connect.UnimplementedDumpServiceHandler
	renderers RendererFactory
}

var _ privatev1connect.DumpServiceHandler = (*Service)(nil)

// New constructs a DumpService handler from board-scoped render operations.
func New(cfg Config) *Service {
	must.NotBeNilf(cfg.Renderers, "dumpconnect: renderer factory is required")
	return &Service{renderers: cfg.Renderers}
}

// RenderDump streams one manifest followed by every generated file in
// canonical path order.
func (s *Service) RenderDump(
	ctx context.Context,
	request *connect.Request[privatev1.RenderDumpRequest],
	stream *connect.ServerStream[privatev1.RenderDumpResponse],
) error {
	boardID, err := board.NewID(request.Msg.GetBoardId())
	if err != nil {
		return web.FromError(err)
	}
	selection, err := selectionFromProto(request.Msg.GetSelection())
	if err != nil {
		return web.FromError(err)
	}
	renderer, err := s.renderers.Renderer(ctx, boardID)
	if err != nil {
		return web.FromError(err)
	}
	if renderer == nil {
		return web.FromError(errors.New("dump renderer factory returned a nil renderer"))
	}
	rendered, err := renderer.Render(ctx, dump.RenderRequest{Selection: selection})
	if err != nil {
		return web.FromError(err)
	}
	manifest, err := manifestFromRendered(rendered)
	if err != nil {
		return web.FromError(err)
	}
	if err := stream.Send(&privatev1.RenderDumpResponse{
		Frame: &privatev1.RenderDumpResponse_Manifest{Manifest: manifest},
	}); err != nil {
		return web.FromError(err)
	}
	for index, file := range rendered.Files {
		if err := streamGeneratedFile(stream, uint32(index), file); err != nil {
			return web.FromError(err)
		}
	}
	return nil
}

func streamGeneratedFile(
	stream *connect.ServerStream[privatev1.RenderDumpResponse],
	index uint32,
	file *dump.GeneratedFile,
) (err error) {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf(
				"close generated file %q: %w", file.Path(), closeErr,
			))
		}
	}()

	buffer := make([]byte, dumpFileChunkSize)
	for offset := int64(0); offset < file.Size(); {
		chunkSize := min(int64(len(buffer)), file.Size()-offset)
		read, readErr := io.ReadFull(reader, buffer[:chunkSize])
		if read > 0 {
			if err := stream.Send(&privatev1.RenderDumpResponse{
				Frame: &privatev1.RenderDumpResponse_FileChunk{FileChunk: &privatev1.DumpFileChunk{
					FileIndex: index,
					Offset:    uint64(offset),
					Content:   bytes.Clone(buffer[:read]),
				}},
			}); err != nil {
				return fmt.Errorf("send generated file %q at offset %d: %w", file.Path(), offset, err)
			}
			offset += int64(read)
		}
		if readErr != nil {
			return fmt.Errorf("read generated file %q at offset %d: %w", file.Path(), offset, readErr)
		}
	}
	return nil
}

func selectionFromProto(value *privatev1.DumpSelection) (dump.Selection, error) {
	if value == nil {
		return dump.Selection{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: dump selection is required",
		)
	}
	switch mode := value.GetMode().(type) {
	case *privatev1.DumpSelection_WholeBoard:
		return dump.WholeBoard(), nil
	case *privatev1.DumpSelection_Issues:
		if mode.Issues == nil || len(mode.Issues.GetIssueIds()) == 0 {
			return dump.Selection{}, errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: dump issue selection requires an issue ID",
			)
		}
		if mode.Issues.GetIncludeDescendants() {
			return dump.SelectedIssues(mode.Issues.GetIssueIds()...), nil
		}
		return dump.NamedIssuesOnly(mode.Issues.GetIssueIds()...), nil
	default:
		return dump.Selection{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: dump selection is required",
		)
	}
}

func manifestFromRendered(rendered dump.RenderedDump) (*privatev1.DumpManifest, error) {
	if rendered.Revision < 0 {
		return nil, errors.New("dump revision must not be negative")
	}
	if rendered.IssueCount < 0 || uint64(rendered.IssueCount) > math.MaxUint32 {
		return nil, errors.New("dump issue count is out of range")
	}
	if uint64(len(rendered.Files)) > math.MaxUint32 {
		return nil, errors.New("dump file count is out of range")
	}
	selection, err := selectionToProto(rendered.Selection)
	if err != nil {
		return nil, err
	}
	files := make([]*privatev1.DumpFileManifest, 0, len(rendered.Files))
	for _, file := range rendered.Files {
		if file == nil {
			return nil, errors.New("dump generated file is required")
		}
		files = append(files, &privatev1.DumpFileManifest{
			Path: file.Path(), Identity: file.Identity(), SizeBytes: uint64(file.Size()),
		})
	}
	return &privatev1.DumpManifest{
		ProjectId: rendered.Provenance.ProjectID, ProjectName: rendered.Provenance.ProjectName,
		BoardId: rendered.Provenance.BoardID, BoardName: rendered.Provenance.BoardName,
		Revision: uint64(rendered.Revision), Selection: selection,
		IssueCount: uint32(rendered.IssueCount), Files: files,
	}, nil
}

func selectionToProto(selection dump.Selection) (*privatev1.DumpSelection, error) {
	switch selection.Mode {
	case dump.SelectionWholeBoard:
		return &privatev1.DumpSelection{Mode: &privatev1.DumpSelection_WholeBoard{
			WholeBoard: &privatev1.WholeBoardDumpSelection{},
		}}, nil
	case dump.SelectionIssues:
		return &privatev1.DumpSelection{Mode: &privatev1.DumpSelection_Issues{
			Issues: &privatev1.IssueDumpSelection{
				IssueIds:           selection.IssueIDs,
				IncludeDescendants: selection.Descendants == dump.IncludeDescendants,
			},
		}}, nil
	default:
		return nil, errors.New("dump rendered an unsupported selection")
	}
}

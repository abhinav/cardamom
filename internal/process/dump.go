package process

import (
	"context"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/dump"
	"go.abhg.dev/cardamom/internal/project"
)

func (r *namespaceRuntime) dumpRenderer(
	ctx context.Context,
	boardID board.ID,
) (*dump.Service, error) {
	selected, err := r.boards.Get(ctx, boardID)
	if err != nil {
		return nil, err
	}
	projectSelector, err := project.NewSelector(selected.ProjectID())
	if err != nil {
		return nil, err
	}
	namespace, err := r.projects.Resolve(ctx, &projectSelector)
	if err != nil {
		return nil, err
	}
	repository, err := r.boardRepository(boardID)
	if err != nil {
		return nil, err
	}
	attachments, err := r.attachmentService()
	if err != nil {
		return nil, err
	}
	return dump.NewService(dump.ServiceConfig{
		Reader: repository, Attachments: attachments,
		Provenance: dump.Provenance{
			ProjectID: selected.ProjectID(), ProjectName: namespace.Name(),
			BoardID: selected.ID().String(), BoardName: selected.Name(),
		},
	})
}

func provideDumpPublicationService(
	invocation *cli.Invocation,
	runtime *namespaceRuntime,
	selected *board.State,
) (cli.DumpOperation, error) {
	renderer, err := runtime.dumpRenderer(invocation.Context, selected.ID())
	if err != nil {
		return nil, err
	}
	return dump.NewPublicationService(renderer, &dump.FilePublisher{})
}

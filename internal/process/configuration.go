package process

import (
	"context"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/project"
)

// configurationOperations defers target selection until the command has
// chosen board- or project-scoped configuration behavior. This keeps direct
// project operations independent of checkout and ambient board selection.
type configurationOperations struct {
	runtime       *namespaceRuntime // required
	boardSelector string
	boardIssueIDs []string
}

var _ cli.ConfigurationOperations = (*configurationOperations)(nil)

// provideConfigurationOperations captures invocation selectors without
// resolving either target. Individual operations resolve only the target the
// command requests.
func provideConfigurationOperations(
	invocation *cli.Invocation,
	runtime *namespaceRuntime,
) cli.ConfigurationOperations {
	return &configurationOperations{
		runtime:       runtime,
		boardSelector: invocation.Board,
		boardIssueIDs: invocation.BoardIssueIDs,
	}
}

// ResolveBoard selects the invocation's board and resolves all four layers.
func (o *configurationOperations) ResolveBoard(
	ctx context.Context,
) (configuration.View, error) {
	selected, err := o.selectBoard(ctx)
	if err != nil {
		return configuration.View{}, err
	}
	return o.runtime.configuration.Resolve(ctx, selected.ID())
}

// ResolveProject resolves a stable ID or exact name without selecting a board.
func (o *configurationOperations) ResolveProject(
	ctx context.Context,
	selector project.Selector,
) (configuration.ProjectView, error) {
	selected, err := o.runtime.projects.Resolve(ctx, &selector)
	if err != nil {
		return configuration.ProjectView{}, err
	}
	return o.runtime.configuration.ResolveProject(ctx, selected.ID())
}

// UpdateBoard selects the invocation's board before applying the requested
// store, project, or board layer patch.
func (o *configurationOperations) UpdateBoard(
	ctx context.Context,
	invocation configuration.Invocation,
	scope configuration.Scope,
	patch configuration.Patch,
) (configuration.View, error) {
	selected, err := o.selectBoard(ctx)
	if err != nil {
		return configuration.View{}, err
	}
	return o.runtime.configuration.Update(ctx, invocation, configuration.UpdateRequest{
		BoardID: selected.ID(),
		Scope:   scope,
		Patch:   patch,
	})
}

// UpdateProject resolves a stable ID or exact name before applying a patch to
// that project's layer. It never uses the captured board selectors.
func (o *configurationOperations) UpdateProject(
	ctx context.Context,
	invocation configuration.Invocation,
	selector project.Selector,
	patch configuration.Patch,
) (configuration.ProjectView, error) {
	selected, err := o.runtime.projects.Resolve(ctx, &selector)
	if err != nil {
		return configuration.ProjectView{}, err
	}
	return o.runtime.configuration.UpdateProject(
		ctx,
		invocation,
		configuration.ProjectUpdateRequest{
			ProjectID: selected.ID(),
			Patch:     patch,
		},
	)
}

func (o *configurationOperations) selectBoard(
	ctx context.Context,
) (*board.State, error) {
	return o.runtime.selectBoard(ctx, o.boardSelector, o.boardIssueIDs)
}

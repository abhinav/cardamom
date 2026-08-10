package inspection

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/project"
)

func TestService_ShowResolvesProjectConfigurationAndBoards(t *testing.T) {
	selected := testProject(t, "project-one", "Mission")
	first := testBoard(t, "board-alpha", selected.ID(), "Alpha")
	second := testBoard(t, "board-zulu", selected.ID(), "Zulu")
	prefix, err := configuration.NewPrefix("mission-")
	require.NoError(t, err)
	view := configuration.ProjectView{Effective: configuration.Configuration{
		Issue: configuration.IssueConfiguration{ID: configuration.IssueIDConfiguration{
			Prefix: prefix, Strategy: configuration.IDStrategySequential,
		}},
	}}
	selector, err := project.NewSelector("Mission")
	require.NoError(t, err)
	projects := projectResolverFunc(func(
		_ context.Context,
		got *project.Selector,
	) (*project.State, error) {
		assert.Equal(t, selector, *got)
		return selected, nil
	})
	configurations := projectConfigurationFunc(func(
		_ context.Context,
		got project.ID,
	) (configuration.ProjectView, error) {
		assert.Equal(t, selected.ID(), got)
		return view, nil
	})
	boards := projectBoardsFunc(func(
		_ context.Context,
		got project.ID,
	) ([]*board.State, error) {
		assert.Equal(t, selected.ID(), got)
		return []*board.State{first, second}, nil
	})

	detail, err := NewService(projects, configurations, boards).Show(
		t.Context(), selector,
	)
	require.NoError(t, err)

	assert.Same(t, selected, detail.Project)
	assert.Equal(t, view, detail.Configuration)
	assert.Equal(t, []*board.State{first, second}, detail.Boards)
}

type projectResolverFunc func(context.Context, *project.Selector) (*project.State, error)

func (f projectResolverFunc) Resolve(
	ctx context.Context,
	selector *project.Selector,
) (*project.State, error) {
	return f(ctx, selector)
}

type projectConfigurationFunc func(
	context.Context,
	project.ID,
) (configuration.ProjectView, error)

func (f projectConfigurationFunc) ResolveProject(
	ctx context.Context,
	id project.ID,
) (configuration.ProjectView, error) {
	return f(ctx, id)
}

type projectBoardsFunc func(context.Context, project.ID) ([]*board.State, error)

func (f projectBoardsFunc) ListProjectBoards(
	ctx context.Context,
	id project.ID,
) ([]*board.State, error) {
	return f(ctx, id)
}

func testProject(t *testing.T, id, name string) *project.State {
	t.Helper()
	state, err := project.Load(project.Snapshot{
		ID: project.ID(id), Name: name, Created: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return state
}

func testBoard(
	t *testing.T,
	id string,
	projectID project.ID,
	name string,
) *board.State {
	t.Helper()
	state, err := board.Load(board.Snapshot{
		ID: board.ID(id), ProjectID: projectID.String(), Name: name,
		Created: time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)
	return state
}

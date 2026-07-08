package board

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceCreatesBoard(t *testing.T) {
	created := testBoardState(t, Snapshot{
		ID:        testBoardID(t, "board-one"),
		ProjectID: "project-one",
		Name:      "Development",
		Created:   time.Unix(10, 0).UTC(),
	})
	repository := &fakeRepository{created: created}
	service := NewService(repository, repository)
	request := CreateRequest{ProjectID: created.ProjectID(), Name: "Development"}

	result, err := service.Create(t.Context(), NewInvocation("  captain  "), request)
	require.NoError(t, err)

	assert.Same(t, created, result)
	assert.Equal(t, request, repository.createRequest)
}

func TestServiceReadsBoardCatalog(t *testing.T) {
	primary := testBoardState(t, Snapshot{
		ID:        testBoardID(t, "board-one"),
		ProjectID: "project-one",
		Name:      "Development",
		Created:   time.Unix(10, 0).UTC(),
	})
	secondary := testBoardState(t, Snapshot{
		ID:        testBoardID(t, "board-two"),
		ProjectID: "project-one",
		Name:      "Operations",
		Created:   time.Unix(20, 0).UTC(),
	})
	repository := &fakeRepository{
		states: []*State{primary, secondary},
		board:  secondary,
		sole:   primary,
	}
	service := NewService(repository, repository)

	listed, err := service.List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []*State{primary, secondary}, listed)

	selected, err := service.Get(t.Context(), secondary.ID())
	require.NoError(t, err)
	assert.Same(t, secondary, selected)

	sole, err := service.Sole(t.Context())
	require.NoError(t, err)
	assert.Same(t, primary, sole)
}

func TestServiceEditsBoardSettings(t *testing.T) {
	edited := testBoardState(t, Snapshot{
		ID:        testBoardID(t, "board-one"),
		ProjectID: "project-one",
		Name:      "Operations",
		Created:   time.Unix(10, 0).UTC(),
	})
	repository := &fakeRepository{edited: edited}
	service := NewService(repository, repository)
	name := "Operations"
	request := EditRequest{BoardID: edited.ID(), Settings: SettingsEdit{Name: &name}}

	result, err := service.EditSettings(t.Context(), NewInvocation("captain"), request)
	require.NoError(t, err)

	assert.Same(t, edited, result)
	assert.Equal(t, request, repository.editRequest)
}

type fakeRepository struct {
	states []*State
	board  *State
	sole   *State

	createRequest CreateRequest
	created       *State

	editRequest EditRequest
	edited      *State
}

func (f *fakeRepository) ListAllBoards(context.Context) ([]*State, error) {
	return f.states, nil
}

func (f *fakeRepository) Board(context.Context, ID) (*State, error) {
	return f.board, nil
}

func (f *fakeRepository) SoleBoard(context.Context) (*State, error) {
	return f.sole, nil
}

func (f *fakeRepository) CreateBoard(
	_ context.Context,
	request CreateRequest,
) (*State, error) {
	f.createRequest = request
	return f.created, nil
}

func (f *fakeRepository) EditBoardSettings(
	_ context.Context,
	request EditRequest,
) (*State, error) {
	f.editRequest = request
	return f.edited, nil
}

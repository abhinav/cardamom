package board

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestServiceCreatesBoard(t *testing.T) {
	created := testBoardState(t, Snapshot{
		ID:        testBoardID(t, "board-one"),
		ProjectID: "project-one",
		Name:      "Development",
		Created:   time.Unix(10, 0).UTC(),
	})
	request := CreateRequest{ProjectID: created.ProjectID(), Name: "Development"}
	changes := NewMockChanges(gomock.NewController(t))
	changes.EXPECT().CreateBoard(gomock.Any(), request).Return(created, nil)
	service := NewService(NewMockCatalog(gomock.NewController(t)), changes)

	result, err := service.Create(t.Context(), NewInvocation("  captain  "), request)
	require.NoError(t, err)

	assert.Same(t, created, result)
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
	catalog := NewMockCatalog(gomock.NewController(t))
	catalog.EXPECT().ListAllBoards(gomock.Any()).Return([]*State{primary, secondary}, nil)
	catalog.EXPECT().Board(gomock.Any(), secondary.ID()).Return(secondary, nil)
	catalog.EXPECT().SoleBoard(gomock.Any()).Return(primary, nil)
	service := NewService(catalog, NewMockChanges(gomock.NewController(t)))

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
	name := "Operations"
	request := EditRequest{BoardID: edited.ID(), Settings: SettingsEdit{Name: &name}}
	changes := NewMockChanges(gomock.NewController(t))
	changes.EXPECT().EditBoardSettings(gomock.Any(), request).Return(edited, nil)
	service := NewService(NewMockCatalog(gomock.NewController(t)), changes)

	result, err := service.EditSettings(t.Context(), NewInvocation("captain"), request)
	require.NoError(t, err)

	assert.Same(t, edited, result)
}

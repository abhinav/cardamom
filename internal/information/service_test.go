package information

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/project"
	"go.uber.org/mock/gomock"
)

func TestService_ReadReturnsTypedStoreInformation(t *testing.T) {
	projectID, err := project.NewID("project-one")
	require.NoError(t, err)
	selectedProject, err := project.Load(project.Snapshot{
		ID: projectID, Name: "Mission", Created: time.Unix(10, 0).UTC(),
	})
	require.NoError(t, err)
	boardID, err := board.NewID("board-one")
	require.NoError(t, err)
	selectedBoard, err := board.Load(board.Snapshot{
		ID: boardID, ProjectID: projectID.String(), Name: "Operations",
		Created: time.Unix(20, 0).UTC(),
	})
	require.NoError(t, err)
	effective := configuration.Defaults()
	snapshot := Snapshot{
		Schema:   Schema{DatabaseVersion: 12, CodeVersion: 13},
		Revision: Revision{Current: 42},
		Issues:   IssueInventory{Total: 3},
	}
	reader := NewMockReader(gomock.NewController(t))
	reader.EXPECT().Read(gomock.Any()).Return(snapshot, nil)
	readers := NewMockReaders(gomock.NewController(t))
	readers.EXPECT().Reader(boardID, effective).Return(reader, nil)
	projects := NewMockProjects(gomock.NewController(t))
	projects.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(selectedProject, nil)
	boards := NewMockBoards(gomock.NewController(t))
	boards.EXPECT().Get(gomock.Any(), boardID).Return(selectedBoard, nil)
	configurations := NewMockConfigurations(gomock.NewController(t))
	configurations.EXPECT().ResolveConfiguration(gomock.Any(), boardID).Return(effective, nil)
	service := NewService(ServiceConfig{
		Store: Store{
			Directory:    "/stores/mission",
			DatabasePath: "/stores/mission/board.sqlite3",
		},
		Projects:       projects,
		Boards:         boards,
		Configurations: configurations,
		Readers:        readers,
	})

	report, err := service.Read(t.Context(), Request{BoardID: boardID})
	require.NoError(t, err)

	assert.Equal(t, Store{
		Directory:    "/stores/mission",
		DatabasePath: "/stores/mission/board.sqlite3",
	}, report.Store)
	assert.Same(t, selectedProject, report.Project)
	assert.Same(t, selectedBoard, report.Board)
	assert.Equal(t, effective, report.Configuration)
	assert.Equal(t, snapshot.Schema, report.Schema)
	assert.Equal(t, snapshot.Revision, report.Revision)
	assert.Equal(t, snapshot.Issues, report.Issues)
}

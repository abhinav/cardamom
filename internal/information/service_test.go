package information

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
	reader := &fakeReader{snapshot: snapshot}
	readers := &fakeReaders{reader: reader}
	service := NewService(ServiceConfig{
		Store: Store{
			Directory:    "/stores/mission",
			DatabasePath: "/stores/mission/board.sqlite3",
		},
		Projects:       &fakeProjects{state: selectedProject},
		Boards:         &fakeBoards{state: selectedBoard},
		Configurations: &fakeConfigurations{effective: effective},
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
	assert.Equal(t, boardID, readers.boardID)
	assert.Equal(t, effective, readers.configuration)
}

type fakeProjects struct{ state *project.State }

func (f *fakeProjects) Resolve(
	context.Context,
	*project.Selector,
) (*project.State, error) {
	return f.state, nil
}

type fakeBoards struct{ state *board.State }

func (f *fakeBoards) Get(context.Context, board.ID) (*board.State, error) {
	return f.state, nil
}

type fakeConfigurations struct {
	effective configuration.Configuration
}

func (f *fakeConfigurations) ResolveConfiguration(
	context.Context,
	board.ID,
) (configuration.Configuration, error) {
	return f.effective, nil
}

type fakeReaders struct {
	reader        Reader
	boardID       board.ID
	configuration configuration.Configuration
}

func (f *fakeReaders) Reader(
	boardID board.ID,
	effective configuration.Configuration,
) (Reader, error) {
	f.boardID = boardID
	f.configuration = effective
	return f.reader, nil
}

type fakeReader struct{ snapshot Snapshot }

func (f *fakeReader) Read(context.Context) (Snapshot, error) {
	return f.snapshot, nil
}

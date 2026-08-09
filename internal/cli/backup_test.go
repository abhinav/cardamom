package cli

import (
	"bytes"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestBackupCommand_rejectsConflictingSelections(t *testing.T) {
	t.Setenv("CARDAMOM_STORE", "")
	t.Setenv("CARDAMOM_BOARD", "")
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name: "IncludeAndAll",
			args: []string{
				"backup", "archive.cardamom", "--include-board", "Bridge", "--all",
			},
			wantStderr: "error: --include-board cannot be combined with --all\n",
		},
		{
			name: "ExplicitBoardAndInclude",
			args: []string{
				"--board", "Bridge", "backup", "archive.cardamom",
				"--include-board", "Engineering",
			},
			wantStderr: "error: --board cannot be combined with --include-board or --all\n",
		},
		{
			name: "ExplicitBoardAndAll",
			args: []string{
				"--board", "Bridge", "backup", "archive.cardamom", "--all",
			},
			wantStderr: "error: --board cannot be combined with --include-board or --all\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			operation := NewMockBackupOperation(gomock.NewController(t))
			app, err := New(
				testConfig(&stdout, &stderr),
				kong.BindTo(operation, (*BackupOperation)(nil)),
			)
			require.NoError(t, err)

			assert.Equal(t, ExitUsage, app.Run(t.Context(), tt.args))
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func TestBackupCommand_ambientBoardDoesNotConflictWithAggregateSelection(t *testing.T) {
	t.Setenv("CARDAMOM_STORE", "")
	t.Setenv("CARDAMOM_BOARD", "Bridge")
	var stdout, stderr bytes.Buffer
	result := BackupResult{
		Source: "/source/.cardamom", Destination: "archive.cardamom",
		Projects: 1, Boards: 2, Blobs: 0,
	}
	request := BackupRequest{
		Destination:   "archive.cardamom",
		IncludeBoards: []string{"Engineering"},
	}
	operation := NewMockBackupOperation(gomock.NewController(t))
	operation.EXPECT().Backup(gomock.Any(), request).Return(result, nil)
	app, err := New(
		testConfig(&stdout, &stderr),
		kong.BindTo(operation, (*BackupOperation)(nil)),
	)
	require.NoError(t, err)

	exitCode := app.Run(t.Context(), []string{
		"backup", "archive.cardamom", "--include-board", "Engineering",
	})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Equal(
		t,
		"backed up 1 project, 2 boards, and 0 blobs from /source/.cardamom to archive.cardamom\n",
		stdout.String(),
	)
	assert.Empty(t, stderr.String())
}

func TestBackupCommand_rendersOneJSONSummary(t *testing.T) {
	t.Setenv("CARDAMOM_STORE", "")
	t.Setenv("CARDAMOM_BOARD", "")
	var stdout, stderr bytes.Buffer
	result := BackupResult{
		Source: "/source/.cardamom", Destination: "/tmp/archive.cardamom",
		Projects: 2, Boards: 3, Blobs: 4,
	}
	request := BackupRequest{Destination: "/tmp/archive.cardamom", All: true}
	operation := NewMockBackupOperation(gomock.NewController(t))
	operation.EXPECT().Backup(gomock.Any(), request).Return(result, nil)
	app, err := New(
		testConfig(&stdout, &stderr),
		kong.BindTo(operation, (*BackupOperation)(nil)),
	)
	require.NoError(t, err)

	exitCode := app.Run(t.Context(), []string{
		"--json", "backup", "/tmp/archive.cardamom", "--all",
	})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.JSONEq(t, `{
		"source":"/source/.cardamom",
		"destination":"/tmp/archive.cardamom",
		"projects":2,
		"boards":3,
		"blobs":4
	}`, stdout.String())
	assert.Equal(t, 1, bytes.Count(stdout.Bytes(), []byte("\n")))
	assert.Empty(t, stderr.String())
}

func TestRestoreCommand_passesDestinationAndRendersSummaries(t *testing.T) {
	t.Setenv("CARDAMOM_STORE", "")
	t.Setenv("CARDAMOM_BOARD", "")
	t.Run("Human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		result := RestoreResult{
			Source: "archive.cardamom", Destination: "/restore/.cardamom",
			Projects: 1, Boards: 2, Blobs: 3, AlreadyCompletedBoards: 1,
		}
		request := RestoreRequest{
			Source: "archive.cardamom", DestinationStore: "/restore/.cardamom",
			DestinationStoreExplicit: true,
		}
		operation := NewMockRestoreOperation(gomock.NewController(t))
		operation.EXPECT().Restore(gomock.Any(), request).Return(result, nil)
		app, err := New(
			testConfig(&stdout, &stderr),
			kong.BindTo(operation, (*RestoreOperation)(nil)),
		)
		require.NoError(t, err)

		exitCode := app.Run(t.Context(), []string{
			"--store", "/restore/.cardamom", "restore", "archive.cardamom",
		})

		assert.Equal(t, ExitSuccess, exitCode)
		assert.Equal(
			t,
			"restored 1 project, 2 boards, and 3 blobs from archive.cardamom to /restore/.cardamom (1 already completed)\n",
			stdout.String(),
		)
		assert.Empty(t, stderr.String())
	})

	t.Run("JSON", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		result := RestoreResult{
			Source: "archive.cardamom", Destination: "/restore/.cardamom",
			Projects: 1, Boards: 2, Blobs: 3, AlreadyCompletedBoards: 1,
		}
		request := RestoreRequest{
			Source: "archive.cardamom", DestinationStore: "/restore/.cardamom",
			DestinationStoreExplicit: true,
		}
		operation := NewMockRestoreOperation(gomock.NewController(t))
		operation.EXPECT().Restore(gomock.Any(), request).Return(result, nil)
		app, err := New(
			testConfig(&stdout, &stderr),
			kong.BindTo(operation, (*RestoreOperation)(nil)),
		)
		require.NoError(t, err)

		exitCode := app.Run(t.Context(), []string{
			"--json", "--store", "/restore/.cardamom", "restore", "archive.cardamom",
		})

		assert.Equal(t, ExitSuccess, exitCode)
		assert.JSONEq(t, `{
			"source":"archive.cardamom",
			"destination":"/restore/.cardamom",
			"projects":1,
			"boards":2,
			"blobs":3,
			"already_completed_boards":1
		}`, stdout.String())
		assert.Equal(t, 1, bytes.Count(stdout.Bytes(), []byte("\n")))
		assert.Empty(t, stderr.String())
	})
}

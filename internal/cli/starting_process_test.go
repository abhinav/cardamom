package cli

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/board/selection"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/komplete"
	"go.uber.org/mock/gomock"
)

func TestInitCommandForwardsSelectionAndRendersJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	boardName := "Mission board"
	result := InitResult{
		Dir: ".cardamom", IDPrefix: "mission-", IDStrategy: "random",
		SchemaVersion: 12, ConfigWritten: true, DatabaseWritten: true,
		ProjectCreated: true, ProjectID: new("project-one"), ProjectName: new("mission"),
		BoardCreated: true, BoardID: new("board-one"),
		BoardName: new(boardName), IgnoreOutcome: InitIgnoreStoragePatternsAdded,
	}
	invocation := testInvocation(t, &stdout, &stderr)
	invocation.Store = "/stores/mission"
	invocation.Output = newOutput(&stdout, &stderr, true, false)
	request := InitRequest{
		Store: "/stores/mission", ProjectPath: "project", IDPrefix: new("mission-"),
		BoardName: &boardName, ConfigMode: InitConfigSkipMissing,
	}
	operation := NewMockInitOperation(gomock.NewController(t))
	operation.EXPECT().Initialize(invocation.Context, request).Return(result, nil)

	err := (&initCommand{
		Path: "project", Prefix: new("mission-"), BoardName: &boardName, NoConfig: true,
	}).Run(invocation, operation)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"dir":".cardamom","id_prefix":"mission-","id_strategy":"random",
		"schema_version":12,"config_written":true,"db_written":true,
		"project_created":true,"project_id":"project-one","project_name":"mission",
		"board_created":true,"board_id":"board-one",
		"board_name":"Mission board","already_initialized":false
	}`, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestInitCommandPreservesOmittedPrefix(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result := InitResult{
		Dir: ".cardamom", BoardCreated: true,
	}
	invocation := testInvocation(t, &stdout, &stderr)
	invocation.Output = newOutput(&stdout, &stderr, false, true)
	var request InitRequest
	operation := NewMockInitOperation(gomock.NewController(t))
	operation.EXPECT().Initialize(invocation.Context, gomock.Any()).DoAndReturn(func(
		_ context.Context,
		got InitRequest,
	) (InitResult, error) {
		request = got
		return result, nil
	})

	require.NoError(t, (&initCommand{}).Run(invocation, operation))

	assert.Nil(t, request.IDPrefix)
}

func TestWriteInitResultReportsAttachmentBlobExclusions(t *testing.T) {
	tests := []struct {
		name        string
		giveOutcome InitIgnoreOutcome
		wantNotice  string
	}{
		{
			name:        "Automatic",
			giveOutcome: InitIgnoreStoragePatternsAdded,
			wantNotice:  "local database and attachment blob files",
		},
		{
			name:        "Manual",
			giveOutcome: InitIgnoreManual,
			wantNotice:  ".cardamom/board.sqlite3-wal\n.cardamom/blobs/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			output := newOutput(&stdout, &stderr, false, false)

			require.NoError(t, writeInitResult(output, InitResult{
				Dir: ".cardamom", BoardCreated: true, IgnoreOutcome: tt.giveOutcome,
			}))

			assert.Contains(t, stdout.String(), tt.wantNotice)
			assert.Empty(t, stderr.String())
		})
	}
}

func TestWriteInitResultRejectsUnsupportedIgnoreOutcome(t *testing.T) {
	var stdout, stderr bytes.Buffer
	output := newOutput(&stdout, &stderr, false, false)

	err := writeInitResult(output, InitResult{
		Dir: ".cardamom", BoardCreated: true, IgnoreOutcome: InitIgnoreOutcome(255),
	})

	assert.EqualError(t, err, "unsupported initialization ignore outcome 255")
}

func TestBoardCommandsDelegateNamespaceRulesAndRenderResults(t *testing.T) {
	primary := testBoard(t, "board-one", "project-one", "Primary", nil)
	description := "Shared mission context."
	secondary := testBoard(t, "board-two", "project-one", "Secondary", &description)
	catalog := &fakeProjectCatalog{
		projects: []*project.State{testProject(t, "project-one", "cardamom")},
		boards:   []*board.State{primary, secondary},
	}
	binding := &fakeBoardBinding{selected: secondary.ID()}
	boards := board.NewService(catalog, catalog)
	resolver := selection.NewResolver(boards, binding, fakeIssueBoardLocator{})
	projects := project.NewService(catalog, catalog)

	t.Run("ListJSONLines", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		invocation := testInvocation(t, &stdout, &stderr)
		invocation.Output = newOutput(&stdout, &stderr, true, false)

		require.NoError(t, (&boardListCommand{}).Run(invocation, boards))
		assert.Equal(
			t,
			"{\"id\":\"board-one\",\"project_id\":\"project-one\",\"name\":\"Primary\",\"created\":10}\n"+
				"{\"id\":\"board-two\",\"project_id\":\"project-one\",\"name\":\"Secondary\",\"created\":10}\n",
			stdout.String(),
		)
		assert.Empty(t, stderr.String())
	})

	t.Run("CreateResolvesProject", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		invocation := testInvocation(t, &stdout, &stderr)
		catalog.created = secondary

		err := (&boardCreateCommand{
			Name: "Delivery", Project: "project-one",
		}).Run(invocation, projects, boards)
		require.NoError(t, err)

		assert.Equal(t, board.CreateRequest{
			ProjectID: primary.ProjectID(), Name: "Delivery",
		}, catalog.createRequest)
		assert.Equal(t, "board-two\n", stdout.String())
	})

	t.Run("UsePersistsCheckoutSelection", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		invocation := testInvocation(t, &stdout, &stderr)

		require.NoError(t, (&boardUseCommand{Selector: "Primary"}).Run(invocation, resolver))
		assert.Equal(t, primary.ID(), binding.written)
		assert.Equal(t, "using board-one (Primary)\n", stdout.String())
	})

	t.Run("ShowUsesInvocationSelection", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		invocation := testInvocation(t, &stdout, &stderr)
		invocation.Board = "Primary"
		invocation.Output = newOutput(&stdout, &stderr, true, false)

		require.NoError(t, (&boardShowCommand{}).Run(invocation, resolver))
		assert.JSONEq(t, `{
			"id":"board-one","project_id":"project-one","name":"Primary",
			"created":10,"description":null
		}`, stdout.String())
	})

	t.Run("ShowHumanIncludesCreationTime", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		invocation := testInvocation(t, &stdout, &stderr)
		invocation.Board = "Primary"

		require.NoError(t, (&boardShowCommand{}).Run(invocation, resolver))
		assert.Equal(t, "ID:      board-one\n"+
			"Project: project-one\n"+
			"Name:    Primary\n"+
			"Created: 1970-01-01T00:00:10Z\n"+
			"Description: (none)\n", stdout.String())
	})

	t.Run("EditBuildsOneAtomicSettingsRequest", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		newName := "Delivery"
		markdown := "# Delivery context"
		invocation := testInvocation(t, &stdout, &stderr)
		catalog.edited = testBoard(t, "board-two", "project-one", newName, &markdown)

		err := (&boardEditCommand{
			Name: &newName, Description: &markdown,
		}).Run(invocation, boards, resolver, &invocation.Markdown)
		require.NoError(t, err)

		assert.Equal(t, secondary.ID(), catalog.editRequest.BoardID)
		assert.Equal(t, &newName, catalog.editRequest.Settings.Name)
		applied, applyErr := secondary.EditSettings(catalog.editRequest.Settings)
		require.NoError(t, applyErr)
		assert.Equal(t, catalog.edited, applied)
		assert.Equal(t, "updated board settings\n", stdout.String())
	})

	t.Run("EditEmptyDescriptionClears", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		empty := ""
		invocation := testInvocation(t, &stdout, &stderr)
		catalog.edited = testBoard(
			t, "board-two", "project-one", secondary.Name(), nil,
		)

		err := (&boardEditCommand{
			Description: &empty,
		}).Run(invocation, boards, resolver, &invocation.Markdown)
		require.NoError(t, err)

		applied, applyErr := secondary.EditSettings(catalog.editRequest.Settings)
		require.NoError(t, applyErr)
		assert.Nil(t, applied.Description())
		assert.Equal(t, "updated board description\n", stdout.String())
	})
}

func TestBoardPinCommandsDelegateSelectorsAndRenderResults(t *testing.T) {
	value := issue.Reference{
		ID: "an-one", Title: "Pinned\nissue\tname", Type: "task",
		Status: "ready", Priority: 2,
	}

	t.Run("PinByKey", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		operations := &fakeBoardPinOperations{
			pinResult: board.PinMutation{Issue: value, Changed: true},
		}
		app, err := New(
			testConfig(&stdout, &stderr),
			kong.BindTo(operations, (*BoardPinOperations)(nil)),
		)
		require.NoError(t, err)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--actor", "hooke", "board", "pin", "--key", "source:one",
		}))
		assert.Equal(t, "hooke", operations.actor)
		assert.Equal(t, BoardPinRequest{Value: "source:one", Key: true}, operations.request)
		assert.Empty(t, (&boardPinCommand{Issue: "source:one", Key: true}).referencedIssueIDs())
		assert.Equal(t, "pinned an-one: Pinned issue name\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("PinRejectsEmptyKey", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		operations := &fakeBoardPinOperations{}
		app, err := New(
			testConfig(&stdout, &stderr),
			kong.BindTo(operations, (*BoardPinOperations)(nil)),
		)
		require.NoError(t, err)

		assert.Equal(t, ExitUsage, app.Run(t.Context(), []string{
			"board", "pin", "--key", "",
		}))
		assert.Equal(t, BoardPinRequest{}, operations.request)
		assert.Empty(t, stdout.String())
		assert.Contains(t, stderr.String(), "external key must not be empty")
	})

	t.Run("ListHuman", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		operations := &fakeBoardPinOperations{pins: []issue.Reference{value}}
		app, err := New(
			testConfig(&stdout, &stderr),
			kong.BindTo(operations, (*BoardPinOperations)(nil)),
		)
		require.NoError(t, err)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"board", "pins"}))
		assert.Contains(t, stdout.String(), "an-one  2    ready   task  Pinned issue name\n")
		assert.Empty(t, stderr.String())
	})

	t.Run("ListJSONLines", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		operations := &fakeBoardPinOperations{pins: []issue.Reference{value}}
		app, err := New(
			testConfig(&stdout, &stderr),
			kong.BindTo(operations, (*BoardPinOperations)(nil)),
		)
		require.NoError(t, err)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--json", "board", "pins",
		}))
		assert.Equal(t, "{\"id\":\"an-one\",\"title\":\"Pinned\\nissue\\tname\",\"type\":\"task\",\"status\":\"ready\",\"priority\":2}\n", stdout.String())
		assert.Empty(t, stderr.String())
	})
}

func TestVersionCommandRendersAvailableBuildMetadata(t *testing.T) {
	const (
		revision  = "946440c7b34322afb39e1693431aaf1d41f3f5af"
		buildTime = "2026-08-07T05:06:12Z"
	)
	tests := []struct {
		name          string
		giveVersion   string
		giveBuildTime string
		giveRevision  string
		giveModified  bool
		want          string
	}{
		{
			name:          "Clean",
			giveVersion:   "0.6.0",
			giveBuildTime: buildTime,
			giveRevision:  revision,
			want:          "0.6.0 (" + revision + ", built " + buildTime + ")\n",
		},
		{
			name:          "Dirty",
			giveVersion:   "dev",
			giveBuildTime: "2026-08-08T19:42:31Z",
			giveRevision:  revision,
			giveModified:  true,
			want:          "dev (" + revision + "-dirty, built 2026-08-08T19:42:31Z)\n",
		},
		{
			name:        "WithoutGitMetadata",
			giveVersion: "dev", giveBuildTime: buildTime,
			want: "dev (built " + buildTime + ")\n",
		},
		{
			name:         "WithoutBuildTime",
			giveVersion:  "dev",
			giveRevision: revision,
			want:         "dev (" + revision + ")\n",
		},
		{name: "WithoutBuildMetadata", giveVersion: "dev", want: "dev\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			invocation := testInvocation(t, &stdout, &stderr)
			command := newVersionCommand(
				tt.giveVersion,
				tt.giveBuildTime,
				tt.giveRevision,
				tt.giveModified,
			)

			require.NoError(t, command.Run(invocation))
			assert.Equal(t, tt.want, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestInfoCommandForwardsStoreAndBoardAndRendersJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result := InfoResult{
		Store: InfoStore{
			Directory:    "/repo/.cardamom",
			DatabasePath: "/repo/.cardamom/board.sqlite3",
		},
		Project: InfoProject{ID: "project-one", Name: "Cardamom"},
		Board: InfoBoard{
			ID: "board-one", ProjectID: "project-one", Name: "Mission",
		},
		Schema: InfoSchema{DatabaseVersion: 12, CodeVersion: 13},
		Configuration: InfoConfiguration{
			Issue: InfoIssueConfiguration{
				ID: InfoIssueIDConfiguration{
					Prefix: "mission-", Strategy: "sequential",
				},
				Summary: InfoSummaryConfiguration{MaxBytes: 2048},
			},
			Attachment: InfoAttachmentConfiguration{MaxBytes: 104857600},
			Board:      InfoBoardConfiguration{Pins: InfoPinConfiguration{MaxCount: 8}},
		},
		Revision: InfoRevision{Current: 42},
		Issues: InfoIssueInventory{
			Total: 3,
			ByStatus: []InfoIssueStatusCount{
				{Status: "ready", Count: 2},
				{Status: "closed", Count: 1},
			},
		},
	}
	invocation := testInvocation(t, &stdout, &stderr)
	invocation.Store = "/repo/.cardamom"
	invocation.StoreExplicit = true
	invocation.Board = "Mission"
	invocation.Output = newOutput(&stdout, &stderr, true, false)
	request := InfoRequest{Store: "/repo/.cardamom", Board: "Mission"}
	operation := NewMockInfoOperation(gomock.NewController(t))
	operation.EXPECT().Read(invocation.Context, request).Return(result, nil)

	require.NoError(t, (&infoCommand{}).Run(invocation, operation))

	assert.JSONEq(t, `{
		"store":{"directory":"/repo/.cardamom","database_path":"/repo/.cardamom/board.sqlite3"},
		"project":{"id":"project-one","name":"Cardamom"},
		"board":{"id":"board-one","project_id":"project-one","name":"Mission"},
		"schema":{"database_version":12,"code_version":13},
		"configuration":{"issue":{"id":{"prefix":"mission-","strategy":"sequential"},
		"summary":{"max_bytes":2048}},"attachment":{"max_bytes":104857600},
		"board":{"pins":{"max_count":8}}},
		"revision":{"current":42},"issues":{"total":3,"by_status":[
		{"status":"ready","count":2},{"status":"closed","count":1}]}
	}`, stdout.String())
}

func TestWebCommandDelegatesLongRunningInvocation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	invocation := testInvocation(t, &stdout, &stderr)
	invocation.Store = "/repo/.cardamom"
	invocation.Board = "Mission"
	command := &webCommand{
		Bind: "0.0.0.0", Port: 9000, NoBrowser: true, ReadOnly: true,
	}
	request := WebRequest{
		Store: "/repo/.cardamom", Board: "Mission",
		Bind: "0.0.0.0", Port: 9000, NoBrowser: true, ReadOnly: true,
		Notice: &stdout, Diagnostic: &stderr,
	}
	operation := NewMockWebOperation(gomock.NewController(t))
	operation.EXPECT().Run(invocation.Context, request).Return(nil)

	err := command.Run(invocation, operation)
	require.NoError(t, err)
}

func TestWebCommandRejectsAggregateWithLocalSelectors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := NewMockWebOperation(gomock.NewController(t))
	invocation := testInvocation(t, &stdout, &stderr)
	invocation.Store = "/repo/.cardamom"
	invocation.StoreExplicit = true
	sourceURL, err := url.Parse("http://primary.test")
	require.NoError(t, err)
	command := &webCommand{
		Sources: []WebSource{{Alias: "primary", URL: sourceURL}},
	}

	err = command.Run(invocation, operation)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be combined with --store or --board")
}

func TestWebCommandParsesAndValidatesSources(t *testing.T) {
	t.Run("RepeatedSources", func(t *testing.T) {
		t.Setenv("CARDAMOM_STORE", "")
		var stdout, stderr bytes.Buffer
		primaryURL, err := url.Parse("https://primary.example")
		require.NoError(t, err)
		backupURL, err := url.Parse("http://127.0.0.1:5758/card")
		require.NoError(t, err)
		request := WebRequest{
			Bind: "127.0.0.1", Port: 5757, NoBrowser: true,
			Sources: []WebSource{
				{Alias: "primary", URL: primaryURL},
				{Alias: "backup", URL: backupURL},
			},
			Notice: &stdout, Diagnostic: &stderr,
		}
		operation := NewMockWebOperation(gomock.NewController(t))
		operation.EXPECT().Run(gomock.Any(), request).Return(nil)
		app, err := New(
			testConfig(&stdout, &stderr),
			kong.BindTo(operation, (*WebOperation)(nil)),
		)
		require.NoError(t, err)

		exitCode := app.Run(t.Context(), []string{
			"web", "--no-browser",
			"--source", "primary=https://primary.example",
			"--source", "backup=http://127.0.0.1:5758/card",
		})

		assert.Equal(t, ExitSuccess, exitCode)
	})

	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "MissingEquals", value: "primary"},
		{name: "EmptyAlias", value: "=https://primary.example"},
		{name: "RelativeURL", value: "primary=/card"},
		{name: "UnsupportedScheme", value: "primary=ftp://primary.example"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			operation := NewMockWebOperation(gomock.NewController(t))
			app, err := New(
				testConfig(&stdout, &stderr),
				kong.BindTo(operation, (*WebOperation)(nil)),
			)
			require.NoError(t, err)

			exitCode := app.Run(t.Context(), []string{
				"web", "--no-browser", "--source", test.value,
			})

			assert.Equal(t, ExitUsage, exitCode)
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), "source")
		})
	}
}

func TestWebCommandDefaultsToPort5757(t *testing.T) {
	t.Setenv("CARDAMOM_STORE", "")
	var stdout, stderr bytes.Buffer
	request := WebRequest{
		Bind: "127.0.0.1", Port: 5757, NoBrowser: true,
		Notice: &stdout, Diagnostic: &stderr,
	}
	operation := NewMockWebOperation(gomock.NewController(t))
	operation.EXPECT().Run(gomock.Any(), request).Return(nil)
	app, err := New(
		testConfig(&stdout, &stderr),
		kong.BindTo(operation, (*WebOperation)(nil)),
	)
	require.NoError(t, err)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"web", "--no-browser"}))
}

func TestStatusCommandsHonorQuietWithoutSkippingOperations(t *testing.T) {
	t.Run("Init", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		result := InitResult{
			Dir: ".cardamom", BoardCreated: true,
			IgnoreOutcome: InitIgnoreStoragePatternsAdded,
		}
		invocation := testInvocation(t, &stdout, &stderr)
		invocation.Output = newOutput(&stdout, &stderr, false, true)
		operation := NewMockInitOperation(gomock.NewController(t))
		operation.EXPECT().Initialize(
			invocation.Context,
			gomock.Any(),
		).Return(result, nil)

		require.NoError(t, (&initCommand{}).Run(invocation, operation))
		assert.Empty(t, stdout.String())
	})

	t.Run("BoardUse", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		state := testBoard(t, "board-one", "project-one", "Primary", nil)
		catalog := &fakeProjectCatalog{
			projects: []*project.State{testProject(t, "project-one", "cardamom")},
			boards:   []*board.State{state},
		}
		binding := new(fakeBoardBinding)
		boards := board.NewService(catalog, catalog)
		resolver := selection.NewResolver(boards, binding, fakeIssueBoardLocator{})
		invocation := testInvocation(t, &stdout, &stderr)
		invocation.Output = newOutput(&stdout, &stderr, false, true)

		require.NoError(t, (&boardUseCommand{Selector: "Primary"}).Run(
			invocation,
			resolver,
		))
		assert.Equal(t, state.ID(), binding.written)
		assert.Empty(t, stdout.String())
	})
}

func TestCompletionCommandGeneratesExplicitShellScript(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app, err := New(testConfig(&stdout, &stderr))
	require.NoError(t, err)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"completion", "bash"}))
	assert.Contains(t, stdout.String(), "complete -C ")
	assert.Contains(t, stdout.String(), " card\n")
	assert.Empty(t, stderr.String())
}

func TestApplicationRunServesImplicitShellCompletion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	t.Setenv("COMP_LINE", "card ")
	t.Setenv("COMP_POINT", "5")
	app, err := New(testConfig(&stdout, &stderr))
	require.NoError(t, err)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), nil))
	assert.Contains(t, stdout.String(), "init\n")
	assert.Contains(t, stdout.String(), "board\n")
	assert.Contains(t, stdout.String(), "completion\n")
	assert.Empty(t, stderr.String())
}

func TestApplicationRunCompletesAttachmentPaginationFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	t.Setenv("COMP_LINE", "card attachment list --")
	t.Setenv("COMP_POINT", "23")
	app, err := New(testConfig(&stdout, &stderr))
	require.NoError(t, err)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), nil))
	assert.Contains(t, stdout.String(), "--limit\n")
	assert.Contains(t, stdout.String(), "--after\n")
	assert.NotContains(t, stdout.String(), "--page-size\n")
	assert.NotContains(t, stdout.String(), "--page-token\n")
	assert.Empty(t, stderr.String())
}

func TestApplicationRunUsesConfiguredCompletionPredictors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	t.Setenv("COMP_LINE", "card --board bo")
	t.Setenv("COMP_POINT", "15")
	config := testConfig(&stdout, &stderr)
	config.CompletionOptions = []komplete.Option{
		komplete.WithPredictor("boards", komplete.PredictFunc(
			func(komplete.Args) []string { return []string{"board-one"} },
		)),
	}
	app, err := New(config)
	require.NoError(t, err)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), nil))
	assert.Equal(t, "board-one\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestApplicationRunCompletesLeaseRevocationOwner(t *testing.T) {
	var stdout, stderr bytes.Buffer
	t.Setenv("COMP_LINE", "card lease revoke staging-db --owner wo")
	t.Setenv("COMP_POINT", "39")
	config := testConfig(&stdout, &stderr)
	config.CompletionOptions = []komplete.Option{
		komplete.WithPredictor("actor", komplete.PredictFunc(
			func(komplete.Args) []string { return []string{"worker-a"} },
		)),
	}
	app, err := New(config)
	require.NoError(t, err)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), nil))
	assert.Equal(t, "worker-a\n", stdout.String())
	assert.Empty(t, stderr.String())
}

type fakeProjectCatalog struct {
	projects []*project.State
	boards   []*board.State

	created       *board.State
	createRequest board.CreateRequest
	edited        *board.State
	editRequest   board.EditRequest
}

func (f *fakeProjectCatalog) ListProjects(context.Context) ([]*project.State, error) {
	return f.projects, nil
}

func (f *fakeProjectCatalog) EditProjectName(
	_ context.Context,
	request project.EditNameRequest,
) (*project.State, error) {
	for index, state := range f.projects {
		if state.ID() != request.ProjectID {
			continue
		}
		edited, err := state.EditName(request.Name)
		if err != nil {
			return nil, err
		}
		f.projects[index] = edited
		return edited, nil
	}
	return nil, errkind.Errorf(errkind.NotFound, "project not found")
}

func (f *fakeProjectCatalog) ListAllBoards(context.Context) ([]*board.State, error) {
	return f.boards, nil
}

func (f *fakeProjectCatalog) Board(_ context.Context, id board.ID) (*board.State, error) {
	for _, board := range f.boards {
		if board.ID() == id {
			return board, nil
		}
	}
	return nil, errkind.Errorf(errkind.NotFound, "board not found")
}

func (f *fakeProjectCatalog) SoleBoard(context.Context) (*board.State, error) {
	if len(f.boards) != 1 {
		return nil, errkind.Errorf(errkind.Conflict, "board selection is ambiguous")
	}
	return f.boards[0], nil
}

func (f *fakeProjectCatalog) CreateBoard(
	_ context.Context,
	request board.CreateRequest,
) (*board.State, error) {
	f.createRequest = request
	return f.created, nil
}

func (f *fakeProjectCatalog) EditBoardSettings(
	_ context.Context,
	request board.EditRequest,
) (*board.State, error) {
	f.editRequest = request
	return f.edited, nil
}

func (f *fakeProjectCatalog) ArchiveBoard(
	_ context.Context,
	invocation board.Invocation,
	request board.ArchiveRequest,
) (board.ArchiveResult, error) {
	state, err := f.Board(context.Background(), request.BoardID)
	if err != nil {
		return board.ArchiveResult{}, err
	}
	changed, err := state.ArchiveBoard(invocation.Actor(), time.Unix(40, 0).UTC(), request.Reason)
	return board.ArchiveResult{Board: state, Changed: changed}, err
}

func (f *fakeProjectCatalog) UnarchiveBoard(
	_ context.Context,
	id board.ID,
) (*board.State, bool, error) {
	state, err := f.Board(context.Background(), id)
	if err != nil {
		return nil, false, err
	}
	return state, state.Unarchive(), nil
}

type fakeBoardBinding struct {
	selected board.ID
	written  board.ID
}

type fakeBoardPinOperations struct {
	pins      []issue.Reference
	pinResult board.PinMutation
	request   BoardPinRequest
	actor     string
}

func (f *fakeBoardPinOperations) ListBoardPins(context.Context) ([]issue.Reference, error) {
	return f.pins, nil
}

func (f *fakeBoardPinOperations) PinBoardIssue(
	_ context.Context,
	invocation board.Invocation,
	request BoardPinRequest,
) (board.PinMutation, error) {
	f.actor = invocation.Actor()
	f.request = request
	return f.pinResult, nil
}

func (f *fakeBoardPinOperations) UnpinBoardIssue(
	context.Context,
	board.Invocation,
	BoardPinRequest,
) (board.PinMutation, error) {
	return board.PinMutation{}, nil
}

func (f *fakeBoardBinding) Read() (board.ID, error) {
	if f.selected == "" {
		return "", selection.ErrBindingNotFound
	}
	return f.selected, nil
}

func (f *fakeBoardBinding) Write(id board.ID) error {
	f.written = id
	return nil
}

type fakeIssueBoardLocator map[string]board.ID

func (f fakeIssueBoardLocator) BoardForIssue(
	_ context.Context,
	issueID string,
) (board.ID, error) {
	boardID, ok := f[issueID]
	if !ok {
		return "", selection.ErrIssueNotFound
	}
	return boardID, nil
}

func testInvocation(t *testing.T, stdout, stderr *bytes.Buffer) *Invocation {
	t.Helper()
	ctx := t.Context()
	return &Invocation{
		Context: ctx,
		Actor:   "worker",
		Output:  newOutput(stdout, stderr, false, false),
		Stdin:   strings.NewReader(""),
		Markdown: MarkdownInput{
			Context: ctx, Stdin: strings.NewReader(""), IsTerminal: true,
		},
	}
}

func testProject(t *testing.T, id, name string) *project.State {
	t.Helper()
	projectID, err := project.NewID(id)
	require.NoError(t, err)
	namespace, err := project.Load(project.Snapshot{
		ID: projectID, Name: name, Created: time.Unix(10, 0).UTC(),
	})
	require.NoError(t, err)
	return namespace
}

func testBoard(
	t *testing.T,
	id string,
	projectID string,
	name string,
	description *string,
) *board.State {
	t.Helper()
	boardIdentity, err := board.NewID(id)
	require.NoError(t, err)
	projectIdentity, err := project.NewID(projectID)
	require.NoError(t, err)
	board, err := board.Load(board.Snapshot{
		ID: boardIdentity, ProjectID: projectIdentity.String(), Name: name,
		Description: description, Created: time.Unix(10, 0).UTC(),
	})
	require.NoError(t, err)
	return board
}

var (
	_ BoardCatalog           = (*board.Service)(nil)
	_ board.Catalog          = (*fakeProjectCatalog)(nil)
	_ board.Changes          = (*fakeProjectCatalog)(nil)
	_ project.Projects       = (*fakeProjectCatalog)(nil)
	_ selection.Catalog      = (*board.Service)(nil)
	_ selection.Binding      = (*fakeBoardBinding)(nil)
	_ selection.IssueLocator = fakeIssueBoardLocator{}
)

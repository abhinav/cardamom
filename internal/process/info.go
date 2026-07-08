package process

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/information"
	repositoryboard "go.abhg.dev/cardamom/internal/repository/board"
	repositoryinformation "go.abhg.dev/cardamom/internal/repository/information"
	"go.abhg.dev/cardamom/internal/storelocation"
)

// infoOperation opens and closes a namespace for one information read.
type infoOperation struct{ config Config }

func provideInfo(config *Config) cli.InfoOperation {
	return &infoOperation{config: *config}
}

func (o *infoOperation) Read(
	ctx context.Context,
	request cli.InfoRequest,
) (result cli.InfoResult, err error) {
	runtime, err := openNamespace(ctx, o.config, request.Store)
	if err != nil {
		return cli.InfoResult{}, err
	}
	defer func() { err = errors.Join(err, runtime.close()) }()
	selectedBoard, err := runtime.selectBoard(ctx, request.Board, nil)
	if err != nil {
		return cli.InfoResult{}, err
	}
	report, err := runtime.informationService().Read(ctx, information.Request{
		BoardID: selectedBoard.ID(),
	})
	if err != nil {
		return cli.InfoResult{}, err
	}
	statusCounts := make(
		[]cli.InfoIssueStatusCount,
		len(report.Issues.ByStatus),
	)
	for index, count := range report.Issues.ByStatus {
		statusCounts[index] = cli.InfoIssueStatusCount{
			Status: count.Status.String(), Count: count.Count,
		}
	}
	return cli.InfoResult{
		Store: cli.InfoStore{
			Directory:    report.Store.Directory,
			DatabasePath: report.Store.DatabasePath,
		},
		Project: cli.InfoProject{
			ID: report.Project.ID().String(), Name: report.Project.Name(),
		},
		Board: cli.InfoBoard{
			ID:        report.Board.ID().String(),
			ProjectID: report.Board.ProjectID(),
			Name:      report.Board.Name(),
		},
		Schema: cli.InfoSchema{
			DatabaseVersion: report.Schema.DatabaseVersion,
			CodeVersion:     report.Schema.CodeVersion,
		},
		Configuration: cli.InfoConfiguration{
			Issue: cli.InfoIssueConfiguration{
				ID: cli.InfoIssueIDConfiguration{
					Prefix:   report.Configuration.Issue.ID.Prefix.String(),
					Strategy: report.Configuration.Issue.ID.Strategy.String(),
				},
				Summary: cli.InfoSummaryConfiguration{
					MaxBytes: report.Configuration.Issue.Summary.MaxBytes.Uint64(),
				},
			},
			Attachment: cli.InfoAttachmentConfiguration{
				MaxBytes: report.Configuration.Attachment.MaxBytes.Uint64(),
			},
		},
		Revision: cli.InfoRevision{Current: report.Revision.Current},
		Issues: cli.InfoIssueInventory{
			Total: report.Issues.Total, ByStatus: statusCounts,
		},
	}, nil
}

// informationService composes information for this open namespace.
func (r *namespaceRuntime) informationService() *information.Service {
	return information.NewService(information.ServiceConfig{
		Store: information.Store{
			Directory:    r.directory,
			DatabasePath: storelocation.DatabasePath(r.directory),
		},
		Projects: r.projects, Boards: r.boards,
		Configurations: r.configuration,
		Readers:        &informationReaderFactory{runtime: r},
	})
}

// informationReaderFactory opens snapshot readers against one process-lifetime
// store.
type informationReaderFactory struct {
	// runtime owns the store and board repository configuration.
	runtime *namespaceRuntime
}

// Reader returns an information reader scoped to boardID.
func (f *informationReaderFactory) Reader(
	boardID board.ID,
	effective configuration.Configuration,
) (information.Reader, error) {
	boardRepository, err := repositoryboard.New(
		f.runtime.store,
		repositoryboard.Config{
			BoardID:    boardID,
			IDPrefix:   effective.Issue.ID.Prefix.String(),
			IDStrategy: effective.Issue.ID.Strategy.String(),
			Clock:      f.runtime.clock, Entropy: f.runtime.entropy,
		},
	)
	if err != nil {
		return nil, err
	}
	return repositoryinformation.New(f.runtime.store, boardRepository), nil
}

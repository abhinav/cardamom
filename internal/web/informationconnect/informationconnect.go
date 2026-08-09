// Package informationconnect exposes store information through Connect.
package informationconnect

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/information"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web"
)

//go:generate go tool mockgen -destination mocks_test.go -package informationconnect -typed -write_package_comment=false . Reader

// Reader returns information for an explicitly selected board.
type Reader interface {
	// Read returns typed store identity and inventory.
	Read(context.Context, information.Request) (information.Report, error)
}

// Service adapts store information to generated InformationService RPCs.
type Service struct {
	privatev1connect.UnimplementedInformationServiceHandler
	reader Reader
}

var _ privatev1connect.InformationServiceHandler = (*Service)(nil)

// New constructs an InformationService handler.
func New(reader Reader) *Service {
	must.NotBeNilf(reader, "informationconnect: reader is required")
	return &Service{reader: reader}
}

// GetInformation returns typed information for one selected board.
func (s *Service) GetInformation(
	ctx context.Context,
	request *connect.Request[privatev1.GetInformationRequest],
) (*connect.Response[privatev1.GetInformationResponse], error) {
	boardID, err := board.NewID(request.Msg.GetBoardId())
	if err != nil {
		return nil, web.FromError(err)
	}
	report, err := s.reader.Read(ctx, information.Request{BoardID: boardID})
	if err != nil {
		return nil, web.FromError(err)
	}
	response, err := informationResponse(report)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(response), nil
}

func informationResponse(
	report information.Report,
) (*privatev1.GetInformationResponse, error) {
	counts := make(
		[]*privatev1.IssueStatusCount,
		len(report.Issues.ByStatus),
	)
	for index, count := range report.Issues.ByStatus {
		status, err := issueStatus(count.Status)
		if err != nil {
			return nil, err
		}
		counts[index] = &privatev1.IssueStatusCount{
			Status: status, Count: uint64(count.Count),
		}
	}
	return &privatev1.GetInformationResponse{
		Store: &privatev1.StoreInformation{
			Directory:    report.Store.Directory,
			DatabasePath: report.Store.DatabasePath,
		},
		Project: &privatev1.Project{
			Id: report.Project.ID().String(), Name: report.Project.Name(),
		},
		Board: &privatev1.BoardSummary{
			Id: report.Board.ID().String(), ProjectId: report.Board.ProjectID(),
			Name: report.Board.Name(),
		},
		Schema: &privatev1.SchemaInformation{
			DatabaseVersion: uint64(report.Schema.DatabaseVersion),
			CodeVersion:     uint64(report.Schema.CodeVersion),
		},
		Configuration: configurationMessage(report.Configuration),
		Revision: &privatev1.RevisionInformation{
			Current: uint64(report.Revision.Current),
		},
		Issues: &privatev1.IssueInventory{
			Total: uint64(report.Issues.Total), ByStatus: counts,
		},
	}, nil
}

func configurationMessage(
	value configuration.Configuration,
) *privatev1.Configuration {
	return &privatev1.Configuration{
		Issue: &privatev1.ConfigurationIssue{
			Id: &privatev1.ConfigurationIssueID{
				Prefix:   value.Issue.ID.Prefix.String(),
				Strategy: configurationIssueIDStrategy(value.Issue.ID.Strategy),
			},
			Summary: &privatev1.ConfigurationSummary{
				MaxBytes: value.Issue.Summary.MaxBytes.Uint64(),
			},
		},
		Attachment: &privatev1.ConfigurationAttachment{
			MaxBytes: value.Attachment.MaxBytes.Uint64(),
		},
	}
}

func configurationIssueIDStrategy(
	value configuration.IDStrategy,
) privatev1.ConfigurationIssueIDStrategy {
	switch value {
	case configuration.IDStrategyRandom:
		return privatev1.ConfigurationIssueIDStrategy_CONFIGURATION_ISSUE_ID_STRATEGY_RANDOM
	case configuration.IDStrategySequential:
		return privatev1.ConfigurationIssueIDStrategy_CONFIGURATION_ISSUE_ID_STRATEGY_SEQUENTIAL
	default:
		return privatev1.ConfigurationIssueIDStrategy_CONFIGURATION_ISSUE_ID_STRATEGY_UNSPECIFIED
	}
}

func issueStatus(value issue.Status) (privatev1.IssueStatus, error) {
	switch value {
	case issue.StatusReady:
		return privatev1.IssueStatus_ISSUE_STATUS_READY, nil
	case issue.StatusBlocked:
		return privatev1.IssueStatus_ISSUE_STATUS_BLOCKED, nil
	case issue.StatusInProgress:
		return privatev1.IssueStatus_ISSUE_STATUS_IN_PROGRESS, nil
	case issue.StatusWaiting:
		return privatev1.IssueStatus_ISSUE_STATUS_WAITING, nil
	case issue.StatusClosed:
		return privatev1.IssueStatus_ISSUE_STATUS_CLOSED, nil
	case issue.StatusCancelled:
		return privatev1.IssueStatus_ISSUE_STATUS_CANCELLED, nil
	default:
		return privatev1.IssueStatus_ISSUE_STATUS_UNSPECIFIED,
			fmt.Errorf("unsupported issue status %q", value.String())
	}
}

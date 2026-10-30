package boardconflict

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/configuration"
)

//go:generate go tool mockgen -destination service_mocks_test.go -package boardconflict -typed -write_package_comment=false go.abhg.dev/cardamom/internal/boardconflict Repository,ConfigurationResolver

func TestServiceAdmitsComposedSummary(t *testing.T) {
	for _, test := range []struct {
		name       string
		body       string
		unresolved bool
		reject     bool
	}{
		{name: "within limit", body: "small"},
		{name: "over limit", body: "too much text", reject: true},
		{name: "unresolved limit", body: "small", unresolved: true, reject: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			control := gomock.NewController(t)
			repository := NewMockRepository(control)
			config := NewMockConfigurationResolver(control)
			service := Service{Repository: repository, Configuration: config}
			conflict := Load(Snapshot{ID: "reviewed", BoardID: board.ID("board-test"), Unit: Unit{Kind: boardhistory.EventIssueSummarySet}, Alternatives: nil})
			repository.EXPECT().ReadConflict(gomock.Any(), "reviewed").Return(Inspection{Conflict: conflict}, nil)
			policy := configuration.Configuration{}
			if test.unresolved {
				policy.Conflicts = []configuration.ConflictReference{{ID: "limit-conflict", Field: configuration.FieldIssueSummaryMaxBytes}}
			} else {
				limit, err := configuration.NewByteLimit(5)
				require.NoError(t, err)
				policy.Issue.Summary.MaxBytes = &limit
			}
			config.EXPECT().ResolveConfiguration(gomock.Any(), board.ID("board-test")).Return(policy, nil)
			request := ResolveRequest{ConflictID: "reviewed", Choice: ReplaceText{Body: test.body}}
			if !test.reject {
				repository.EXPECT().ResolveConflict(gomock.Any(), request).Return(Resolution{ConflictID: "reviewed"}, nil)
			}
			_, err := service.Resolve(t.Context(), request)
			if test.reject {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestServiceRetainedSelectionAndClearDoNotConsumeSummaryLimit(t *testing.T) {
	for _, test := range []struct {
		name   string
		choice Choice
	}{
		{name: "retained selection", choice: TakeAlternative{ID: "retained"}},
		{name: "clear summary", choice: ReplaceText{Body: "\n "}},
	} {
		t.Run(test.name, func(t *testing.T) {
			control := gomock.NewController(t)
			repository := NewMockRepository(control)
			service := Service{Repository: repository, Configuration: NewMockConfigurationResolver(control)}
			request := ResolveRequest{ConflictID: "reviewed", Choice: test.choice}
			repository.EXPECT().ResolveConflict(gomock.Any(), request).Return(Resolution{ConflictID: "reviewed"}, nil)
			_, err := service.Resolve(t.Context(), request)
			assert.NoError(t, err)
		})
	}
}

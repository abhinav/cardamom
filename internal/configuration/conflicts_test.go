package configuration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.uber.org/mock/gomock"
)

func TestResolveConflictedOverrideDoesNotInherit(t *testing.T) {
	layers := DatabaseLayers{
		ProjectID:      mustProjectID(t, "project-test"),
		Project:        Overrides{Issue: IssueOverrides{ID: IssueIDOverrides{Prefix: new(Prefix("parent-"))}}},
		BoardConflicts: []ConflictReference{{ID: "conflict-prefix", Field: FieldIssueIDPrefix}},
	}
	store := NewMockStore(gomock.NewController(t))
	store.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(Overrides{}, nil)
	repository := NewMockRepository(gomock.NewController(t))
	repository.EXPECT().ReadConfigurationLayers(gomock.Any(), gomock.Any()).Return(layers, nil)
	view, err := NewService(store, repository).Resolve(t.Context(), mustBoardID(t, "board-test"))
	require.NoError(t, err)
	assert.Nil(t, view.Board.Overrides.Issue.ID.Prefix)
	assert.Equal(t, layers.BoardConflicts, view.Board.Conflicts)
	assert.Nil(t, view.Effective.Issue.ID.Prefix)
	assert.Equal(t, layers.BoardConflicts, view.Effective.Conflicts)
	assert.Equal(t, ScopeBoard, view.Origins.Issue.ID.Prefix.Scope)
	assert.Equal(t, errkind.Conflict, errkind.Of(view.Effective.RequireResolved(FieldIssueIDPrefix)))
	assert.NoError(t, view.Effective.RequireResolved(FieldAttachmentMaxBytes, FieldBoardPinsMaxCount))
	assert.Equal(t, Defaults().Attachment.MaxBytes, view.Effective.Attachment.MaxBytes)
}

func TestResolveSuppressesOnlyTheDisputedConfigurationField(t *testing.T) {
	for _, test := range []struct {
		field Field
		omit  func(*Configuration)
	}{
		{FieldIssueIDPrefix, func(c *Configuration) { c.Issue.ID.Prefix = nil }},
		{FieldIssueIDStrategy, func(c *Configuration) { c.Issue.ID.Strategy = nil }},
		{FieldIssueSummaryMaxBytes, func(c *Configuration) { c.Issue.Summary.MaxBytes = nil }},
		{FieldAttachmentMaxBytes, func(c *Configuration) { c.Attachment.MaxBytes = nil }},
		{FieldBoardPinsMaxCount, func(c *Configuration) { c.Board.Pins.MaxCount = nil }},
	} {
		t.Run(test.field.String(), func(t *testing.T) {
			conflicts := []ConflictReference{{ID: "conflict", Field: test.field}}
			store := NewMockStore(gomock.NewController(t))
			store.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(Overrides{}, nil)
			repository := NewMockRepository(gomock.NewController(t))
			repository.EXPECT().ReadConfigurationLayers(gomock.Any(), gomock.Any()).Return(DatabaseLayers{
				ProjectID: mustProjectID(t, "project-test"), BoardConflicts: conflicts,
			}, nil)
			got, err := NewService(store, repository).ResolveConfiguration(t.Context(), mustBoardID(t, "board-test"))
			require.NoError(t, err)
			want := Defaults()
			test.omit(&want)
			want.Conflicts = conflicts
			assert.Equal(t, want, got)
			assert.Equal(t, errkind.Conflict, errkind.Of(got.RequireResolved(test.field)))
		})
	}
}

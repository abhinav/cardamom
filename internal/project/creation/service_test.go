package creation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/project"
	"go.uber.org/mock/gomock"
)

func TestService_CreateProject_infersPrefix(t *testing.T) {
	created := testProject(t, "project-one", "Mission Control")
	configurations := NewMockConfiguration(gomock.NewController(t))
	configurations.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(configuration.Overrides{}, nil)
	projects := NewMockProjects(gomock.NewController(t))
	inferred := testPrefix(t, "mission-control-")
	projects.EXPECT().CreateProject(gomock.Any(), Creation{
		Name: "Mission Control", Prefix: &inferred,
	}).Return(created, nil)
	service := NewService(configurations, projects)

	result, err := service.CreateProject(
		t.Context(),
		NewInvocation("captain"),
		Request{Name: "Mission Control"},
	)
	require.NoError(t, err)

	assert.Same(t, created, result)
}

func TestService_CreateProject_explicitPrefixOverridesStore(t *testing.T) {
	created := testProject(t, "project-one", "Mission Control")
	storePrefix := testPrefix(t, "store-")
	overrides := configuration.Overrides{Issue: configuration.IssueOverrides{
		ID: configuration.IssueIDOverrides{Prefix: &storePrefix},
	}}
	configurations := NewMockConfiguration(gomock.NewController(t))
	configurations.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(overrides, nil)
	projects := NewMockProjects(gomock.NewController(t))
	explicitPrefix := testPrefix(t, "explicit-")
	projects.EXPECT().CreateProject(gomock.Any(), Creation{
		Name: "Mission Control", Prefix: &explicitPrefix,
	}).Return(created, nil)
	service := NewService(configurations, projects)
	explicit := "explicit-"

	_, err := service.CreateProject(
		t.Context(),
		NewInvocation("captain"),
		Request{Name: "Mission Control", Prefix: &explicit},
	)
	require.NoError(t, err)
}

func TestService_CreateProject_inheritsStorePrefix(t *testing.T) {
	created := testProject(t, "project-one", "Mission Control")
	storePrefix := testPrefix(t, "store-")
	overrides := configuration.Overrides{Issue: configuration.IssueOverrides{
		ID: configuration.IssueIDOverrides{Prefix: &storePrefix},
	}}
	configurations := NewMockConfiguration(gomock.NewController(t))
	configurations.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(overrides, nil)
	projects := NewMockProjects(gomock.NewController(t))
	projects.EXPECT().CreateProject(gomock.Any(), Creation{
		Name: "Mission Control",
	}).Return(created, nil)
	service := NewService(configurations, projects)

	_, err := service.CreateProject(
		t.Context(),
		NewInvocation("captain"),
		Request{Name: "Mission Control"},
	)
	require.NoError(t, err)
}

func TestService_CreateProject_rejectsInvalidExplicitPrefix(t *testing.T) {
	configurations := NewMockConfiguration(gomock.NewController(t))
	configurations.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(configuration.Overrides{}, nil)
	projects := NewMockProjects(gomock.NewController(t))
	service := NewService(configurations, projects)
	invalid := "INVALID"

	_, err := service.CreateProject(
		t.Context(),
		NewInvocation("captain"),
		Request{Name: "Mission Control", Prefix: &invalid},
	)

	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func testProject(t *testing.T, id, name string) *project.State {
	t.Helper()
	state, err := project.Load(project.Snapshot{
		ID:      project.ID(id),
		Name:    name,
		Created: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return state
}

func testPrefix(t *testing.T, value string) configuration.Prefix {
	t.Helper()
	prefix, err := configuration.NewPrefix(value)
	require.NoError(t, err)
	return prefix
}

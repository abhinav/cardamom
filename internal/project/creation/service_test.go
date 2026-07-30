package creation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/project"
)

func TestService_CreateProject_infersPrefix(t *testing.T) {
	created := testProject(t, "project-one", "Mission Control")
	projects := &recordingProjects{created: created}
	service := NewService(
		&staticConfiguration{},
		projects,
	)

	result, err := service.CreateProject(
		t.Context(),
		NewInvocation("captain"),
		Request{Name: "Mission Control"},
	)
	require.NoError(t, err)

	assert.Same(t, created, result)
	require.NotNil(t, projects.request.Prefix)
	assert.Equal(t, "mission-control-", projects.request.Prefix.String())
}

func TestService_CreateProject_explicitPrefixOverridesStore(t *testing.T) {
	created := testProject(t, "project-one", "Mission Control")
	projects := &recordingProjects{created: created}
	storePrefix := testPrefix(t, "store-")
	service := NewService(
		&staticConfiguration{overrides: configuration.Overrides{
			Issue: configuration.IssueOverrides{
				ID: configuration.IssueIDOverrides{Prefix: &storePrefix},
			},
		}},
		projects,
	)
	explicit := "explicit-"

	_, err := service.CreateProject(
		t.Context(),
		NewInvocation("captain"),
		Request{Name: "Mission Control", Prefix: &explicit},
	)
	require.NoError(t, err)

	require.NotNil(t, projects.request.Prefix)
	assert.Equal(t, "explicit-", projects.request.Prefix.String())
}

func TestService_CreateProject_inheritsStorePrefix(t *testing.T) {
	created := testProject(t, "project-one", "Mission Control")
	projects := &recordingProjects{created: created}
	storePrefix := testPrefix(t, "store-")
	service := NewService(
		&staticConfiguration{overrides: configuration.Overrides{
			Issue: configuration.IssueOverrides{
				ID: configuration.IssueIDOverrides{Prefix: &storePrefix},
			},
		}},
		projects,
	)

	_, err := service.CreateProject(
		t.Context(),
		NewInvocation("captain"),
		Request{Name: "Mission Control"},
	)
	require.NoError(t, err)

	assert.Nil(t, projects.request.Prefix)
}

func TestService_CreateProject_rejectsInvalidExplicitPrefix(t *testing.T) {
	projects := &recordingProjects{}
	service := NewService(&staticConfiguration{}, projects)
	invalid := "INVALID"

	_, err := service.CreateProject(
		t.Context(),
		NewInvocation("captain"),
		Request{Name: "Mission Control", Prefix: &invalid},
	)

	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.False(t, projects.called)
}

type staticConfiguration struct {
	overrides configuration.Overrides
}

func (c *staticConfiguration) ReadStoreConfiguration(
	context.Context,
) (configuration.Overrides, error) {
	return c.overrides, nil
}

type recordingProjects struct {
	request Creation
	created *project.State
	called  bool
}

func (p *recordingProjects) CreateProject(
	_ context.Context,
	request Creation,
) (*project.State, error) {
	p.called = true
	p.request = request
	return p.created, nil
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

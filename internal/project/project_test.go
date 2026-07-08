package project

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
)

func TestLoadProjectRejectsBlankName(t *testing.T) {
	_, err := Load(Snapshot{
		ID:      testProjectID(t, "project-one"),
		Name:    " ",
		Created: time.Unix(10, 0).UTC(),
	})

	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestSelectorMatchesIdentityOrName(t *testing.T) {
	state := testProjectState(t, Snapshot{
		ID:      testProjectID(t, "project-one"),
		Name:    "cardamom",
		Created: time.Unix(10, 0).UTC(),
	})
	id := testProjectSelector(t, " project-one ")
	name := testProjectSelector(t, "cardamom")

	assert.True(t, id.Matches(state))
	assert.True(t, name.Matches(state))
	assert.False(t, testProjectSelector(t, "other").Matches(state))
}

func TestSelectorRejectsBlankValue(t *testing.T) {
	_, err := NewSelector(" ")
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestServiceListsAndSelectsExplicitOrSoleProject(t *testing.T) {
	primary := testProjectState(t, Snapshot{
		ID:      testProjectID(t, "project-one"),
		Name:    "Primary",
		Created: time.Unix(10, 0).UTC(),
	})
	secondary := testProjectState(t, Snapshot{
		ID:      testProjectID(t, "project-two"),
		Name:    "Secondary",
		Created: time.Unix(10, 0).UTC(),
	})
	projects := &fakeProjects{states: []*State{primary, secondary}}
	service := NewService(projects)
	selector := testProjectSelector(t, "Secondary")

	listed, err := service.List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []*State{primary, secondary}, listed)

	selected, err := service.Resolve(t.Context(), &selector)
	require.NoError(t, err)
	assert.Same(t, secondary, selected)

	projects.states = []*State{primary}
	selected, err = service.Resolve(t.Context(), nil)
	require.NoError(t, err)
	assert.Same(t, primary, selected)
}

func TestServiceRejectsAmbiguousProject(t *testing.T) {
	created := time.Unix(10, 0).UTC()
	service := NewService(&fakeProjects{states: []*State{
		testProjectState(t, Snapshot{ID: testProjectID(t, "one"), Name: "Shared", Created: created}),
		testProjectState(t, Snapshot{ID: testProjectID(t, "two"), Name: "Shared", Created: created}),
	}})
	selector := testProjectSelector(t, "Shared")

	_, err := service.Resolve(t.Context(), &selector)

	assert.Equal(t, errkind.Conflict, errkind.Of(err))
}

func TestIDStrategyPreservesTextualJSON(t *testing.T) {
	strategy, err := NewIDStrategy("sequential")
	require.NoError(t, err)

	body, err := json.Marshal(strategy)
	require.NoError(t, err)
	assert.JSONEq(t, `"sequential"`, string(body))
}

func testProjectID(t *testing.T, value string) ID {
	t.Helper()
	id, err := NewID(value)
	require.NoError(t, err)
	return id
}

func testProjectSelector(t *testing.T, value string) Selector {
	t.Helper()
	selector, err := NewSelector(value)
	require.NoError(t, err)
	return selector
}

func testProjectState(t *testing.T, snapshot Snapshot) *State {
	t.Helper()
	state, err := Load(snapshot)
	require.NoError(t, err)
	return state
}

type fakeProjects struct {
	states []*State
}

func (f *fakeProjects) ListProjects(context.Context) ([]*State, error) {
	return f.states, nil
}

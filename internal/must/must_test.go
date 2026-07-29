package must

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNotErrorf(t *testing.T) {
	errInvariant := errors.New("invariant failed")
	assert.PanicsWithError(t, "operation must succeed: invariant failed", func() {
		NotErrorf(errInvariant, "operation must succeed")
	})
	assert.NotPanics(t, func() { NotErrorf(nil, "operation must succeed") })
}

func TestNotBeNilfRejectsMissingDependency(t *testing.T) {
	assert.PanicsWithError(t, "dependency is required", func() {
		NotBeNilf(nil, "dependency is required")
	})
	assert.NotPanics(t, func() {
		NotBeNilf(42, "dependency is required")
	})
}

package errkind

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrap(t *testing.T) {
	t.Parallel()

	cause := errors.New("record missing")

	assert.Nil(t, Wrap(NotFound, nil))
	assert.Same(t, cause, Wrap(Unknown, cause))

	err := Wrap(NotFound, cause)
	assert.EqualError(t, err, "record missing")
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, NotFound, Of(err))
}

func TestErrorf(t *testing.T) {
	t.Parallel()

	cause := errors.New("revision changed")
	err := Errorf(Conflict, "update issue: %w", cause)

	assert.EqualError(t, err, "update issue: revision changed")
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, Conflict, Of(err))
}

func TestOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give error
		want Kind
	}{
		{name: "Nil", give: nil, want: Unknown},
		{name: "Unclassified", give: errors.New("storage failure"), want: Unknown},
		{name: "OrdinaryWrap", give: fmt.Errorf("read: %w", Wrap(NotFound, errors.New("missing"))), want: NotFound},
		{name: "Joined", give: errors.Join(errors.New("cleanup"), Wrap(Conflict, errors.New("stale"))), want: Conflict},
		{
			name: "NestedJoin",
			give: fmt.Errorf("operation: %w", errors.Join(
				errors.New("cleanup"),
				fmt.Errorf("lookup: %w", Wrap(InvalidInput, errors.New("bad selector"))),
			)),
			want: InvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Of(tt.give))
		})
	}
}

func TestOperationsRejectInvalidKind(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "errkind: invalid kind -1", func() {
		_ = Wrap(Kind(-1), errors.New("failure"))
	})
	assert.PanicsWithValue(t, "errkind: invalid kind 100", func() {
		_ = Wrap(Kind(100), nil)
	})
	assert.PanicsWithValue(t, "errkind: invalid kind 100", func() {
		_ = Errorf(Kind(100), "failure")
	})
}

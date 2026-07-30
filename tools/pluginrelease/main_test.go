package main

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_Check(t *testing.T) {
	root := newMetadataFixture(t, "0.1.0-beta.2")
	var stdout bytes.Buffer

	require.NoError(t, run(root, &stdout, []string{
		"check",
		"v0.1.0-beta.2",
	}))

	assert.Equal(
		t,
		"plugin version 0.1.0-beta.2 is consistent\n",
		stdout.String(),
	)
}

func TestRun_Materialize(t *testing.T) {
	root := newMetadataFixture(t, "0.1.0-beta.2")
	var stdout bytes.Buffer

	require.NoError(t, run(root, &stdout, []string{
		"materialize",
		"1.2.3",
	}))

	assert.Equal(t, "materialized plugin version 1.2.3\n", stdout.String())
}

func TestRun_RejectsInvalidCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "MissingOperation",
			want: commandUsage,
		},
		{
			name: "TooManyCheckArguments",
			args: []string{"check", "1.2.3", "extra"},
			want: checkUsage,
		},
		{
			name: "MissingMaterializeVersion",
			args: []string{"materialize"},
			want: materializeUsage,
		},
		{
			name: "UnknownOperation",
			args: []string{"publish"},
			want: `unknown operation "publish"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(t.TempDir(), io.Discard, tt.args)

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestRun_ReportsOutputFailure(t *testing.T) {
	root := newMetadataFixture(t, "0.1.0-beta.2")

	err := run(root, errorWriter{}, []string{"check"})

	require.Error(t, err)
	assert.ErrorContains(t, err, "write check result")
	assert.ErrorIs(t, err, errWriteOutput)
}

var errWriteOutput = errors.New("write output")

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errWriteOutput
}

package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
)

func TestApplication_Run_usageFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app, err := New(testConfig(&stdout, &stderr))
	require.NoError(t, err)

	assert.Equal(t, ExitUsage, app.Run(t.Context(), []string{"removed-command"}))
	assert.Empty(t, stdout.String())
	assert.Regexp(t, `^error:`, stderr.String())
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		give error
		want int
	}{
		{name: "Success", give: nil, want: ExitSuccess},
		{name: "Operation", give: errors.New("operation failed"), want: ExitOperation},
		{name: "InvalidOperation", give: errkind.Errorf(errkind.InvalidInput, "invalid input"), want: ExitOperation},
		{name: "Usage", give: UsageErrorf("invalid value"), want: ExitUsage},
		{name: "Cancellation", give: context.Canceled, want: ExitCanceled},
		{name: "WrappedCancellation", give: errors.Join(errors.New("stop"), context.Canceled), want: ExitCanceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExitCode(tt.give))
		})
	}
	assert.Equal(t, errkind.InvalidInput, errkind.Of(UsageErrorf("invalid value")))
}

func testConfig(stdout, stderr *bytes.Buffer) Config {
	return Config{
		Version:         "v1.2.3",
		DefaultActor:    "tester",
		Stdin:           strings.NewReader(""),
		StdinIsTerminal: true,
		Stdout:          stdout,
		Stderr:          stderr,
	}
}

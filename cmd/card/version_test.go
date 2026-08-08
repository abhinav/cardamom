package main

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionLinkerOverride(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "card")
	build := exec.CommandContext(
		t.Context(),
		"go",
		"build",
		"-o", binary,
		"-ldflags",
		"-X go.abhg.dev/cardamom/internal/cli.Version=v1.2.3 "+
			"-X go.abhg.dev/cardamom/internal/cli.BuildTime=2026-08-07T05:06:12Z",
		".",
	)
	buildOutput, err := build.CombinedOutput()
	require.NoErrorf(t, err, "build card:\n%s", buildOutput)

	output, err := exec.CommandContext(t.Context(), binary, "version").CombinedOutput()
	require.NoErrorf(t, err, "run card version:\n%s", output)
	assert.Contains(t, string(output), "v1.2.3 (")
	assert.Contains(t, string(output), "built 2026-08-07T05:06:12Z)")
}

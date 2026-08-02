package hooks_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	hookCommand = `command -v card >/dev/null 2>&1 && ` +
		`card hook context 2>/dev/null || true`
	hookCommandWindows = `cmd.exe /d /s /c "where card >nul 2>nul && ` +
		`card hook context 2>nul || exit /b 0"`
)

func TestClaudeLifecycleHookWiring(t *testing.T) {
	configuration := readClaudeHookConfiguration(t)

	for _, event := range []string{"SessionStart", "SubagentStart"} {
		t.Run(event, func(t *testing.T) {
			groups := configuration.Hooks[event]
			require.Len(t, groups, 1)
			assert.Nil(t, groups[0].Matcher)
			require.Len(t, groups[0].Hooks, 1)
			assert.Equal(t, claudeHookHandler{
				Type:    "command",
				Command: hookCommand,
				Timeout: 5,
			}, groups[0].Hooks[0])
		})
	}

	assert.Len(t, configuration.Hooks, 2)
	assertManifestHookPath(
		t,
		filepath.Join("..", ".claude-plugin", "plugin.json"),
		"./hooks/claude.json",
	)
}

func TestCodexLifecycleHookWiring(t *testing.T) {
	configuration := readCodexHookConfiguration(t)

	for _, event := range []string{"SessionStart", "SubagentStart"} {
		t.Run(event, func(t *testing.T) {
			groups := configuration.Hooks[event]
			require.Len(t, groups, 1)
			assert.Nil(t, groups[0].Matcher)
			require.Len(t, groups[0].Hooks, 1)
			assert.Equal(t, codexHookHandler{
				Type:           "command",
				Command:        hookCommand,
				CommandWindows: hookCommandWindows,
				Timeout:        5,
			}, groups[0].Hooks[0])
		})
	}

	assert.Len(t, configuration.Hooks, 2)
	assertManifestHookPath(
		t,
		filepath.Join("..", ".codex-plugin", "plugin.json"),
		"./hooks/codex.json",
	)
}

func TestLifecycleHookDefaultFileIsAbsent(t *testing.T) {
	_, err := os.Stat("hooks.json")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestClaudeLifecycleHookIsSilentWithoutCard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Claude Code runs shell hooks through Git Bash on Windows")
	}

	handler := readClaudeHookConfiguration(t).Hooks["SessionStart"][0].Hooks[0]
	shell, err := exec.LookPath("sh")
	require.NoError(t, err)
	t.Setenv("PATH", t.TempDir())

	output, err := exec.Command(shell, "-c", handler.Command).CombinedOutput()
	assert.NoError(t, err)
	assert.Empty(t, output)
}

func TestCodexLifecycleHookIsSilentWithoutCard(t *testing.T) {
	handler := readCodexHookConfiguration(t).Hooks["SessionStart"][0].Hooks[0]
	commands := codexHookCommandsForHost(t, handler)
	t.Setenv("PATH", t.TempDir())

	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			output, err := command.CombinedOutput()

			assert.NoError(t, err)
			assert.Empty(t, output)
		})
	}
}

func codexHookCommandsForHost(
	t *testing.T,
	handler codexHookHandler,
) map[string]*exec.Cmd {
	t.Helper()
	if runtime.GOOS == "windows" {
		cmd, err := exec.LookPath("cmd.exe")
		require.NoError(t, err)
		powershell, err := exec.LookPath("powershell.exe")
		require.NoError(t, err)
		return map[string]*exec.Cmd{
			"cmd": exec.Command(cmd, "/d", "/s", "/c", handler.CommandWindows),
			"powershell": exec.Command(
				powershell, "-NoProfile", "-NonInteractive", "-Command",
				handler.CommandWindows,
			),
		}
	}

	shell, err := exec.LookPath("sh")
	require.NoError(t, err)
	return map[string]*exec.Cmd{
		"sh": exec.Command(shell, "-c", handler.Command),
	}
}

type claudeHookConfiguration struct {
	Description string                       `json:"description"`
	Hooks       map[string][]claudeHookGroup `json:"hooks"`
}

type claudeHookGroup struct {
	Matcher *string             `json:"matcher"`
	Hooks   []claudeHookHandler `json:"hooks"`
}

type claudeHookHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

func readClaudeHookConfiguration(t *testing.T) claudeHookConfiguration {
	t.Helper()
	var configuration claudeHookConfiguration
	decodeHookConfiguration(t, "claude.json", &configuration)
	return configuration
}

type codexHookConfiguration struct {
	Description string                      `json:"description"`
	Hooks       map[string][]codexHookGroup `json:"hooks"`
}

type codexHookGroup struct {
	Matcher *string            `json:"matcher"`
	Hooks   []codexHookHandler `json:"hooks"`
}

type codexHookHandler struct {
	Type           string `json:"type"`
	Command        string `json:"command"`
	CommandWindows string `json:"commandWindows"`
	Timeout        int    `json:"timeout"`
}

func readCodexHookConfiguration(t *testing.T) codexHookConfiguration {
	t.Helper()
	var configuration codexHookConfiguration
	decodeHookConfiguration(t, "codex.json", &configuration)
	return configuration
}

func decodeHookConfiguration(t *testing.T, name string, target any) {
	t.Helper()
	body, err := os.Open(name)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, body.Close())
	})

	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(target))
}

func assertManifestHookPath(t *testing.T, path, expected string) {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var manifest struct {
		Hooks string `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(body, &manifest))
	assert.Equal(t, expected, manifest.Hooks)
}

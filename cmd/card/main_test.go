package main

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/process"
)

func TestVersionLinkerOverride(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "card")
	build := exec.CommandContext(
		t.Context(),
		"go",
		"build",
		"-o", binary,
		"-ldflags",
		"-X go.abhg.dev/cardamom/internal/cli.Version=v1.2.3",
		".",
	)
	buildOutput, err := build.CombinedOutput()
	require.NoErrorf(t, err, "build card:\n%s", buildOutput)

	output, err := exec.CommandContext(t.Context(), binary, "version").CombinedOutput()
	require.NoErrorf(t, err, "run card version:\n%s", output)
	assert.Equal(t, "v1.2.3\n", string(output))
}

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"card": testMain,
	})
}

func testMain() {
	cfg, err := process.SystemConfig(os.Args[1:])
	if err != nil {
		panic(err)
	}
	boardID := "board_00000000000000000000"
	for index := 1; index+1 < len(os.Args); index++ {
		if os.Args[index] == "board" && os.Args[index+1] == "create" {
			boardID = "board_11111111111111111111"
			break
		}
	}
	cfg.ProjectIDs = staticProjectIDs{boardID: boardID}
	cfg.Entropy = &processEntropy{
		path: filepath.Join(os.Getenv("WORK"), ".cardamom-test-entropy"),
	}
	cfg.DefaultActor = "tester"
	os.Exit(process.Execute(context.Background(), cfg))
}

// processEntropy keeps generated repository identities stable and distinct
// across the commands in one disposable testscript workspace.
type processEntropy struct {
	mu   sync.Mutex
	path string
}

func (r *processEntropy) Read(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var counter uint64
	encoded, err := os.ReadFile(r.path)
	if err == nil {
		counter, err = strconv.ParseUint(string(encoded), 10, 64)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	counter++
	if err := os.WriteFile(r.path, []byte(strconv.FormatUint(counter, 10)), 0o600); err != nil {
		return 0, err
	}
	var seed [16]byte
	binary.BigEndian.PutUint64(seed[8:], counter)
	for offset := 0; offset < len(data); offset += len(seed) {
		copy(data[offset:], seed[:])
	}
	return len(data), nil
}

// staticProjectIDs keeps namespace command output stable across test processes.
// The test entry point selects the board identity for first-board initialization
// or the retained explicit board-creation workflow. Transaction identities use
// the child process ID because separate commands share one store but must not
// reuse an internal logical transaction identity.
type staticProjectIDs struct {
	// boardID is the deterministic identity for a board created by this command.
	boardID string
}

func (s staticProjectIDs) NewID(kind string) (string, error) {
	if kind == "tx" {
		return kind + "_" + strconv.Itoa(os.Getpid()), nil
	}
	if kind == "board" {
		return s.boardID, nil
	}
	return kind + "_00000000000000000000", nil
}

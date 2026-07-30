//go:build script

package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"go.abhg.dev/cardamom/internal/process"
	"go.abhg.dev/cardamom/internal/storelocation"
)

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
	projectID := "project_00000000000000000000"
	boardID := "board_00000000000000000000"
	if !staticHelpRequested(cfg.Args) {
		for index := 1; index+1 < len(os.Args); index++ {
			switch {
			case os.Args[index] == "project" && os.Args[index+1] == "create":
				projectID = nextStaticNamespaceID(cfg, "project")
			case os.Args[index] == "board" && os.Args[index+1] == "create":
				boardID = nextStaticNamespaceID(cfg, "board")
			}
		}
	}
	cfg.ProjectIDs = staticProjectIDs{
		projectID: projectID,
		boardID:   boardID,
	}
	cfg.Entropy = &processEntropy{
		path: filepath.Join(os.Getenv("WORK"), ".cardamom-test-entropy"),
	}
	cfg.DefaultActor = "tester"
	os.Exit(process.Execute(context.Background(), cfg))
}

func staticHelpRequested(args []string) bool {
	return slices.Contains(args, "--help") || slices.Contains(args, "-h")
}

func nextStaticNamespaceID(cfg process.Config, kind string) string {
	store, err := storelocation.Resolve(staticStoreSelector(cfg.Args), cfg.CWD)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256([]byte(store))
	key := hex.EncodeToString(sum[:8])
	path := filepath.Join(os.Getenv("WORK"), ".cardamom-test-"+kind+"-"+key)
	var counter uint64
	encoded, err := os.ReadFile(path)
	if err == nil {
		counter, err = strconv.ParseUint(string(encoded), 10, 64)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		panic(err)
	}
	counter++
	if err := os.WriteFile(path, []byte(strconv.FormatUint(counter, 10)), 0o600); err != nil {
		panic(err)
	}
	digit := strconv.FormatUint(counter, 10)
	return kind + "_" + strings.Repeat(digit, 20/len(digit))
}

func staticStoreSelector(args []string) string {
	selector := os.Getenv("CARDAMOM_STORE")
	for index := 0; index < len(args); index++ {
		switch {
		case args[index] == "--store" && index+1 < len(args):
			selector = args[index+1]
			index++
		case strings.HasPrefix(args[index], "--store="):
			selector = strings.TrimPrefix(args[index], "--store=")
		}
	}
	return selector
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
// Initialization uses zero identities,
// while explicit create commands receive per-store identities.
// Transaction identities use the child process ID because separate commands
// share one store but must not reuse an internal logical transaction identity.
type staticProjectIDs struct {
	// projectID is the deterministic identity for a project created by this command.
	projectID string

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
	if kind == "project" {
		return s.projectID, nil
	}
	return kind + "_00000000000000000000", nil
}

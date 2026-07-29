package repository_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/dump"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/board"
	repositoryproject "go.abhg.dev/cardamom/internal/repository/project"
	"go.abhg.dev/cardamom/internal/repository/store"
	"go.abhg.dev/cardamom/internal/storelocation"
)

func TestRepositoryAggregatesKeepConcurrentDumpSnapshotsCoherent(t *testing.T) {
	clock := newTestClock(time.Unix(1_700_000_000, 0).UTC())
	boardRepository := openAggregateBoardRepository(t, clock)
	planner := planning.NewPlanner(boardRepository, boardRepository, nil)

	readerReady := make(chan struct{})
	writerDone := make(chan struct{})
	errorsFound := make(chan error, 2)
	go func() {
		close(readerReady)
		for {
			dumpSnapshot, err := boardRepository.ReadDumpSnapshot(t.Context())
			if err == nil {
				err = validateDumpGraphs(dumpSnapshot)
			}
			if err != nil {
				errorsFound <- err
				return
			}
			select {
			case <-writerDone:
				errorsFound <- nil
				return
			default:
			}
		}
	}()
	<-readerReady
	go func() {
		defer close(writerDone)
		for index := range 24 {
			parent := fmt.Sprintf("parent-%02d", index)
			parentTitle := fmt.Sprintf("Parent %02d", index)
			childTitle := fmt.Sprintf("Child %02d", index)
			workstreamType := "workstream"
			taskType := "task"
			dependencies := []planning.ApplyIssueReference{{
				Kind: planning.ApplyReferenceAlias, Alias: parent,
			}}
			_, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("writer"), planning.ApplyDocumentRequest{
				Version: 1, Mode: planning.ApplyModeCommit,
				Issues: []planning.ApplyIssue{
					{Alias: &parent, Title: &parentTitle, Type: &workstreamType},
					{
						Title: &childTitle, Type: &taskType, DependsOn: &dependencies,
						Parent: planning.ApplyParentChange{
							Kind: planning.ParentReplace,
							Reference: planning.ApplyIssueReference{
								Kind: planning.ApplyReferenceAlias, Alias: parent,
							},
						},
					},
				},
			})
			if err != nil {
				errorsFound <- err
				return
			}
		}
		errorsFound <- nil
	}()
	assert.NoError(t, <-errorsFound)
	assert.NoError(t, <-errorsFound)
}

func openAggregateBoardRepository(
	t *testing.T,
	clock *testClock,
) *board.Repository {
	t.Helper()
	dir := t.TempDir()
	boardName := "Primary"
	initialized, err := repositoryproject.NewInitializer(repositoryproject.Config{
		Clock: clock,
		IDSource: &idQueue{values: []string{
			"project-alpha", "board-primary", "tx-initialize",
		}},
	}).InitializeStore(t.Context(), project.StoreInitializationRequest{
		Dir: dir, ProjectName: "Alpha", BoardName: &boardName,
	})
	require.NoError(t, err)
	require.NotNil(t, initialized.Namespace)
	require.NotNil(t, initialized.Namespace.Board)
	persistence, err := store.Open(t.Context(), store.Config{
		Path: storelocation.DatabasePath(dir),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	boardRepository, err := board.New(persistence, board.Config{
		BoardID: initialized.Namespace.Board.ID(), IDPrefix: "it-", IDStrategy: "sequential",
		Clock: clock, Entropy: deterministicEntropy(4096),
	})
	require.NoError(t, err)
	return boardRepository
}

func validateDumpGraphs(snapshot dump.BoardSnapshot) error {
	issues := make(map[string]string, len(snapshot.Issues))
	for _, value := range snapshot.Issues {
		issues[value.Title] = value.ID
	}
	dependencies := make(map[string]string, len(snapshot.Dependencies))
	for _, value := range snapshot.Dependencies {
		dependencies[value.ChildID] = value.ParentID
	}
	containment := make(map[string]string, len(snapshot.Containment))
	for _, value := range snapshot.Containment {
		containment[value.ChildID] = value.ParentID
	}
	return validateGraphEdges(issues, dependencies, containment)
}

func validateGraphEdges(
	issues map[string]string,
	dependencies map[string]string,
	containment map[string]string,
) error {
	for title, childID := range issues {
		index, child := strings.CutPrefix(title, "Child ")
		if !child {
			continue
		}
		parentID, exists := issues["Parent "+index]
		if !exists {
			return fmt.Errorf("%s has no parent issue", title)
		}
		if dependencies[childID] != parentID {
			return fmt.Errorf("%s has no committed dependency", title)
		}
		if containment[childID] != parentID {
			return fmt.Errorf("%s has no committed containment", title)
		}
	}
	return nil
}

// testClock supplies deterministic time to concurrent repository operations.
type testClock struct {
	mu  sync.RWMutex
	now time.Time
}

func newTestClock(now time.Time) *testClock {
	return &testClock{now: now}
}

func (c *testClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

// idQueue returns deterministic identities in repository call order.
type idQueue struct {
	values []string
}

func (q *idQueue) NewID(kind string) (string, error) {
	if len(q.values) == 0 {
		return "", fmt.Errorf("no %s identity available", kind)
	}
	value := q.values[0]
	q.values = q.values[1:]
	return value, nil
}

func deterministicEntropy(blocks int) io.Reader {
	var body bytes.Buffer
	for value := 1; value <= blocks; value++ {
		var block [16]byte
		binary.BigEndian.PutUint64(block[8:], uint64(value))
		body.Write(block[:])
	}
	return bytes.NewReader(body.Bytes())
}

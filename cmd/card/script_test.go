//go:build !webdev

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/stretchr/testify/assert"
)

// TestScript runs the process-boundary CLI scenarios in isolated workspaces.
func TestScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                "../../testdata/script",
		UpdateScripts:      updateFlag(),
		RequireUniqueNames: true,
		Setup:              setupTestScript,
		Condition: func(condition string) (bool, error) {
			if condition == "webassets" {
				return webAssetsBuild, nil
			}
			return false, fmt.Errorf("unknown condition %q", condition)
		},
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"at":      cmdAt,
			"cmpjson": cmdCmpJSON,
		},
	})
}

func TestCompareJSON(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected string
		negated  bool
	}{
		{name: "MalformedActual", actual: `{"name":`, expected: `{"name":"alpha"}`},
		{name: "MalformedExpected", actual: `{"name":"alpha"}`, expected: `{"name":`},
		{name: "MalformedActualNegated", actual: `{"name":`, expected: `{"name":"alpha"}`, negated: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			satisfied, malformed, diagnostic := compareJSON(test.actual, test.expected, test.negated)

			assert.False(t, satisfied)
			assert.True(t, malformed)
			assert.NotEmpty(t, diagnostic)
		})
	}
}

// setupTestScript isolates actor and Git identity from the host machine.
// CARDAMOM_ACTOR gives unqualified commands a stable actor.
// Explicit --actor values still override that default.
func setupTestScript(env *testscript.Env) error {
	home := filepath.Join(env.WorkDir, "home")
	if err := os.Mkdir(home, 0o755); err != nil {
		return fmt.Errorf("create test home: %w", err)
	}

	env.Setenv("HOME", home)
	env.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	env.Setenv("CARDAMOM_ACTOR", "test-user")
	env.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	env.Setenv("GIT_AUTHOR_NAME", "Test User")
	env.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	env.Setenv("GIT_COMMITTER_NAME", "Test User")
	env.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	return nil
}

// cmdAt sets CARDAMOM_NOW to the supplied RFC 3339 timestamp so scenarios can
// control the application clock. Negation is not meaningful for this command.
func cmdAt(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: at <RFC3339>")
	}
	now, err := time.Parse(time.RFC3339, args[0])
	if err != nil {
		ts.Fatalf("invalid time: %v", err)
	}
	ts.Setenv("CARDAMOM_NOW", now.Format(time.RFC3339))
}

// cmdCmpJSON compares two script files using Testify JSONEq semantics.
// Negation succeeds only when both files contain valid, unequal JSON values.
func cmdCmpJSON(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) != 2 {
		ts.Fatalf("usage: cmpjson file1 file2")
	}

	actual := ts.ReadFile(args[0])
	expected := ts.ReadFile(args[1])
	satisfied, malformed, diagnostic := compareJSON(actual, expected, neg)
	if malformed {
		ts.Fatalf("cannot compare %s and %s:\n%s", args[0], args[1], diagnostic)
	}
	if satisfied {
		return
	}
	if neg {
		ts.Fatalf("%s and %s do not differ", args[0], args[1])
	}
	ts.Fatalf("%s and %s differ:\n%s", args[0], args[1], diagnostic)
}

// compareJSON applies Testify JSONEq semantics and reports whether the result
// satisfies the command's possibly negated expectation. Malformed JSON never
// satisfies the expectation.
func compareJSON(actual, expected string, negated bool) (satisfied, malformed bool, diagnostic string) {
	failure := jsonAssertionFailure{}
	equal := assert.JSONEq(&failure, expected, actual)
	malformed = !json.Valid([]byte(actual)) || !json.Valid([]byte(expected))
	return !malformed && equal == !negated, malformed, failure.message
}

// jsonAssertionFailure captures a Testify diagnostic for testscript to report
// with the script path and line of the failed cmpjson command.
type jsonAssertionFailure struct {
	// message is Testify's formatted JSONEq failure output.
	message string
}

// Errorf records the assertion diagnostic emitted by Testify JSONEq.
func (jaf *jsonAssertionFailure) Errorf(format string, args ...any) {
	jaf.message = fmt.Sprintf(format, args...)
}

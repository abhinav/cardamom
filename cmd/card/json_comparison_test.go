// Package main tests the card command at its process boundary.
package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

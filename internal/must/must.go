// Package must provides runtime assertions for code-owned invariants.
package must

import "fmt"

// NotErrorf panics with the formatted invariant when err is non-nil.
// Use NotErrorf only when the error proves a fault in Cardamom code.
func NotErrorf(err error, format string, args ...any) {
	if err != nil {
		panic(fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err))
	}
}

// NotBeNilf panics with the formatted message when value is nil.
// Use NotBeNilf for dependencies that Cardamom's composition code must supply.
func NotBeNilf(value any, format string, args ...any) {
	if value == nil {
		panic(fmt.Errorf(format, args...))
	}
}

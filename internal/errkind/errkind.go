// Package errkind classifies caller-repairable errors independently of the
// product domain or adapter that produced them.
package errkind

import (
	"errors"
	"fmt"
)

// Kind identifies how a caller can respond to an error.
// Unknown is the zero value for errors without a caller-facing classification.
type Kind int

const (
	// Unknown identifies an error without a caller-facing classification.
	Unknown Kind = iota
	// InvalidInput identifies input that a caller can correct.
	InvalidInput
	// NotFound identifies a requested resource that does not exist.
	NotFound
	// Conflict identifies a request that conflicts with current state.
	Conflict
)

// Wrap associates kind with err while preserving err as the cause.
// It returns nil for a nil error and returns err unchanged for Unknown.
func Wrap(kind Kind, err error) error {
	validate(kind)
	if err == nil || kind == Unknown {
		return err
	}
	return &classifiedError{kind: kind, cause: err}
}

// Errorf formats an error and associates kind with it.
// Formatting follows fmt.Errorf, including support for wrapped causes.
func Errorf(kind Kind, format string, args ...any) error {
	validate(kind)
	return Wrap(kind, fmt.Errorf(format, args...))
}

// Of returns the first classification found while traversing err.
// It returns Unknown when err is nil or no wrapped error is classified.
func Of(err error) Kind {
	var classified *classifiedError
	// errors.As traverses both ordinary and joined unwrap trees in depth-first
	// order, so adapter-added context does not obscure the originating kind.
	if !errors.As(err, &classified) {
		return Unknown
	}
	return classified.kind
}

// classifiedError preserves one validated caller classification and its cause.
type classifiedError struct {
	// kind is one of the nonzero Kind constants.
	kind Kind

	// cause retains the original error chain and diagnostic.
	cause error
}

// Error returns the original error diagnostic.
func (e *classifiedError) Error() string { return e.cause.Error() }

// Unwrap returns the original error chain.
func (e *classifiedError) Unwrap() error { return e.cause }

func validate(kind Kind) {
	// Kind is a closed code-owned enum. An out-of-range value is a programming
	// defect even when the associated operation would otherwise return nil.
	if kind < Unknown || kind > Conflict {
		panic(fmt.Sprintf("errkind: invalid kind %d", kind))
	}
}

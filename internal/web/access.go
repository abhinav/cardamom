package web

import (
	"context"
	"errors"

	"connectrpc.com/connect"
)

// AccessMode identifies the server-side write policy for one web invocation.
type AccessMode int

const (
	// AccessModeReadWrite permits reads and mutations.
	AccessModeReadWrite AccessMode = iota

	// AccessModeReadOnly permits only protobuf methods classified as having no
	// side effects.
	AccessModeReadOnly
)

// readOnlyInterceptor enforces protobuf side-effect classifications before a
// generated Connect handler begins execution.
type readOnlyInterceptor struct{}

var _ connect.Interceptor = readOnlyInterceptor{}

func (readOnlyInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(
		ctx context.Context,
		request connect.AnyRequest,
	) (connect.AnyResponse, error) {
		if err := requireNoSideEffects(request.Spec()); err != nil {
			return nil, err
		}
		return next(ctx, request)
	}
}

func (readOnlyInterceptor) WrapStreamingClient(
	next connect.StreamingClientFunc,
) connect.StreamingClientFunc {
	return next
}

func (readOnlyInterceptor) WrapStreamingHandler(
	next connect.StreamingHandlerFunc,
) connect.StreamingHandlerFunc {
	return func(ctx context.Context, connection connect.StreamingHandlerConn) error {
		if err := requireNoSideEffects(connection.Spec()); err != nil {
			return err
		}
		return next(ctx, connection)
	}
}

func requireNoSideEffects(spec connect.Spec) error {
	if spec.IdempotencyLevel == connect.IdempotencyNoSideEffects {
		return nil
	}
	return connect.NewError(
		connect.CodePermissionDenied,
		errors.New("web server is read-only"),
	)
}

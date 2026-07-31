// Package web owns Connect policies shared by every generated Cardamom service.
package web

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/must"
)

// HandlerConfig supplies the generated service implementations mounted by
// NewHandler.
type HandlerConfig struct {
	// AccessMode selects the server-side write policy for every Connect service.
	AccessMode AccessMode

	// Project serves project catalog and board RPCs.
	Project privatev1connect.ProjectServiceHandler // required

	// Configuration serves layered configuration reads and mutations.
	Configuration privatev1connect.ConfigurationServiceHandler // required

	// Information serves typed store identity and inventory.
	Information privatev1connect.InformationServiceHandler // required

	// Issue serves issue read and mutation RPCs.
	Issue privatev1connect.IssueServiceHandler // required

	// Planning serves issue creation, editing, and graph RPCs.
	Planning privatev1connect.PlanningServiceHandler // required

	// Execution serves eligibility, custody, and lifecycle RPCs.
	Execution privatev1connect.ExecutionServiceHandler // required

	// Checkpoint serves actionable-checkpoint and decision RPCs.
	Checkpoint privatev1connect.CheckpointServiceHandler // required

	// Record serves log entry, state, and result RPCs.
	Record privatev1connect.RecordServiceHandler // required

	// Change serves committed-change streams.
	Change privatev1connect.ChangeServiceHandler // required

	// Dump serves deterministic artifact streams.
	Dump privatev1connect.DumpServiceHandler // required

	// Mail serves store-scoped mailboxes and topic subscriptions.
	Mail privatev1connect.MailServiceHandler // required

	// Lease serves store-scoped resource lease ownership.
	Lease privatev1connect.LeaseServiceHandler // required

	// Attachment serves attachment upload, metadata, and maintenance RPCs.
	Attachment privatev1connect.AttachmentServiceHandler // required
}

// NewHandler registers every generated Cardamom service on one HTTP mux and returns
// their common route prefix.
func NewHandler(cfg HandlerConfig, opts ...connect.HandlerOption) (string, *http.ServeMux) {
	must.NotBeNilf(cfg.Project, "web: project service handler is required")
	must.NotBeNilf(cfg.Configuration, "web: configuration service handler is required")
	must.NotBeNilf(cfg.Information, "web: information service handler is required")
	must.NotBeNilf(cfg.Issue, "web: issue service handler is required")
	must.NotBeNilf(cfg.Planning, "web: planning service handler is required")
	must.NotBeNilf(cfg.Execution, "web: execution service handler is required")
	must.NotBeNilf(cfg.Checkpoint, "web: checkpoint service handler is required")
	must.NotBeNilf(cfg.Record, "web: record service handler is required")
	must.NotBeNilf(cfg.Change, "web: change service handler is required")
	must.NotBeNilf(cfg.Dump, "web: dump service handler is required")
	must.NotBeNilf(cfg.Mail, "web: mail service handler is required")
	must.NotBeNilf(cfg.Lease, "web: lease service handler is required")
	must.NotBeNilf(cfg.Attachment, "web: attachment service handler is required")
	switch cfg.AccessMode {
	case AccessModeReadWrite:
	case AccessModeReadOnly:
		opts = append(opts, connect.WithInterceptors(readOnlyInterceptor{}))
	default:
		panic("web: unsupported access mode")
	}

	mux := http.NewServeMux()
	path, handler := privatev1connect.NewProjectServiceHandler(cfg.Project, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewConfigurationServiceHandler(cfg.Configuration, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewInformationServiceHandler(cfg.Information, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewIssueServiceHandler(cfg.Issue, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewPlanningServiceHandler(cfg.Planning, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewExecutionServiceHandler(cfg.Execution, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewCheckpointServiceHandler(cfg.Checkpoint, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewRecordServiceHandler(cfg.Record, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewChangeServiceHandler(cfg.Change, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewDumpServiceHandler(cfg.Dump, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewMailServiceHandler(cfg.Mail, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewLeaseServiceHandler(cfg.Lease, opts...)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewAttachmentServiceHandler(cfg.Attachment, opts...)
	mux.Handle(path, handler)
	return "/cardamom.private.v1.", mux
}

// FromError preserves caller-repairable domain diagnostics while hiding
// infrastructure details behind a stable internal failure.
func FromError(err error) error {
	if err == nil {
		return nil
	}
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectErr
	}
	code := connect.CodeInternal
	message := "internal server error"
	switch kind := errkind.Of(err); {
	case errors.Is(err, context.Canceled):
		code, message = connect.CodeCanceled, context.Canceled.Error()
	case errors.Is(err, context.DeadlineExceeded):
		code, message = connect.CodeDeadlineExceeded, context.DeadlineExceeded.Error()
	case kind == errkind.InvalidInput:
		code, message = connect.CodeInvalidArgument, err.Error()
	case kind == errkind.NotFound:
		code, message = connect.CodeNotFound, err.Error()
	case kind == errkind.Conflict:
		code, message = connect.CodeFailedPrecondition, err.Error()
	}
	return connect.NewError(code, errors.New(message))
}

package markdown

import (
	"context"
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/markdown/reference"
	"go.abhg.dev/cardamom/internal/must"
)

// AttachmentResolver resolves one board-scoped attachment reference batch.
type AttachmentResolver interface {
	// ResolveAttachments returns input-aligned attachment outcomes.
	ResolveAttachments(context.Context, attachment.ResolveRequest) ([]attachment.Resolution, error)
}

// IssueReferenceResolver resolves one board-scoped issue reference batch.
type IssueReferenceResolver interface {
	// ResolveIssueReferences returns issue IDs known to the board.
	ResolveIssueReferences(
		context.Context,
		board.ID,
		[]issue.ID,
	) ([]issue.ID, error)
}

// LogReferenceResolver resolves one board-scoped log reference batch.
type LogReferenceResolver interface {
	// ResolveLogReferences returns log ownership metadata known to the board.
	ResolveLogReferences(
		context.Context,
		board.ID,
		[]issue.LogID,
	) ([]issue.LogReference, error)
}

// NewWithAttachments constructs a Renderer that resolves attachment
// destinations in board-scoped batches.
func NewWithAttachments(attachments AttachmentResolver) *Renderer {
	must.NotBeNilf(attachments, "markdown: attachment resolver is required")
	return &Renderer{markdown: newGoldmark(), attachments: attachments}
}

// NewWithReferences constructs a Renderer that resolves attachment
// destinations and typed Cardamom references in board-scoped batches.
func NewWithReferences(
	attachments AttachmentResolver,
	issues IssueReferenceResolver,
	logs LogReferenceResolver,
) *Renderer {
	must.NotBeNilf(attachments, "markdown: attachment resolver is required")
	must.NotBeNilf(issues, "markdown: issue reference resolver is required")
	must.NotBeNilf(logs, "markdown: log reference resolver is required")
	return &Renderer{
		markdown:    newGoldmark(),
		attachments: attachments,
		issues:      issues,
		logs:        logs,
	}
}

// RenderBoard renders one response's Markdown sources after resolving unique
// typed references and attachment destinations in bounded board-scoped batches.
func (r *Renderer) RenderBoard(
	ctx context.Context,
	boardID board.ID,
	routePrefix string,
	sources []string,
) ([]string, error) {
	routePrefix, err := validateRoutePrefix(routePrefix)
	if err != nil {
		return nil, err
	}
	documents := make([]parsedDocument, len(sources))
	references := make([]attachmentReference, 0)
	attachmentIDs := make([]attachment.ID, 0)
	issueIDs := make([]issue.ID, 0)
	logIDs := make([]issue.LogID, 0)
	seenAttachments := make(map[attachment.ID]struct{})
	seenIssues := make(map[issue.ID]struct{})
	seenLogs := make(map[issue.LogID]struct{})
	for index, source := range sources {
		documents[index] = r.parse(source)
		documentReferences, err := collectAttachmentReferences(documents[index])
		if err != nil {
			return nil, err
		}
		references = append(references, documentReferences...)
		for _, reference := range documentReferences {
			if reference.id == "" {
				continue
			}
			if _, ok := seenAttachments[reference.id]; ok {
				continue
			}
			seenAttachments[reference.id] = struct{}{}
			attachmentIDs = append(attachmentIDs, reference.id)
		}
		typedIssues, typedAttachments, typedLogs := collectTypedReferences(
			documents[index],
		)
		for _, id := range typedIssues {
			if _, ok := seenIssues[id]; ok {
				continue
			}
			seenIssues[id] = struct{}{}
			issueIDs = append(issueIDs, id)
		}
		for _, id := range typedAttachments {
			if _, ok := seenAttachments[id]; ok {
				continue
			}
			seenAttachments[id] = struct{}{}
			attachmentIDs = append(attachmentIDs, id)
		}
		for _, id := range typedLogs {
			if _, ok := seenLogs[id]; ok {
				continue
			}
			seenLogs[id] = struct{}{}
			logIDs = append(logIDs, id)
		}
	}

	attachmentResolutions, err := r.resolveAttachments(
		ctx,
		boardID,
		attachmentIDs,
	)
	if err != nil {
		return nil, err
	}
	issueReferences, err := r.resolveIssueReferences(ctx, boardID, issueIDs)
	if err != nil {
		return nil, err
	}
	logReferences, err := r.resolveLogReferences(ctx, boardID, logIDs)
	if err != nil {
		return nil, err
	}
	for _, reference := range references {
		applyAttachmentReference(
			routePrefix,
			boardID,
			reference,
			attachmentResolutions[reference.id],
		)
	}

	presentation := &referenceRenderer{
		boardID:     boardID,
		routePrefix: routePrefix,
		attachments: attachmentResolutions,
		issues:      issueReferences,
		logs:        logReferences,
	}
	boardMarkdown := newGoldmarkWithReferences(presentation)
	rendered := make([]string, len(documents))
	for index, document := range documents {
		value, err := renderDocument(boardMarkdown, document)
		if err != nil {
			return nil, err
		}
		rendered[index] = value
	}
	return rendered, nil
}

const (
	maxIssueReferenceBatch = 1000
	maxLogReferenceBatch   = 1000
)

func collectTypedReferences(
	document parsedDocument,
) ([]issue.ID, []attachment.ID, []issue.LogID) {
	var issues []issue.ID
	var attachments []attachment.ID
	var logs []issue.LogID
	_ = ast.Walk(document.document, func(
		node ast.Node,
		entering bool,
	) (ast.WalkStatus, error) {
		value, ok := node.(*reference.Node)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		switch value.Identity.Kind {
		case reference.KindIssue:
			issues = append(issues, issue.ID(value.Identity.ID))
		case reference.KindAttachment:
			attachments = append(
				attachments,
				attachment.ID(value.Identity.ID),
			)
		case reference.KindLog:
			logs = append(logs, issue.LogID(value.Identity.ID))
		}
		return ast.WalkSkipChildren, nil
	})
	return issues, attachments, logs
}

func (r *Renderer) resolveAttachments(
	ctx context.Context,
	boardID board.ID,
	ids []attachment.ID,
) (map[attachment.ID]attachment.Resolution, error) {
	resolutions := make(map[attachment.ID]attachment.Resolution, len(ids))
	if r.attachments == nil {
		return resolutions, nil
	}
	for start := 0; start < len(ids); start += attachment.MaxResolveAttachmentIDs {
		end := min(start+attachment.MaxResolveAttachmentIDs, len(ids))
		batch := ids[start:end]
		values, err := r.attachments.ResolveAttachments(
			ctx,
			attachment.ResolveRequest{
				BoardID: boardID, AttachmentIDs: batch,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("resolve Markdown attachments: %w", err)
		}
		if len(values) != len(batch) {
			return nil, fmt.Errorf(
				"resolve Markdown attachments: got %d results for %d references",
				len(values),
				len(batch),
			)
		}
		for index, value := range values {
			if value.AttachmentID != batch[index] {
				return nil, fmt.Errorf(
					"resolve Markdown attachments: result %d identifies %q, want %q",
					index,
					value.AttachmentID,
					batch[index],
				)
			}
			resolutions[value.AttachmentID] = value
		}
	}
	return resolutions, nil
}

func (r *Renderer) resolveIssueReferences(
	ctx context.Context,
	boardID board.ID,
	ids []issue.ID,
) (map[issue.ID]struct{}, error) {
	if r.issues == nil {
		return nil, nil
	}
	references := make(map[issue.ID]struct{}, len(ids))
	for start := 0; start < len(ids); start += maxIssueReferenceBatch {
		end := min(start+maxIssueReferenceBatch, len(ids))
		batch := ids[start:end]
		values, err := r.issues.ResolveIssueReferences(ctx, boardID, batch)
		if err != nil {
			return nil, fmt.Errorf("resolve Markdown issue references: %w", err)
		}
		requested := make(map[issue.ID]struct{}, len(batch))
		for _, id := range batch {
			requested[id] = struct{}{}
		}
		for _, value := range values {
			if _, ok := requested[value]; !ok {
				return nil, fmt.Errorf(
					"resolve Markdown issue references: unexpected result %q",
					value,
				)
			}
			references[value] = struct{}{}
		}
	}
	return references, nil
}

func (r *Renderer) resolveLogReferences(
	ctx context.Context,
	boardID board.ID,
	ids []issue.LogID,
) (map[issue.LogID]issue.LogReference, error) {
	references := make(map[issue.LogID]issue.LogReference, len(ids))
	if r.logs == nil {
		return references, nil
	}
	for start := 0; start < len(ids); start += maxLogReferenceBatch {
		end := min(start+maxLogReferenceBatch, len(ids))
		batch := ids[start:end]
		values, err := r.logs.ResolveLogReferences(ctx, boardID, batch)
		if err != nil {
			return nil, fmt.Errorf("resolve Markdown log references: %w", err)
		}
		requested := make(map[issue.LogID]struct{}, len(batch))
		for _, id := range batch {
			requested[id] = struct{}{}
		}
		for _, value := range values {
			if _, ok := requested[value.LogID]; !ok {
				return nil, fmt.Errorf(
					"resolve Markdown log references: unexpected result %q",
					value.LogID,
				)
			}
			references[value.LogID] = value
		}
	}
	return references, nil
}

type attachmentReference struct {
	// node is the parsed link or image changed only for presentation.
	node ast.Node
	// id is empty when an attachment destination contains an invalid handle.
	id attachment.ID
	// label is authored link text or image alt text used by fallback markers.
	label string
}

func collectAttachmentReferences(document parsedDocument) ([]attachmentReference, error) {
	result := make([]attachmentReference, 0)
	err := ast.Walk(document.document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		destination, ok := attachmentDestination(node)
		if !ok || !strings.HasPrefix(destination, "attachment:") {
			return ast.WalkContinue, nil
		}
		id, err := attachment.NewID(strings.TrimPrefix(destination, "attachment:"))
		if err != nil {
			id = ""
		}
		result = append(result, attachmentReference{
			node:  node,
			id:    id,
			label: attachmentLabel(node, document.source),
		})
		return ast.WalkSkipChildren, nil
	})
	return result, err
}

func attachmentLabel(node ast.Node, source []byte) string {
	var label strings.Builder
	_ = ast.Walk(node, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch value := child.(type) {
		case *ast.Text:
			label.Write(value.Segment.Value(source))
			if value.SoftLineBreak() || value.HardLineBreak() {
				label.WriteByte(' ')
			}
		case *ast.String:
			label.Write(value.Value)
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(label.String())
}

func attachmentDestination(node ast.Node) (string, bool) {
	switch value := node.(type) {
	case *ast.Link:
		return string(value.Destination), true
	case *ast.Image:
		return string(value.Destination), true
	default:
		return "", false
	}
}

func applyAttachmentReference(
	routePrefix string,
	boardID board.ID,
	reference attachmentReference,
	resolution attachment.Resolution,
) {
	if !availableAttachmentResolution(resolution) {
		label := reference.label
		if label == "" && resolution.Attachment != nil {
			label = resolution.Attachment.Filename.String()
		}
		if label == "" && reference.id != "" {
			label = reference.id.String()
		}
		if label == "" {
			label = "attachment"
		}
		reference.node.Parent().ReplaceChild(
			reference.node.Parent(),
			reference.node,
			ast.NewString([]byte(label+" (attachment unavailable)")),
		)
		return
	}

	value := resolution.Attachment
	destination := attachmentContentURL(routePrefix, boardID, reference.id)
	switch node := reference.node.(type) {
	case *ast.Link:
		node.Destination = []byte(destination)
		appendAttachmentLabel(node, reference.label, value.Filename.String())
	case *ast.Image:
		if attachment.IsInlineMediaType(value.MediaType) {
			node.Destination = []byte(destination)
			appendAttachmentLabel(node, reference.label, value.Filename.String())
			return
		}
		link := ast.NewLink()
		link.Destination = []byte(destination)
		for child := node.FirstChild(); child != nil; {
			next := child.NextSibling()
			node.RemoveChild(node, child)
			link.AppendChild(link, child)
			child = next
		}
		appendAttachmentLabel(link, reference.label, value.Filename.String())
		node.Parent().ReplaceChild(node.Parent(), node, link)
	}
}

func availableAttachmentResolution(resolution attachment.Resolution) bool {
	if resolution.State != attachment.ResolutionActive || resolution.Attachment == nil {
		return false
	}
	switch resolution.Attachment.Availability {
	case attachment.BlobAvailabilityPresentUnverified,
		attachment.BlobAvailabilityVerified:
		return true
	default:
		return false
	}
}

func appendAttachmentLabel(node ast.Node, current, fallback string) {
	if current == "" {
		node.AppendChild(node, ast.NewString([]byte(fallback)))
	}
}

func attachmentContentURL(routePrefix string, boardID board.ID, attachmentID attachment.ID) string {
	return boardEntityURL(
		routePrefix,
		boardID,
		"attachment",
		attachmentID.String(),
		"",
	)
}

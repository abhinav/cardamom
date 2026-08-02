package markdown

import (
	"fmt"
	"net/url"
	"slices"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/markdown/reference"
)

// referenceRenderer presents syntax-certified Cardamom references using
// resolution data owned by one RenderBoard call.
type referenceRenderer struct {
	boardID     board.ID
	routePrefix string
	issues      map[issue.ID]struct{}
	logs        map[issue.LogID]issue.LogReference
	attachments map[attachment.ID]attachment.Resolution
}

var _ renderer.NodeRenderer = (*referenceRenderer)(nil)

// RegisterFuncs registers Cardamom reference rendering with Goldmark.
func (r *referenceRenderer) RegisterFuncs(
	registerer renderer.NodeRendererFuncRegisterer,
) {
	registerer.Register(reference.ASTKind, r.renderReference)
}

func (r *referenceRenderer) renderReference(
	writer util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	referenceNode, ok := node.(*reference.Node)
	if !ok {
		return ast.WalkStop, fmt.Errorf(
			"render Cardamom reference: unexpected node %T",
			node,
		)
	}

	authored := referenceNode.Segment.Value(source)
	switch referenceNode.Identity.Kind {
	case reference.KindIssue:
		id := issue.ID(referenceNode.Identity.ID)
		if r.boardID == "" {
			break
		}
		if r.issues != nil {
			if _, ok := r.issues[id]; !ok {
				break
			}
		}
		return renderIssueReference(writer, authored, r.routePrefix, r.boardID, id)
	case reference.KindLog:
		id := issue.LogID(referenceNode.Identity.ID)
		value, ok := r.logs[id]
		if !ok {
			break
		}
		destination := boardEntityURL(
			r.routePrefix,
			r.boardID,
			"issue",
			value.IssueID.String(),
			id.String(),
		)
		return renderObjectReference(
			writer,
			"log",
			id.String(),
			string(authored),
			destination,
		)
	case reference.KindAttachment:
		id := attachment.ID(referenceNode.Identity.ID)
		value, ok := r.attachments[id]
		if !ok || !availableAttachmentResolution(value) {
			break
		}
		return renderObjectReference(
			writer,
			"attachment",
			id.String(),
			value.Attachment.Filename.String(),
			attachmentContentURL(r.routePrefix, r.boardID, id),
		)
	}

	_, _ = writer.Write(util.EscapeHTML(authored))
	return ast.WalkSkipChildren, nil
}

func renderIssueReference(
	writer util.BufWriter,
	authored []byte,
	routePrefix string,
	boardID board.ID,
	identity issue.ID,
) (ast.WalkStatus, error) {
	id := []byte(identity.String())
	destination := []byte(boardEntityURL(
		routePrefix,
		boardID,
		"issue",
		identity.String(),
		"",
	))
	// Marker attributes let the browser mount its clipboard pill without
	// reparsing rendered HTML. The fallback anchor preserves navigation before
	// browser enhancement.
	_, _ = writer.WriteString(`<span data-issue-reference="`)
	_, _ = writer.Write(util.EscapeHTML(id))
	_, _ = writer.WriteString(`" data-issue-reference-href="`)
	_, _ = writer.Write(util.EscapeHTML(destination))
	_, _ = writer.WriteString(`"><a href="`)
	_, _ = writer.Write(util.EscapeHTML(destination))
	_, _ = writer.WriteString(`">`)
	_, _ = writer.Write(util.EscapeHTML(authored))
	_, _ = writer.WriteString(`</a></span>`)
	return ast.WalkSkipChildren, nil
}

// boardEntityURL preserves each logical identity as one escaped route segment.
func boardEntityURL(
	routePrefix string,
	boardID board.ID,
	kind string,
	identity string,
	fragment string,
) string {
	path := routePrefix + "/" + boardID.String() + "/" + kind + "/" + identity
	rawPath := routePrefix + "/" + url.PathEscape(boardID.String()) +
		"/" + kind + "/" + url.PathEscape(identity)
	return (&url.URL{Path: path, RawPath: rawPath, Fragment: fragment}).String()
}

func renderObjectReference(
	writer util.BufWriter,
	kind string,
	id string,
	label string,
	destination string,
) (ast.WalkStatus, error) {
	_, _ = writer.WriteString(`<span data-cardamom-reference="`)
	_, _ = writer.Write(util.EscapeHTML([]byte(kind)))
	_, _ = writer.WriteString(`" data-cardamom-reference-id="`)
	_, _ = writer.Write(util.EscapeHTML([]byte(id)))
	_, _ = writer.WriteString(`" data-cardamom-reference-label="`)
	_, _ = writer.Write(util.EscapeHTML([]byte(label)))
	_, _ = writer.WriteString(`" data-cardamom-reference-href="`)
	_, _ = writer.Write(util.EscapeHTML([]byte(destination)))
	_, _ = writer.WriteString(`"><a href="`)
	_, _ = writer.Write(util.EscapeHTML([]byte(destination)))
	_, _ = writer.WriteString(`">`)
	_, _ = writer.Write(util.EscapeHTML([]byte(label)))
	_, _ = writer.WriteString(`</a></span>`)
	return ast.WalkSkipChildren, nil
}

// RewriteReferences replaces parsed Cardamom references with caller-rendered
// Markdown while preserving references in escaped, code, and label contexts.
func RewriteReferences(
	source string,
	replace func(reference.Identity) string,
) string {
	value := []byte(source)
	document := newGoldmark().Parser().Parse(text.NewReader(value))
	var edits []referenceEdit
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		referenceNode, ok := node.(*reference.Node)
		if !ok {
			return ast.WalkContinue, nil
		}
		edits = append(edits, referenceEdit{
			start:       referenceNode.Segment.Start,
			end:         referenceNode.Segment.Stop,
			replacement: replace(referenceNode.Identity),
		})
		return ast.WalkSkipChildren, nil
	})

	slices.SortFunc(edits, func(a, b referenceEdit) int {
		return b.start - a.start
	})
	for _, edit := range edits {
		source = source[:edit.start] + edit.replacement + source[edit.end:]
	}
	return source
}

// referenceEdit replaces one half-open byte range in authored Markdown.
type referenceEdit struct {
	// start is the byte offset of the percent prefix.
	start int

	// end is the byte offset immediately after the reference identity.
	end int

	// replacement is caller-rendered Markdown for the reference.
	replacement string
}

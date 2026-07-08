// Package markdown owns Markdown parsing and rendering for presentation and
// generated-file boundaries.
package markdown

import (
	"bytes"
	"fmt"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/cardamom/internal/markdown/reference"
	"go.abhg.dev/goldmark/mermaid"
)

// Renderer applies Cardamom's shared safe Markdown policy.
type Renderer struct {
	markdown goldmark.Markdown // required
	// attachments is nil only for plain rendering without board resolution.
	attachments AttachmentResolver
	// issues is nil only when issue references render without board resolution.
	issues IssueReferenceResolver
	// logs is nil only when log references remain authored text.
	logs LogReferenceResolver
}

// New constructs a Renderer with GFM, server-side code highlighting, Mermaid
// diagram markup, and Goldmark's safe HTML defaults. The browser owns diagram
// rendering and script loading.
func New() *Renderer {
	return &Renderer{markdown: newGoldmark()}
}

func newGoldmark() goldmark.Markdown {
	return newGoldmarkWithReferences(&referenceRenderer{})
}

func newGoldmarkWithReferences(references *referenceRenderer) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			&reference.Extender{},
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
			&mermaid.Extender{
				RenderMode: mermaid.RenderModeClient,
				NoScript:   true,
			},
		),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(
			util.Prioritized(references, 999),
		)),
	)
}

// Render converts one Markdown source value to safe HTML.
func (r *Renderer) Render(source string) (string, error) {
	parsed := r.parse(source)
	return r.render(parsed)
}

type parsedDocument struct {
	source   []byte
	document ast.Node
}

func (r *Renderer) parse(source string) parsedDocument {
	value := []byte(source)
	return parsedDocument{
		source:   value,
		document: r.markdown.Parser().Parse(text.NewReader(value)),
	}
}

func (r *Renderer) render(document parsedDocument) (string, error) {
	return renderDocument(r.markdown, document)
}

func renderDocument(
	markdown goldmark.Markdown,
	document parsedDocument,
) (string, error) {
	var output bytes.Buffer
	if err := markdown.Renderer().Render(
		&output,
		document.source,
		document.document,
	); err != nil {
		return "", fmt.Errorf("convert Markdown: %w", err)
	}
	return output.String(), nil
}

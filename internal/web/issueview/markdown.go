package issueview

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
)

// MarkdownRenderer converts stored Markdown source to trusted presentation
// HTML under the application's shared safety policy.
type MarkdownRenderer interface {
	// RenderBoard converts one board response's Markdown sources to safe HTML.
	RenderBoard(context.Context, board.ID, []string) ([]string, error)
}

// OptionalMarkdown converts optional Markdown source to protocol content.
func (e *Encoder) OptionalMarkdown(
	ctx context.Context,
	boardID board.ID,
	source *string,
) (*privatev1.MarkdownContent, error) {
	if source == nil {
		return nil, nil
	}
	return e.Markdown(ctx, boardID, *source)
}

// Markdown renders source and preserves both source and safe HTML.
func (e *Encoder) Markdown(
	ctx context.Context,
	boardID board.ID,
	source string,
) (*privatev1.MarkdownContent, error) {
	batch := e.newMarkdownBatch(ctx, boardID)
	result := batch.add(source)
	return result, batch.render()
}

type markdownBatch struct {
	ctx      context.Context
	boardID  board.ID
	renderer MarkdownRenderer
	content  []*privatev1.MarkdownContent
}

func (e *Encoder) newMarkdownBatch(ctx context.Context, boardID board.ID) *markdownBatch {
	return &markdownBatch{ctx: ctx, boardID: boardID, renderer: e.markdown}
}

func (b *markdownBatch) add(source string) *privatev1.MarkdownContent {
	result := &privatev1.MarkdownContent{Source: source}
	b.content = append(b.content, result)
	return result
}

func (b *markdownBatch) addOptional(source *string) *privatev1.MarkdownContent {
	if source == nil {
		return nil
	}
	return b.add(*source)
}

func (b *markdownBatch) render() error {
	sources := make([]string, len(b.content))
	for index, content := range b.content {
		sources[index] = content.Source
	}
	rendered, err := b.renderer.RenderBoard(b.ctx, b.boardID, sources)
	if err != nil {
		return fmt.Errorf("render Markdown: %w", err)
	}
	if len(rendered) != len(b.content) {
		return fmt.Errorf(
			"render Markdown: got %d results for %d sources",
			len(rendered),
			len(b.content),
		)
	}
	for index, value := range rendered {
		b.content[index].RenderedHtml = value
	}
	return nil
}

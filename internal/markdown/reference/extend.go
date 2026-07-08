package reference

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"
)

// Extender adds Cardamom reference parsing to Goldmark.
type Extender struct{}

var _ goldmark.Extender = (*Extender)(nil)

// Extend installs Cardamom reference parsing.
func (*Extender) Extend(markdown goldmark.Markdown) {
	markdown.Parser().AddOptions(
		parser.WithInlineParsers(util.Prioritized(&Parser{}, 999)),
		parser.WithASTTransformers(util.Prioritized(&nestedLabelTransformer{}, 999)),
	)
}

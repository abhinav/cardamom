package reference

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/issue"
)

// Parser parses percent-prefixed Cardamom reference nodes.
type Parser struct{}

var _ parser.InlineParser = (*Parser)(nil)

// Trigger reports the character that begins a Cardamom reference.
func (*Parser) Trigger() []byte { return []byte{'%'} }

// Parse parses a Cardamom reference node from the current reader position.
func (*Parser) Parse(parent ast.Node, block text.Reader, _ parser.Context) ast.Node {
	if hasLinkOrImageAncestor(parent) {
		return nil
	}

	line, segment := block.PeekLine()
	if len(line) < 2 || line[0] != '%' {
		return nil
	}
	identity, length, ok := parseIdentity(line[1:])
	if !ok {
		return nil
	}

	segment = segment.WithStop(segment.Start + 1 + length)
	node := &Node{
		Segment:  segment,
		Identity: identity,
	}
	block.Advance(segment.Len())
	return node
}

func parseIdentity(value []byte) (Identity, int, bool) {
	switch {
	case bytes.HasPrefix(value, []byte("log_")):
		return parseLogIdentity(value)
	case bytes.HasPrefix(value, []byte("att_")):
		return parseAttachmentIdentity(value)
	case bytes.HasPrefix(value, []byte("cmt_")):
		return Identity{}, 0, false
	default:
		return parseIssueIdentity(value)
	}
}

func parseIssueIdentity(value []byte) (Identity, int, bool) {
	end := 0
	for end < len(value) && isIssueIDCharacter(value[end]) {
		end++
	}
	if end == 0 {
		return Identity{}, 0, false
	}
	id := string(value[:end])
	if _, err := issue.NewID(id); err != nil {
		return Identity{}, 0, false
	}
	return Identity{Kind: KindIssue, ID: id}, end, true
}

func parseLogIdentity(value []byte) (Identity, int, bool) {
	end := typedIdentityEnd(value)
	id := string(value[:end])
	if _, err := issue.NewLogID(id); err != nil {
		return Identity{}, 0, false
	}
	return Identity{Kind: KindLog, ID: id}, end, true
}

func parseAttachmentIdentity(value []byte) (Identity, int, bool) {
	end := typedIdentityEnd(value)
	id := string(value[:end])
	if _, err := attachment.NewID(id); err != nil {
		return Identity{}, 0, false
	}
	return Identity{Kind: KindAttachment, ID: id}, end, true
}

func typedIdentityEnd(value []byte) int {
	end := 0
	for end < len(value) && isIdentityCharacter(value[end]) {
		end++
	}
	return end
}

func isASCIIAlphanumeric(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9'
}

func isIssueIDCharacter(value byte) bool {
	return isASCIIAlphanumeric(value) || value == '-'
}

func isIdentityCharacter(value byte) bool {
	return isIssueIDCharacter(value) || value == '_'
}

// nestedLabelTransformer restores reference markers that Goldmark parsed
// before constructing their containing link or image.
type nestedLabelTransformer struct{}

var _ parser.ASTTransformer = (*nestedLabelTransformer)(nil)

// Transform restores nested reference markers after Goldmark constructs links.
func (*nestedLabelTransformer) Transform(
	document *ast.Document,
	_ text.Reader,
	_ parser.Context,
) {
	var references []*Node
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		reference, ok := node.(*Node)
		if !entering || !ok || !hasLinkOrImageAncestor(node.Parent()) {
			return ast.WalkContinue, nil
		}
		references = append(references, reference)
		return ast.WalkSkipChildren, nil
	})
	for _, reference := range references {
		parent := reference.Parent()
		parent.ReplaceChild(parent, reference, ast.NewTextSegment(reference.Segment))
	}
}

func hasLinkOrImageAncestor(node ast.Node) bool {
	for ; node != nil; node = node.Parent() {
		switch node.(type) {
		case *ast.Link, *ast.Image:
			return true
		}
	}
	return false
}

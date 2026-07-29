// Package reference adds structured Cardamom references to Goldmark documents.
package reference

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// ASTKind is the kind of Cardamom reference AST nodes.
var ASTKind = ast.NewNodeKind("CardamomReference")

// Kind classifies the identity named by a Cardamom reference.
type Kind uint8

const (
	_ Kind = iota

	// KindIssue identifies an issue reference.
	KindIssue

	// KindLog identifies a current log-entry reference.
	KindLog

	// KindAttachment identifies an attachment reference.
	KindAttachment
)

// Identity is the syntax-certified identity named by a Cardamom reference.
type Identity struct {
	// Kind identifies the Cardamom domain that owns ID.
	Kind Kind

	// ID is the identity text following the percent prefix.
	ID string
}

// Node is a percent-prefixed Cardamom reference in a Goldmark document.
type Node struct {
	ast.BaseInline

	// Segment covers the complete authored reference, including the percent.
	Segment text.Segment

	// Identity is the classified identity named by the reference.
	Identity Identity
}

// Kind reports the Goldmark node kind for Cardamom references.
func (*Node) Kind() ast.NodeKind { return ASTKind }

// Dump dumps the contents of Node to stdout for debugging.
func (n *Node) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"ID": n.Identity.ID,
	}, nil)
}

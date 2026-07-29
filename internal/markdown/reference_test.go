package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.abhg.dev/cardamom/internal/markdown"
	"go.abhg.dev/cardamom/internal/markdown/reference"
)

func TestRewriteReferences_ClassifiesIdentities(t *testing.T) {
	var gotReferences []reference.Identity
	gotSource := markdown.RewriteReferences(
		"%an-issue %log_0123456789abcdef0123456789abcdef "+
			"%att_aaaaaaaaaaaaaaaaaaaaaaaaaa "+
			"%log_0123456789abcdef0123456789abcdeg "+
			"%cmt_0123456789abcdef0123456789abcdef "+
			"`%code-reference` [label %link-label](https://example.com)",
		func(identity reference.Identity) string {
			gotReferences = append(gotReferences, identity)
			return "[" + identity.ID + "]"
		},
	)

	assert.Equal(t, []reference.Identity{
		{Kind: reference.KindIssue, ID: "an-issue"},
		{
			Kind: reference.KindLog,
			ID:   "log_0123456789abcdef0123456789abcdef",
		},
		{
			Kind: reference.KindAttachment,
			ID:   "att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}, gotReferences)
	assert.Equal(t,
		"[an-issue] [log_0123456789abcdef0123456789abcdef] "+
			"[att_aaaaaaaaaaaaaaaaaaaaaaaaaa] "+
			"%log_0123456789abcdef0123456789abcdeg "+
			"%cmt_0123456789abcdef0123456789abcdef "+
			"`%code-reference` [label %link-label](https://example.com)",
		gotSource,
	)
}

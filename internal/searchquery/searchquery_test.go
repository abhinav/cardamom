package searchquery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{name: "Term", give: "alpha", want: `"alpha"`},
		{name: "ImplicitAnd", give: "alpha beta", want: `("alpha" AND "beta")`},
		{name: "Phrase", give: `"alpha beta"`, want: `"alpha beta"`},
		{name: "EscapedPhraseQuote", give: `"alpha ""beta"""`, want: `"alpha ""beta"""`},
		{name: "Prefix", give: "alph*", want: `"alph"*`},
		{name: "Punctuation", give: "PR #1338451", want: `("PR" AND "#1338451")`},
		{name: "OrAfterAnd", give: "alpha beta OR gamma", want: `(("alpha" AND "beta") OR "gamma")`},
		{name: "AndAfterOr", give: "alpha OR beta gamma", want: `("alpha" OR ("beta" AND "gamma"))`},
		{name: "Not", give: "alpha NOT beta", want: `("alpha" NOT "beta")`},
		{name: "Grouped", give: "alpha (beta OR gamma)", want: `("alpha" AND ("beta" OR "gamma"))`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := Parse(tt.give)
			require.NoError(t, err)
			assert.Equal(t, tt.want, query.Expression())
		})
	}
}

func TestParse_rejectsInvalidSyntax(t *testing.T) {
	tests := []struct {
		name string
		give string
	}{
		{name: "Empty"},
		{name: "OnlyWhitespace", give: " \t"},
		{name: "EmptyPhrase", give: `""`},
		{name: "UnclosedPhrase", give: `"alpha`},
		{name: "UnclosedGroup", give: "(alpha"},
		{name: "UnexpectedClose", give: "alpha)"},
		{name: "LeadingOr", give: "OR alpha"},
		{name: "LeadingNot", give: "NOT alpha"},
		{name: "TrailingOperator", give: "alpha OR"},
		{name: "RepeatedPrefix", give: "alpha**"},
		{name: "EmbeddedPrefix", give: "al*pha"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.give)
			assert.ErrorContains(t, err, "invalid search query")
		})
	}
}

func TestLiteral(t *testing.T) {
	query, err := Literal(`alpha "beta"`)
	require.NoError(t, err)
	assert.Equal(t, `"alpha ""beta"""`, query.Expression())

	_, err = Literal(" \t")
	assert.ErrorContains(t, err, "search query must not be blank")
}

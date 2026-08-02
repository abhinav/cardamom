package markdown_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/markdown"
)

func TestRenderer_RenderGFM(t *testing.T) {
	renderer := markdown.New()

	got, err := renderer.Render("~~removed~~\n\n- [x] shipped\n")

	require.NoError(t, err)
	assert.Contains(t, got, "<del>removed</del>")
	assert.Contains(t, got, `<input checked="" disabled="" type="checkbox">`)
}

func TestRenderer_RenderRawHTML(t *testing.T) {
	tests := []struct {
		name string
		give string
	}{
		{name: "Block", give: `<script>alert("bridge")</script>`},
		{name: "Inline", give: `before <span>bridge</span> after`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := markdown.New().Render(tt.give)

			require.NoError(t, err)
			assert.NotContains(t, got, "<script>")
			assert.NotContains(t, got, "<span>")
			assert.NotContains(t, got, "&lt;script")
			assert.NotContains(t, got, "&lt;span")
		})
	}
}

func TestRenderer_RenderMermaidDiagram(t *testing.T) {
	renderer := markdown.New()

	got, err := renderer.Render("```mermaid\ngraph TD\nA --> B\n```\n")

	require.NoError(t, err)
	assert.Equal(t, `<pre class="mermaid">graph TD
A --&gt; B
</pre>`, got)
	assert.NotContains(t, got, "<script")
}

func TestRenderer_RenderHighlightsKnownCodeFence(t *testing.T) {
	renderer := markdown.New()

	got, err := renderer.Render("Before\n\n```go\npackage main\n\nconst message = \"<ready>\"\n```\n\nAfter\n")

	require.NoError(t, err)
	assert.Contains(t, got, "<p>Before</p>")
	assert.Contains(t, got, `<pre class="chroma">`)
	assert.Regexp(t, regexp.MustCompile(`<span class="[^"]+">package</span>`), got)
	assert.Contains(t, got, `<span class="nx">main</span>`)
	assert.Contains(t, got, "&lt;ready&gt;")
	assert.Len(t, regexp.MustCompile(`<span class="line">`).FindAllStringIndex(got, -1), 3)
	assert.Contains(t, got, "<p>After</p>")
}

func TestRenderer_RenderFallsBackForUnhighlightedCodeFences(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{
			name: "UnknownLanguage",
			give: "```unknown-language\nstatus <alert>\n```\n",
			want: "<pre><code class=\"language-unknown-language\">status &lt;alert&gt;\n</code></pre>\n",
		},
		{
			name: "NoLanguage",
			give: "```\nfirst line\nsecond <line>\n```\n",
			want: "<pre><code>first line\nsecond &lt;line&gt;\n</code></pre>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := markdown.New().Render(tt.give)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, `class="chroma"`)
		})
	}
}

func TestRenderer_RenderEmptyInput(t *testing.T) {
	renderer := markdown.New()

	got, err := renderer.Render("")

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRenderer_RenderLeavesUnscopedIssueReferencesLiteral(t *testing.T) {
	source := "See %an-task, %A1-b2; bare an-other and [[an-old]] stay text."
	got, err := markdown.New().Render(source)

	require.NoError(t, err)
	assert.Equal(t, "<p>"+source+"</p>\n", got)
	assert.NotContains(t, got, `<a href=`)
}

func TestRenderer_RenderTypedReferencesLiterally(t *testing.T) {
	got, err := markdown.New().Render(
		"See %log_0123456789abcdef0123456789abcdef and " +
			"%att_aaaaaaaaaaaaaaaaaaaaaaaaaa.",
	)

	require.NoError(t, err)
	assert.Equal(t,
		"<p>See %log_0123456789abcdef0123456789abcdef and "+
			"%att_aaaaaaaaaaaaaaaaaaaaaaaaaa.</p>\n",
		got,
	)
	assert.NotContains(t, got, `<a href=`)
}

func TestRenderer_RenderMalformedTypedReferencesLiterally(t *testing.T) {
	got, err := markdown.New().Render(strings.Join([]string{
		"%log_0123456789abcdef0123456789abcdeg",
		"%log_0123456789abcdef0123456789abcdef0",
		"%log_0123456789ABCDEF0123456789ABCDEF",
		"%att_aaaaaaaaaaaaaaaaaaaaaaaaa0",
		"%att_aaaaaaaaaaaaaaaaaaaaaaaaab",
		"%att_aaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"%cmt_0123456789abcdef0123456789abcdef",
	}, " "))

	require.NoError(t, err)
	assert.Equal(t,
		"<p>%log_0123456789abcdef0123456789abcdeg "+
			"%log_0123456789abcdef0123456789abcdef0 "+
			"%log_0123456789ABCDEF0123456789ABCDEF "+
			"%att_aaaaaaaaaaaaaaaaaaaaaaaaa0 "+
			"%att_aaaaaaaaaaaaaaaaaaaaaaaaab "+
			"%att_aaaaaaaaaaaaaaaaaaaaaaaaaaa "+
			"%cmt_0123456789abcdef0123456789abcdef</p>\n",
		got,
	)
	assert.NotContains(t, got, `<a href=`)
}

func TestRenderer_RenderReferencesInsideLinkLabelLiterally(t *testing.T) {
	got, err := markdown.New().Render(
		`[see %an-task, %log_0123456789abcdef0123456789abcdef, and %att_aaaaaaaaaaaaaaaaaaaaaaaaaa](https://example.com)`,
	)

	require.NoError(t, err)
	assert.Equal(t,
		"<p><a href=\"https://example.com\">see %an-task, "+
			"%log_0123456789abcdef0123456789abcdef, and "+
			"%att_aaaaaaaaaaaaaaaaaaaaaaaaaa</a></p>\n",
		got,
	)
	assert.Equal(t, 1, strings.Count(got, `<a href=`))
}

func TestRenderer_RenderReferencesInsideImageAltLiterally(t *testing.T) {
	got, err := markdown.New().Render(
		`![see %an-task, %log_0123456789abcdef0123456789abcdef, and %att_aaaaaaaaaaaaaaaaaaaaaaaaaa](https://example.com/image.png)`,
	)

	require.NoError(t, err)
	assert.Equal(t,
		"<p><img src=\"https://example.com/image.png\" "+
			"alt=\"see %an-task, "+
			"%log_0123456789abcdef0123456789abcdef, and "+
			"%att_aaaaaaaaaaaaaaaaaaaaaaaaaa\"></p>\n",
		got,
	)
	assert.NotContains(t, got, `<a href=`)
}

func TestRenderer_RenderIssueReferenceBeforePeriod(t *testing.T) {
	got, err := markdown.New().Render("See %an-task.")

	require.NoError(t, err)
	assert.Equal(t, "<p>See %an-task.</p>\n", got)
}

func TestRenderer_RenderIssueReferenceBoundaries(t *testing.T) {
	source := "%a,%b:%c/%d!%e?%f#%g) %dot.after %under_after %with-hyphen " +
		"%1 %/ %-bad %_bad %.bad %\u00e9 bare-id"
	got, err := markdown.New().Render(source)

	require.NoError(t, err)
	assert.Equal(t, "<p>"+source+"</p>\n", got)
	assert.NotContains(t, got, `<a href=`)
}

func TestRenderer_RenderEscapedAndCodeReferencesLiterally(t *testing.T) {
	got, err := markdown.New().Render(strings.Join([]string{
		`Escaped \%an-escaped, \%log_0123456789abcdef0123456789abcdef, and \%att_aaaaaaaaaaaaaaaaaaaaaaaaaa.`,
		"`%an-code %log_0123456789abcdef0123456789abcdef %att_aaaaaaaaaaaaaaaaaaaaaaaaaa`",
		"```markdown\n%an-block %log_0123456789abcdef0123456789abcdef %att_aaaaaaaaaaaaaaaaaaaaaaaaaa\n```",
	}, "\n\n"))

	require.NoError(t, err)
	assert.NotContains(t, got, `<a href=`)
	for _, authored := range []string{
		"%an-escaped",
		"%log_0123456789abcdef0123456789abcdef",
		"%att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
		"%an-code",
		"%an-block",
	} {
		assert.Contains(t, got, authored)
	}
}

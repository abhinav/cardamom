package dump

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnershipFrontmatterIsReadableAndDeterministic(t *testing.T) {
	body := []byte("# Authored heading\n")
	file, err := encodeOwnedFile(testDumpProvenance("board-1"), "issue:an-a", body)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf(`---
cardamom:
  format_version: 1
  project_id: project-1
  project_name: Project one
  board_id: board-1
  board_name: Board one
  generated_identity: issue:an-a
  body_sha256: %s
---

# Authored heading
`, digest(body)), string(file))
}

func TestOwnershipFrontmatterRoundTripsMetadataAndAuthoredBody(t *testing.T) {
	body := []byte("---\n\nAuthored thematic break.\n")
	file, err := encodeOwnedFile(testDumpProvenance("board-1"), "issue:an-a", body)
	require.NoError(t, err)

	metadata, gotBody, err := decodeOwnedFile(file)
	require.NoError(t, err)
	assert.Equal(t, ownershipMetadata{
		Version:   1,
		ProjectID: "project-1", ProjectName: "Project one",
		BoardID: "board-1", BoardName: "Board one",
		Identity: "issue:an-a", BodySHA256: digest(body),
	}, metadata)
	assert.Equal(t, body, gotBody)
}

func TestOwnershipFrontmatterReadsVersionOneWithoutProvenanceNames(t *testing.T) {
	body := []byte("# Legacy generated file\n")
	file := fmt.Appendf(nil, `---
cardamom:
  format_version: 1
  board_id: board-1
  generated_identity: issue:an-a
  body_sha256: %s
---

# Legacy generated file
`, digest(body))

	metadata, gotBody, err := decodeOwnedFile(file)
	require.NoError(t, err)
	assert.Empty(t, metadata.ProjectID)
	assert.Empty(t, metadata.ProjectName)
	assert.Empty(t, metadata.BoardName)
	assert.Equal(t, "board-1", metadata.BoardID)
	assert.Equal(t, body, gotBody)
}

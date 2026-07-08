package attachment

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDigest(t *testing.T) {
	valid := "sha256:" + strings.Repeat("0", 64)

	digest, err := NewDigest(valid)
	require.NoError(t, err)
	assert.Equal(t, valid, digest.String())

	for _, value := range []string{
		"",
		strings.Repeat("0", 64),
		"sha256:" + strings.Repeat("0", 63),
		"sha256:" + strings.Repeat("A", 64),
		"sha256:" + strings.Repeat("g", 64),
	} {
		_, err := NewDigest(value)
		assert.Error(t, err)
	}
}

func TestNewFilename(t *testing.T) {
	for _, value := range []string{
		"artifact.txt",
		"build output.log",
		"resume.pdf",
		strings.Repeat("a", MaxFilenameBytes),
	} {
		filename, err := NewFilename(value)
		require.NoError(t, err)
		assert.Equal(t, value, filename.String())
	}

	tests := []struct {
		name string
		give string
	}{
		{name: "Empty", give: ""},
		{name: "PathSeparator", give: "logs/output.txt"},
		{name: "WindowsPathSeparator", give: `logs\output.txt`},
		{name: "ReservedPunctuation", give: "report?.txt"},
		{name: "ControlCharacter", give: "report\x00.txt"},
		{name: "TrailingDot", give: "report."},
		{name: "TrailingSpace", give: "report "},
		{name: "CurrentDirectory", give: "."},
		{name: "ParentDirectory", give: ".."},
		{name: "WindowsDevice", give: "CON.txt"},
		{name: "TooLong", give: strings.Repeat("a", MaxFilenameBytes+1)},
		{name: "InvalidUTF8", give: string([]byte{0xff})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFilename(tt.give)
			assert.Error(t, err)
		})
	}
}

func TestNewMediaType(t *testing.T) {
	mediaType, err := NewMediaType("image/png")
	require.NoError(t, err)
	assert.Equal(t, "image/png", mediaType.String())

	_, err = NewMediaType("not a media type")
	assert.Error(t, err)
}

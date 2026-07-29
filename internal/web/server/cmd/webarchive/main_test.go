package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteArchive_deterministic(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("cardamom"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("app"), 0o755))

	var first bytes.Buffer
	require.NoError(t, writeArchive(&first, os.DirFS(dir)))
	fixedTime := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "index.html"), fixedTime, fixedTime))

	var second bytes.Buffer
	require.NoError(t, writeArchive(&second, os.DirFS(dir)))
	assert.Equal(t, first.Bytes(), second.Bytes())

	gzipReader, err := gzip.NewReader(bytes.NewReader(first.Bytes()))
	require.NoError(t, err)
	tarReader := tar.NewReader(gzipReader)
	header, err := tarReader.Next()
	require.NoError(t, err)
	assert.Equal(t, "assets/app.js", header.Name)
	assert.Equal(t, int64(0o644), header.Mode)
	assert.Equal(t, time.Unix(0, 0).UTC(), header.ModTime.UTC())
	_, err = io.Copy(io.Discard, tarReader)
	require.NoError(t, err)
}

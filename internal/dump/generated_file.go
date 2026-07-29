package dump

import (
	"errors"
	"fmt"
	"io"
)

// GeneratedFileConfig describes one immutable generated file and how to open
// its content.
type GeneratedFileConfig struct {
	// Path is the canonical artifact-relative path.
	Path string // required

	// Identity remains stable when a generated file moves between paths.
	Identity string // required

	// Size is the number of content bytes available from Open.
	Size int64

	// Open returns an immutable content reader.
	// The caller owns and must close the returned reader.
	Open func() (io.ReadCloser, error) // required
}

// GeneratedFile is one immutable file in a deterministic dump artifact.
//
// Open returns at most Size bytes and transfers close ownership to the caller.
type GeneratedFile struct {
	path     string
	identity string
	size     int64
	open     func() (io.ReadCloser, error)

	// attachmentRole distinguishes Markdown ownership from sidecar-owned raw
	// attachment content.
	attachmentRole attachmentGeneratedRole

	// attachment carries the ownership record shared by one sidecar/content pair.
	attachment *attachmentSidecar
}

// NewGeneratedFile constructs a generated file backed by a lazy content
// opener.
func NewGeneratedFile(cfg GeneratedFileConfig) (*GeneratedFile, error) {
	switch {
	case cfg.Path == "":
		return nil, errors.New("generated file path is required")
	case cfg.Identity == "":
		return nil, errors.New("generated file identity is required")
	case cfg.Size < 0:
		return nil, errors.New("generated file size must not be negative")
	case cfg.Open == nil:
		return nil, errors.New("generated file opener is required")
	default:
		return &GeneratedFile{
			path: cfg.Path, identity: cfg.Identity, size: cfg.Size, open: cfg.Open,
		}, nil
	}
}

// Path returns the canonical artifact-relative path.
func (f *GeneratedFile) Path() string { return f.path }

// Identity returns the stable generated-file identity.
func (f *GeneratedFile) Identity() string { return f.identity }

// Size returns the number of bytes available from Open.
func (f *GeneratedFile) Size() int64 { return f.size }

// Open opens immutable content limited to Size bytes.
// The caller owns and must close the returned reader.
func (f *GeneratedFile) Open() (io.ReadCloser, error) {
	reader, err := f.open()
	if err != nil {
		return nil, fmt.Errorf("open generated file %q: %w", f.path, err)
	}
	if reader == nil {
		return nil, fmt.Errorf("open generated file %q: opener returned a nil reader", f.path)
	}
	return &limitedReadCloser{
		Reader: io.LimitReader(reader, f.size),
		Closer: reader,
	}, nil
}

// limitedReadCloser preserves source close ownership while limiting reads to
// the generated file's declared size.
type limitedReadCloser struct {
	io.Reader
	io.Closer
}

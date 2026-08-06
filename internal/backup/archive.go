package backup

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"slices"
	"strings"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardcopy"
	backupv1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/backup/v1"
	"go.abhg.dev/cardamom/internal/project"
	"google.golang.org/protobuf/proto"
)

const (
	// Version identifies the first portable backup archive contract.
	Version uint32 = 1

	// ManifestMember is the extensionless ZIP member containing the archive
	// index.
	ManifestMember = "manifest"

	// maxManifestBytes is the 64 MiB uncompressed manifest limit.
	maxManifestBytes = 64 << 20
)

// Board identifies one complete board publication available in an archive.
type Board struct {
	// ProjectID identifies the source project containing the board.
	ProjectID project.ID

	// SourceLineageID identifies the source store lineage.
	SourceLineageID string

	// SourceBoardID identifies the board in the source store.
	SourceBoardID board.ID

	// SourceRevision identifies the retained source view.
	SourceRevision int64

	// SnapshotVersion identifies the semantic board-copy contract.
	SnapshotVersion int

	// SnapshotDigest identifies the canonical semantic snapshot.
	SnapshotDigest string

	// member contains the validated ZIP lookup and integrity metadata.
	member archiveMember
}

// Writer streams one portable backup into a ZIP destination.
//
// Close writes the manifest after all board and blob integrity facts are known.
// A content error after a member starts poisons the writer and is returned again
// from Close.
type Writer struct {
	archive  *zip.Writer
	manifest backupv1.Manifest
	projects map[project.ID]struct{}
	boards   map[board.ID]struct{}
	blobs    map[attachment.Digest]struct{}
	err      error
	closed   bool
}

// NewWriter constructs a streaming portable backup writer.
func NewWriter(destination io.Writer) *Writer {
	return &Writer{
		archive:  zip.NewWriter(destination),
		manifest: backupv1.Manifest{Version: Version},
		projects: make(map[project.ID]struct{}),
		boards:   make(map[board.ID]struct{}),
		blobs:    make(map[attachment.Digest]struct{}),
	}
}

// AddProject registers source project metadata referenced by later boards.
func (w *Writer) AddProject(snapshot project.Snapshot) error {
	if err := w.ready(); err != nil {
		return err
	}
	p, err := project.Load(snapshot)
	if err != nil {
		return fmt.Errorf("validate archive project: %w", err)
	}
	if _, exists := w.projects[p.ID()]; exists {
		return fmt.Errorf("archive project %q is duplicated", p.ID())
	}
	encoded, err := projectToProto(project.Snapshot{
		ID: p.ID(), Name: p.Name(), Created: p.Created(),
	})
	if err != nil {
		return fmt.Errorf("encode archive project %q: %w", p.ID(), err)
	}
	w.projects[p.ID()] = struct{}{}
	w.manifest.Projects = append(w.manifest.Projects, encoded)
	return nil
}

// AddBoard writes one complete semantic board-copy snapshot.
func (w *Writer) AddBoard(
	projectID project.ID,
	snapshot boardcopy.CopySnapshot,
) error {
	if err := w.ready(); err != nil {
		return err
	}
	if _, exists := w.projects[projectID]; !exists {
		return fmt.Errorf("archive project %q is not registered", projectID)
	}
	boardID, err := board.NewID(snapshot.Board.ID)
	if err != nil {
		return fmt.Errorf("validate archive board: %w", err)
	}
	if _, exists := w.boards[boardID]; exists {
		return fmt.Errorf("archive board %q is duplicated", boardID)
	}
	if strings.TrimSpace(snapshot.SourceLineageID) == "" {
		return errors.New("archive board source lineage is required")
	}

	snapshot = boardcopy.PrepareSnapshot(snapshot)
	encoded, err := snapshotToProto(snapshot)
	if err != nil {
		return fmt.Errorf("encode archive board %q: %w", boardID, err)
	}
	body, err := proto.MarshalOptions{Deterministic: true}.Marshal(encoded)
	if err != nil {
		return fmt.Errorf("marshal archive board %q: %w", boardID, err)
	}
	memberName, err := boardMember(snapshot.Digest)
	if err != nil {
		return fmt.Errorf("name archive board %q: %w", boardID, err)
	}
	descriptor, err := w.writeBytes(memberName, zip.Deflate, body)
	if err != nil {
		return w.poison(fmt.Errorf("write archive board %q: %w", boardID, err))
	}

	w.boards[boardID] = struct{}{}
	w.manifest.Boards = append(w.manifest.Boards, &backupv1.BoardPublication{
		ProjectId:       projectID.String(),
		SourceLineageId: snapshot.SourceLineageID,
		SourceBoardId:   boardID.String(),
		SourceRevision:  snapshot.SourceRevision,
		SnapshotVersion: uint32(snapshot.Version),
		SnapshotDigest:  snapshot.Digest,
		Member:          memberToProto(descriptor),
	})
	return nil
}

// AddBlob streams one verified, deduplicated attachment body into the archive.
func (w *Writer) AddBlob(
	descriptor attachment.BlobDescriptor,
	source io.Reader,
) error {
	if err := w.ready(); err != nil {
		return err
	}
	if err := descriptor.Validate(); err != nil {
		return fmt.Errorf("validate archive blob: %w", err)
	}
	if _, exists := w.blobs[descriptor.Digest]; exists {
		return fmt.Errorf("archive blob %s is duplicated", descriptor.Digest)
	}
	name := blobMember(descriptor.Digest)
	destination, err := w.archive.CreateHeader(&zip.FileHeader{
		Name:   name,
		Method: zip.Store,
	})
	if err != nil {
		return w.poison(fmt.Errorf("write blob: create member %q: %w", name, err))
	}
	digest := sha256.New()
	size, err := io.Copy(io.MultiWriter(destination, digest), source)
	if err != nil {
		return w.poison(fmt.Errorf("write blob: copy content: %w", err))
	}
	if uint64(size) != descriptor.SizeBytes {
		return w.poison(fmt.Errorf(
			"write blob: size %d does not match expected size %d",
			size,
			descriptor.SizeBytes,
		))
	}
	if digestString(digest.Sum(nil)) != descriptor.Digest.String() {
		return w.poison(fmt.Errorf(
			"write blob: content digest does not match %s",
			descriptor.Digest,
		))
	}

	w.blobs[descriptor.Digest] = struct{}{}
	w.manifest.Blobs = append(w.manifest.Blobs, &backupv1.Blob{
		Digest:    descriptor.Digest.String(),
		SizeBytes: descriptor.SizeBytes,
		Member:    name,
	})
	return nil
}

// Close writes the deterministic manifest and completes the ZIP container.
func (w *Writer) Close() error {
	if w.closed {
		return w.err
	}
	w.closed = true
	if w.err == nil {
		slices.SortFunc(w.manifest.Projects, func(left, right *backupv1.Project) int {
			return strings.Compare(left.GetId(), right.GetId())
		})
		slices.SortFunc(w.manifest.Boards, func(left, right *backupv1.BoardPublication) int {
			return strings.Compare(left.GetSourceBoardId(), right.GetSourceBoardId())
		})
		slices.SortFunc(w.manifest.Blobs, func(left, right *backupv1.Blob) int {
			return strings.Compare(left.GetDigest(), right.GetDigest())
		})
		body, err := proto.MarshalOptions{Deterministic: true}.Marshal(&w.manifest)
		if err != nil {
			w.err = fmt.Errorf("marshal archive manifest: %w", err)
		} else if len(body) > maxManifestBytes {
			w.err = fmt.Errorf(
				"archive manifest size %d exceeds maximum %d",
				len(body),
				maxManifestBytes,
			)
		} else if _, err := w.writeBytes(ManifestMember, zip.Deflate, body); err != nil {
			w.err = fmt.Errorf("write archive manifest: %w", err)
		}
	}
	return errors.Join(w.err, w.archive.Close())
}

func (w *Writer) ready() error {
	if w.closed {
		return errors.New("archive writer is closed")
	}
	if w.err != nil {
		return w.err
	}
	return nil
}

func (w *Writer) poison(err error) error {
	w.err = err
	return err
}

func (w *Writer) writeBytes(
	name string,
	method uint16,
	body []byte,
) (archiveMember, error) {
	destination, err := w.archive.CreateHeader(&zip.FileHeader{
		Name:   name,
		Method: method,
	})
	if err != nil {
		return archiveMember{}, fmt.Errorf("create member %q: %w", name, err)
	}
	if _, err := destination.Write(body); err != nil {
		return archiveMember{}, fmt.Errorf("write member %q: %w", name, err)
	}
	digest := sha256.Sum256(body)
	return archiveMember{
		name: name, sizeBytes: uint64(len(body)),
		digest: digestString(digest[:]),
	}, nil
}

// Reader indexes one validated portable backup without loading member bodies.
type Reader struct {
	files        map[string]*zip.File
	projects     []project.Snapshot
	boards       []Board
	blobs        []attachment.BlobDescriptor
	boardCatalog map[Board]struct{}
	blobCatalog  map[attachment.BlobDescriptor]struct{}
}

// NewReader opens and validates one portable backup ZIP container.
func NewReader(source io.ReaderAt, size int64) (*Reader, error) {
	archive, err := zip.NewReader(source, size)
	if err != nil {
		return nil, fmt.Errorf("open backup ZIP: %w", err)
	}
	files := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		if _, exists := files[file.Name]; exists {
			return nil, fmt.Errorf("archive member %q is duplicated", file.Name)
		}
		files[file.Name] = file
	}
	manifestFile, found := files[ManifestMember]
	if !found {
		return nil, fmt.Errorf("archive is missing member %q", ManifestMember)
	}
	if manifestFile.UncompressedSize64 > maxManifestBytes {
		return nil, fmt.Errorf(
			"archive manifest size %d exceeds maximum %d",
			manifestFile.UncompressedSize64,
			maxManifestBytes,
		)
	}
	body, err := readZIPMember(manifestFile)
	if err != nil {
		return nil, fmt.Errorf("read archive manifest: %w", err)
	}
	var manifest backupv1.Manifest
	if err := proto.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal archive manifest: %w", err)
	}
	projects, boards, blobs, err := validateManifest(&manifest, files)
	if err != nil {
		return nil, err
	}
	boardCatalog := make(map[Board]struct{}, len(boards))
	for _, publication := range boards {
		boardCatalog[publication] = struct{}{}
	}
	blobCatalog := make(map[attachment.BlobDescriptor]struct{}, len(blobs))
	for _, descriptor := range blobs {
		blobCatalog[descriptor] = struct{}{}
	}
	return &Reader{
		files: files, projects: projects, boards: boards, blobs: blobs,
		boardCatalog: boardCatalog, blobCatalog: blobCatalog,
	}, nil
}

// Projects returns source project metadata in manifest order.
func (r *Reader) Projects() []project.Snapshot {
	return slices.Clone(r.projects)
}

// Boards returns complete board publication metadata in manifest order.
func (r *Reader) Boards() []Board {
	return slices.Clone(r.boards)
}

// Blobs returns deduplicated content descriptors in manifest order.
func (r *Reader) Blobs() []attachment.BlobDescriptor {
	return slices.Clone(r.blobs)
}

// ReadBoard reads, verifies, and decodes one publication returned by Boards.
func (r *Reader) ReadBoard(publication Board) (boardcopy.CopySnapshot, error) {
	if _, found := r.boardCatalog[publication]; !found {
		return boardcopy.CopySnapshot{}, fmt.Errorf(
			"archive board %q is not in this catalog",
			publication.SourceBoardID,
		)
	}
	reader, err := openVerifiedMember(r.files[publication.member.name], publication.member)
	if err != nil {
		return boardcopy.CopySnapshot{}, err
	}
	body, readErr := io.ReadAll(reader)
	err = errors.Join(readErr, reader.Close())
	if err != nil {
		return boardcopy.CopySnapshot{}, fmt.Errorf(
			"read archive board %q: %w",
			publication.SourceBoardID,
			err,
		)
	}
	var encoded backupv1.BoardSnapshot
	if err := proto.Unmarshal(body, &encoded); err != nil {
		return boardcopy.CopySnapshot{}, fmt.Errorf(
			"unmarshal archive board %q: %w",
			publication.SourceBoardID,
			err,
		)
	}
	snapshot, err := snapshotFromProto(&encoded)
	if err != nil {
		return boardcopy.CopySnapshot{}, fmt.Errorf(
			"decode archive board %q: %w",
			publication.SourceBoardID,
			err,
		)
	}
	if err := boardcopy.VerifySnapshot(snapshot); err != nil {
		return boardcopy.CopySnapshot{}, fmt.Errorf(
			"verify archive board %q: %w",
			publication.SourceBoardID,
			err,
		)
	}
	if snapshot.SourceLineageID != publication.SourceLineageID ||
		snapshot.SourceRevision != publication.SourceRevision ||
		snapshot.Version != publication.SnapshotVersion ||
		snapshot.Digest != publication.SnapshotDigest ||
		snapshot.Board.ID != publication.SourceBoardID.String() {
		return boardcopy.CopySnapshot{}, fmt.Errorf(
			"archive board %q does not match its manifest publication",
			publication.SourceBoardID,
		)
	}
	return snapshot, nil
}

// OpenBlob opens one descriptor returned by Blobs and verifies its size and
// digest while read.
// Closing the returned reader drains unread content so integrity is still
// established.
func (r *Reader) OpenBlob(
	descriptor attachment.BlobDescriptor,
) (io.ReadCloser, error) {
	if _, found := r.blobCatalog[descriptor]; !found {
		return nil, fmt.Errorf(
			"archive blob %s is not in this catalog",
			descriptor.Digest,
		)
	}
	return openVerifiedMember(r.files[blobMember(descriptor.Digest)], archiveMember{
		name: blobMember(descriptor.Digest), sizeBytes: descriptor.SizeBytes,
		digest: descriptor.Digest.String(),
	})
}

// archiveMember is one normalized manifest descriptor for uncompressed ZIP content.
type archiveMember struct {
	// name is the canonical extensionless ZIP member name.
	name string

	// sizeBytes is the expected uncompressed byte count.
	sizeBytes uint64

	// digest is the canonical SHA-256 digest of uncompressed content.
	digest string
}

func memberToProto(value archiveMember) *backupv1.Member {
	return &backupv1.Member{
		Name: value.name, SizeBytes: value.sizeBytes, Digest: value.digest,
	}
}

func boardMember(digest string) (string, error) {
	parsed, err := attachment.NewDigest(digest)
	if err != nil {
		return "", err
	}
	return "boards/sha256/" + strings.TrimPrefix(parsed.String(), "sha256:"), nil
}

func blobMember(digest attachment.Digest) string {
	return "blobs/sha256/" + strings.TrimPrefix(digest.String(), "sha256:")
}

func digestString(value []byte) string {
	return "sha256:" + hex.EncodeToString(value)
}

func readZIPMember(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(reader)
	return body, errors.Join(readErr, reader.Close())
}

// verifiedMemberReader establishes manifest integrity as member content flows.
type verifiedMemberReader struct {
	source   io.ReadCloser
	expected archiveMember
	hash     hash.Hash
	size     uint64
	err      error
	done     bool
	closed   bool
}

func openVerifiedMember(
	file *zip.File,
	expected archiveMember,
) (io.ReadCloser, error) {
	if file == nil {
		return nil, fmt.Errorf("archive is missing member %q", expected.name)
	}
	source, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open archive member %q: %w", expected.name, err)
	}
	return &verifiedMemberReader{
		source: source, expected: expected, hash: sha256.New(),
	}, nil
}

func (r *verifiedMemberReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, errors.New("archive member reader is closed")
	}
	if r.done {
		if r.err != nil {
			return 0, r.err
		}
		return 0, io.EOF
	}
	n, err := r.source.Read(p)
	if n > 0 {
		r.size += uint64(n)
		_, _ = r.hash.Write(p[:n])
	}
	if err != nil && !errors.Is(err, io.EOF) {
		r.done = true
		r.err = err
		return n, err
	}
	if errors.Is(err, io.EOF) {
		r.done = true
		r.err = r.verify()
		if r.err != nil {
			return n, r.err
		}
	}
	return n, err
}

func (r *verifiedMemberReader) Close() error {
	if r.closed {
		return r.err
	}
	if !r.done {
		_, err := io.Copy(io.Discard, r)
		if err != nil && r.err == nil {
			r.err = err
		}
	}
	r.closed = true
	return errors.Join(r.err, r.source.Close())
}

func (r *verifiedMemberReader) verify() error {
	if r.size != r.expected.sizeBytes {
		return fmt.Errorf(
			"archive member %q size %d does not match expected size %d",
			r.expected.name,
			r.size,
			r.expected.sizeBytes,
		)
	}
	if digestString(r.hash.Sum(nil)) != r.expected.digest {
		return fmt.Errorf(
			"archive member %q content digest mismatch",
			r.expected.name,
		)
	}
	return nil
}

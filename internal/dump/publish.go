package dump

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var validIssueID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// FilePublisher validates ownership and publishes a rendered dump through the
// local filesystem.
//
// Its zero value uses the operating system filesystem.
type FilePublisher struct {
	// files is nil when publication should use the operating system filesystem.
	files fileSystem
}

var _ Publisher = (*FilePublisher)(nil)

// Publish stages rendered files before applying the complete publication change.
//
// Publish observes context cancellation between filesystem operations.
// An individual operating system filesystem call cannot be interrupted.
func (p *FilePublisher) Publish(ctx context.Context, publication Publication) (PublicationResult, error) {
	if err := ctx.Err(); err != nil {
		return PublicationResult{}, err
	}
	files := p.files
	if files == nil {
		files = osFileSystem{}
	}
	return publish(ctx, publication, files)
}

type fileSystem interface {
	Lstat(string) (fs.FileInfo, error)
	Open(string) (io.ReadCloser, error)
	Create(string, fs.FileMode) (io.WriteCloser, error)
	MkdirAll(string, fs.FileMode) error
	MkdirTemp(string, string) (string, error)
	Rename(string, string) error
	Remove(string) error
	RemoveAll(string) error
}

type osFileSystem struct{}

func (osFileSystem) Lstat(name string) (fs.FileInfo, error)  { return os.Lstat(name) }
func (osFileSystem) Open(name string) (io.ReadCloser, error) { return os.Open(name) }
func (osFileSystem) Create(name string, mode fs.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
}
func (osFileSystem) MkdirAll(name string, mode fs.FileMode) error { return os.MkdirAll(name, mode) }
func (osFileSystem) MkdirTemp(directory, pattern string) (string, error) {
	return os.MkdirTemp(directory, pattern)
}

func (osFileSystem) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }
func (osFileSystem) Remove(name string) error             { return os.Remove(name) }
func (osFileSystem) RemoveAll(path string) error          { return os.RemoveAll(path) }

// publicationChange carries one validated transition from preflight through
// staging and rollback-capable application.
type publicationChange struct {
	publication Publication

	// writePaths and removePaths are the selected transaction's mutations.
	writePaths     []string
	removePaths    []string
	unchangedPaths []string

	// existingTargets identifies write paths that require rollback backups.
	existingTargets map[string]bool

	// destinationExists distinguishes an existing destination from one that
	// staging created and must remove after a pre-publication failure.
	destinationExists bool

	// stage retains next files and rollback backups until publication commits.
	stage string

	// nextRoot contains fully consumed and validated generated files.
	nextRoot string

	// backupRoot receives prior generated files during publication.
	backupRoot string
}

func publish(
	ctx context.Context,
	publication Publication,
	files fileSystem,
) (PublicationResult, error) {
	if err := preflightAttachmentFiles(publication.Rendered.Files); err != nil {
		return PublicationResult{}, err
	}

	change, err := preflight(ctx, publication, files)
	if err != nil {
		return PublicationResult{}, err
	}
	if err := stageChange(ctx, &change, files); err != nil {
		return PublicationResult{}, err
	}
	result := PublicationResult{
		Written: len(change.writePaths), Unchanged: len(change.unchangedPaths), Removed: len(change.removePaths),
	}
	if err := applyChange(ctx, change, files); err != nil {
		return PublicationResult{}, err
	}
	return result, nil
}

func preflight(ctx context.Context, publication Publication, files fileSystem) (publicationChange, error) {
	if err := ctx.Err(); err != nil {
		return publicationChange{}, err
	}
	if publication.Destination == "" {
		return publicationChange{}, errors.New("dump destination is required")
	}
	if err := publication.Rendered.Provenance.validate(); err != nil {
		return publicationChange{}, fmt.Errorf("rendered %w", err)
	}
	if publication.Force != PreserveGenerated && publication.Force != ForceGenerated {
		return publicationChange{}, fmt.Errorf("unsupported force authorization %d", publication.Force)
	}
	change := publicationChange{
		publication:     publication,
		existingTargets: make(map[string]bool),
	}

	info, err := files.Lstat(publication.Destination)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return publicationChange{}, fmt.Errorf("inspect destination %q: %w", publication.Destination, err)
	case info.Mode()&os.ModeSymlink != 0:
		return publicationChange{}, fmt.Errorf("destination %q is a symbolic link", publication.Destination)
	case !info.IsDir():
		return publicationChange{}, fmt.Errorf("destination %q is not a directory", publication.Destination)
	default:
		change.destinationExists = true
	}

	paths := make(map[string]struct{}, len(publication.Rendered.Files))
	identities := make(map[string]string, len(publication.Rendered.Files))
	for _, generated := range publication.Rendered.Files {
		if err := validateGeneratedFile(generated); err != nil {
			return publicationChange{}, err
		}
		if _, duplicate := paths[generated.Path()]; duplicate {
			return publicationChange{}, fmt.Errorf("generated path %q is duplicated", generated.Path())
		}
		if priorPath, duplicate := identities[generated.Identity()]; duplicate {
			return publicationChange{}, fmt.Errorf("generated identity %q is duplicated by %q and %q", generated.Identity(), priorPath, generated.Path())
		}
		paths[generated.Path()] = struct{}{}
		identities[generated.Identity()] = generated.Path()
	}

	for _, generated := range publication.Rendered.Files {
		if err := rejectSymlinkTraversal(publication.Destination, generated.Path(), files); err != nil {
			return publicationChange{}, err
		}
		present, err := inspectPath(publication.Destination, generated.Path(), files)
		if err != nil {
			return publicationChange{}, err
		}
		if !present {
			change.writePaths = append(change.writePaths, generated.Path())
		} else {
			if generated.attachmentRole != attachmentGeneratedNone {
				if err := authorizeAttachmentPath(publication, generated, files); err != nil {
					return publicationChange{}, err
				}
				change.existingTargets[generated.Path()] = true
				continue
			}
			metadata, bodyDigest, err := readOwnedPath(publication.Destination, generated.Path(), files)
			if errors.Is(err, errNotOwned) {
				return publicationChange{}, fmt.Errorf("generated path %q collides with an unowned file", generated.Path())
			}
			if err != nil {
				return publicationChange{}, fmt.Errorf("generated path %q has invalid ownership metadata: %w", generated.Path(), err)
			}
			if err := authorizeOwnedPath(publication, generated.Path(), generated.Identity(), metadata, bodyDigest); err != nil {
				return publicationChange{}, err
			}
			change.existingTargets[generated.Path()] = true
		}

		legacy := legacyIssuePath(generated.Path())
		if legacy == "" {
			continue
		}
		if err := rejectSymlinkTraversal(publication.Destination, legacy, files); err != nil {
			return publicationChange{}, err
		}
		present, err = inspectPath(publication.Destination, legacy, files)
		if err != nil || !present {
			if err != nil {
				return publicationChange{}, err
			}
			continue
		}
		metadata, bodyDigest, err := readOwnedPath(publication.Destination, legacy, files)
		if errors.Is(err, errNotOwned) {
			// Only recognized generated metadata proves that the old canonical
			// path belongs to this selected issue.
			continue
		}
		if err != nil {
			return publicationChange{}, fmt.Errorf("generated counterpart %q has invalid ownership metadata: %w", legacy, err)
		}
		if err := authorizeOwnedPath(publication, legacy, generated.Identity(), metadata, bodyDigest); err != nil {
			return publicationChange{}, err
		}
		change.removePaths = append(change.removePaths, legacy)
	}
	slices.Sort(change.removePaths)
	return change, nil
}

func validateGeneratedFile(file *GeneratedFile) error {
	if file == nil {
		return errors.New("generated file is required")
	}
	if file.open == nil {
		return fmt.Errorf("generated path %q has no content opener", file.Path())
	}
	if file.Size() < 0 {
		return fmt.Errorf("generated path %q has a negative size", file.Path())
	}
	if file.attachmentRole != attachmentGeneratedNone {
		return validateAttachmentGeneratedFile(file)
	}
	if file.Path() == "README.md" {
		if file.Identity() != "board" {
			return fmt.Errorf("generated path %q requires identity %q", file.Path(), "board")
		}
		return nil
	}
	if file.Path() == "" || file.Path() != path.Clean(file.Path()) || path.IsAbs(file.Path()) || strings.HasPrefix(file.Path(), "../") || strings.Contains(file.Path(), `\`) {
		return fmt.Errorf("generated path %q is not a canonical dump path", file.Path())
	}
	directory, name := path.Split(file.Path())
	if directory != "issues/" || !strings.HasSuffix(name, ".md") {
		return fmt.Errorf("generated path %q is not a canonical dump path", file.Path())
	}
	issueID := strings.TrimSuffix(name, ".md")
	if !validIssueID.MatchString(issueID) || issueID == "." || issueID == ".." {
		return fmt.Errorf("generated path %q has an invalid issue ID", file.Path())
	}
	if expected := "issue:" + issueID; file.Identity() != expected {
		return fmt.Errorf("generated path %q requires identity %q", file.Path(), expected)
	}
	return nil
}

func validateGeneratedOwnership(
	provenance Provenance,
	file *GeneratedFile,
	metadata ownershipMetadata,
	bodyDigest string,
) error {
	if metadata.ProjectID != provenance.ProjectID {
		return fmt.Errorf("generated path %q belongs to project %q, not %q", file.Path(), metadata.ProjectID, provenance.ProjectID)
	}
	if metadata.ProjectName != provenance.ProjectName {
		return fmt.Errorf("generated path %q names project %q, not %q", file.Path(), metadata.ProjectName, provenance.ProjectName)
	}
	if metadata.BoardID != provenance.BoardID {
		return fmt.Errorf("generated path %q belongs to board %q, not %q", file.Path(), metadata.BoardID, provenance.BoardID)
	}
	if metadata.BoardName != provenance.BoardName {
		return fmt.Errorf("generated path %q names board %q, not %q", file.Path(), metadata.BoardName, provenance.BoardName)
	}
	if metadata.Identity != file.Identity() {
		return fmt.Errorf("generated path %q belongs to identity %q, not %q", file.Path(), metadata.Identity, file.Identity())
	}
	if bodyDigest != metadata.BodySHA256 {
		return fmt.Errorf("generated path %q ownership body digest does not match content", file.Path())
	}
	return nil
}

func inspectPath(destination, relative string, files fileSystem) (bool, error) {
	fullPath := filepath.Join(destination, filepath.FromSlash(relative))
	info, err := files.Lstat(fullPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect generated path %q: %w", relative, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("generated path %q is not a regular file", relative)
	}
	return true, nil
}

func readOwnedPath(
	destination string,
	relative string,
	files fileSystem,
) (ownershipMetadata, string, error) {
	reader, err := files.Open(filepath.Join(destination, filepath.FromSlash(relative)))
	if err != nil {
		return ownershipMetadata{}, "", fmt.Errorf("open generated path %q: %w", relative, err)
	}
	metadata, bodyDigest, readErr := decodeOwnedReader(reader)
	closeErr := reader.Close()
	return metadata, bodyDigest, errors.Join(readErr, closeErr)
}

func authorizeOwnedPath(publication Publication, relative, identity string, metadata ownershipMetadata, bodyDigest string) error {
	if metadata.BoardID != publication.Rendered.Provenance.BoardID {
		return fmt.Errorf("generated path %q belongs to board %q, not %q", relative, metadata.BoardID, publication.Rendered.Provenance.BoardID)
	}
	if metadata.Identity != identity {
		return fmt.Errorf("generated path %q belongs to identity %q, not %q", relative, metadata.Identity, identity)
	}
	if bodyDigest != metadata.BodySHA256 && publication.Force != ForceGenerated {
		return &GeneratedFileError{Path: relative, Kind: GeneratedFileModified}
	}
	return nil
}

func legacyIssuePath(relative string) string {
	if !strings.HasPrefix(relative, "issues/") {
		return ""
	}
	return "missions/" + strings.TrimPrefix(relative, "issues/")
}

func rejectSymlinkTraversal(destination, relative string, files fileSystem) error {
	current := destination
	parts := strings.Split(filepath.FromSlash(relative), string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := files.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect generated path %q: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			traversed := filepath.ToSlash(filepath.Join(parts[:index+1]...))
			return fmt.Errorf("generated path %q traverses symbolic link %q", relative, traversed)
		}
		if index < len(parts)-1 && !info.IsDir() {
			traversed := filepath.ToSlash(filepath.Join(parts[:index+1]...))
			return fmt.Errorf("generated path %q traverses non-directory %q", relative, traversed)
		}
	}
	return nil
}

func stageChange(
	ctx context.Context,
	change *publicationChange,
	files fileSystem,
) error {
	if !change.destinationExists {
		if err := files.MkdirAll(change.publication.Destination, 0o755); err != nil {
			return fmt.Errorf("create destination: %w", err)
		}
	}
	stage, err := files.MkdirTemp(change.publication.Destination, ".cardamom-dump-stage-")
	if err != nil {
		return discardStage(*change, files, fmt.Errorf("create publication staging directory: %w", err))
	}
	change.stage = stage
	change.nextRoot = filepath.Join(stage, "next")
	change.backupRoot = filepath.Join(stage, "backup")
	if err := files.MkdirAll(change.nextRoot, 0o700); err != nil {
		return discardStage(*change, files, fmt.Errorf("create next-file staging directory: %w", err))
	}

	for _, generated := range change.publication.Rendered.Files {
		if err := ctx.Err(); err != nil {
			return discardStage(*change, files, err)
		}
		staged := filepath.Join(change.nextRoot, filepath.FromSlash(generated.Path()))
		if err := files.MkdirAll(filepath.Dir(staged), 0o700); err != nil {
			return discardStage(*change, files, fmt.Errorf("stage directory for %q: %w", generated.Path(), err))
		}
		if generated.attachmentRole == attachmentGeneratedNone {
			metadata, bodyDigest, err := stageGeneratedFile(generated, staged, files)
			if err != nil {
				return discardStage(*change, files, err)
			}
			if err := validateGeneratedOwnership(
				change.publication.Rendered.Provenance,
				generated,
				metadata,
				bodyDigest,
			); err != nil {
				return discardStage(*change, files, err)
			}
		} else {
			if err := stageAttachmentGeneratedFile(generated, staged, files); err != nil {
				return discardStage(*change, files, err)
			}
		}
		if !change.existingTargets[generated.Path()] {
			continue
		}
		equal, err := equalFiles(
			staged,
			filepath.Join(change.publication.Destination, filepath.FromSlash(generated.Path())),
			files,
		)
		if err != nil {
			return discardStage(*change, files, fmt.Errorf("compare generated path %q: %w", generated.Path(), err))
		}
		if equal {
			change.unchangedPaths = append(change.unchangedPaths, generated.Path())
		} else {
			change.writePaths = append(change.writePaths, generated.Path())
		}
	}
	slices.Sort(change.writePaths)
	slices.Sort(change.unchangedPaths)
	return nil
}

func stageGeneratedFile(
	generated *GeneratedFile,
	destination string,
	files fileSystem,
) (ownershipMetadata, string, error) {
	writer, err := files.Create(destination, 0o644)
	if err != nil {
		return ownershipMetadata{}, "", fmt.Errorf("stage %q: %w", generated.Path(), err)
	}
	reader, err := generated.Open()
	if err != nil {
		closeErr := writer.Close()
		return ownershipMetadata{}, "", errors.Join(err, closeErr)
	}

	counting := &countingWriter{Writer: writer}
	metadata, bodyDigest, readErr := decodeOwnedReader(io.TeeReader(reader, counting))
	closeErr := errors.Join(reader.Close(), writer.Close())
	if readErr != nil {
		return ownershipMetadata{}, "", errors.Join(
			fmt.Errorf("generated path %q has invalid ownership metadata: %w", generated.Path(), readErr),
			closeErr,
		)
	}
	if closeErr != nil {
		return ownershipMetadata{}, "", fmt.Errorf("close staged generated path %q: %w", generated.Path(), closeErr)
	}
	if counting.written != generated.Size() {
		return ownershipMetadata{}, "", fmt.Errorf(
			"generated path %q provided %d bytes, expected %d",
			generated.Path(), counting.written, generated.Size(),
		)
	}
	return metadata, bodyDigest, nil
}

type countingWriter struct {
	io.Writer
	written int64
}

func (w *countingWriter) Write(body []byte) (int, error) {
	written, err := w.Writer.Write(body)
	w.written += int64(written)
	return written, err
}

func equalFiles(leftPath, rightPath string, files fileSystem) (bool, error) {
	leftDigest, leftSize, err := fileDigest(leftPath, files)
	if err != nil {
		return false, err
	}
	rightDigest, rightSize, err := fileDigest(rightPath, files)
	if err != nil {
		return false, err
	}
	return leftSize == rightSize && leftDigest == rightDigest, nil
}

func fileDigest(path string, files fileSystem) ([sha256.Size]byte, int64, error) {
	reader, err := files.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, 0, fmt.Errorf("open %q: %w", path, err)
	}
	hash := sha256.New()
	size, readErr := io.Copy(hash, reader)
	closeErr := reader.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return [sha256.Size]byte{}, 0, fmt.Errorf("read %q: %w", path, err)
	}
	return [sha256.Size]byte(hash.Sum(nil)), size, nil
}

func discardStage(change publicationChange, files fileSystem, cause error) error {
	if change.stage != "" {
		if err := files.RemoveAll(change.stage); err != nil {
			cause = errors.Join(cause, fmt.Errorf("remove staging directory %q: %w", change.stage, err))
		}
	}
	if err := cleanupNewDestination(change, files); err != nil {
		cause = errors.Join(cause, fmt.Errorf("clean new destination: %w", err))
	}
	return cause
}

// appliedPath records enough state to reverse one generated-file transition.
type appliedPath struct {
	target    string
	backup    string
	hadBackup bool
	installed bool
}

func applyChange(ctx context.Context, change publicationChange, files fileSystem) error {
	if err := ctx.Err(); err != nil {
		return discardStage(change, files, err)
	}
	if len(change.writePaths) == 0 && len(change.removePaths) == 0 {
		return discardStage(change, files, nil)
	}

	var applied []appliedPath
	// Rollback ignores cancellation because restoring the preflighted state is
	// more important than stopping between recovery operations.
	fail := func(publicationErr error) error {
		rollbackErr := rollback(applied, files)
		if rollbackErr != nil {
			return &PartialRecoveryError{
				PublicationError: publicationErr, RollbackError: rollbackErr, RecoveryDirectory: change.stage,
			}
		}
		if err := files.RemoveAll(change.stage); err != nil {
			return fmt.Errorf("rollback completed but remove staging directory %q: %w", change.stage, errors.Join(publicationErr, err))
		}
		if err := cleanupNewDestination(change, files); err != nil {
			return fmt.Errorf("rollback restored generated files but clean new destination: %w", errors.Join(publicationErr, err))
		}
		return publicationErr
	}

	for _, relative := range change.writePaths {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		target := filepath.Join(change.publication.Destination, filepath.FromSlash(relative))
		if err := files.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fail(fmt.Errorf("create destination directory for %q: %w", relative, err))
		}
		action := appliedPath{target: target, backup: filepath.Join(change.backupRoot, filepath.FromSlash(relative))}
		if change.existingTargets[relative] {
			if err := files.MkdirAll(filepath.Dir(action.backup), 0o700); err != nil {
				return fail(fmt.Errorf("create backup directory for %q: %w", relative, err))
			}
			if err := files.Rename(target, action.backup); err != nil {
				return fail(fmt.Errorf("back up %q: %w", relative, err))
			}
			action.hadBackup = true
		}
		applied = append(applied, action)
		if err := files.Rename(filepath.Join(change.nextRoot, filepath.FromSlash(relative)), target); err != nil {
			return fail(fmt.Errorf("install %q: %w", relative, err))
		}
		applied[len(applied)-1].installed = true
	}
	for _, relative := range change.removePaths {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		action := appliedPath{
			target: filepath.Join(change.publication.Destination, filepath.FromSlash(relative)),
			backup: filepath.Join(change.backupRoot, filepath.FromSlash(relative)), hadBackup: true,
		}
		if err := files.MkdirAll(filepath.Dir(action.backup), 0o700); err != nil {
			return fail(fmt.Errorf("create backup directory for %q: %w", relative, err))
		}
		if err := files.Rename(action.target, action.backup); err != nil {
			return fail(fmt.Errorf("stage obsolete generated path %q: %w", relative, err))
		}
		applied = append(applied, action)
	}
	if err := files.RemoveAll(change.stage); err != nil {
		return fmt.Errorf("dump published but remove staging directory %q: %w", change.stage, err)
	}
	return nil
}

func cleanupNewDestination(change publicationChange, files fileSystem) error {
	if change.destinationExists {
		return nil
	}
	var cleanupErr error
	for _, directory := range []string{"attachments", "issues", "missions"} {
		err := files.Remove(filepath.Join(change.publication.Destination, directory))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove %q: %w", directory, err))
		}
	}
	if cleanupErr != nil {
		return cleanupErr
	}
	if err := files.Remove(change.publication.Destination); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove destination: %w", err)
	}
	return nil
}

func rollback(applied []appliedPath, files fileSystem) error {
	var rollbackErr error
	for _, action := range slices.Backward(applied) {
		if action.installed {
			if err := files.Remove(action.target); err != nil && !errors.Is(err, fs.ErrNotExist) {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove next file %q: %w", action.target, err))
			}
		}
		if action.hadBackup {
			if err := files.MkdirAll(filepath.Dir(action.target), 0o755); err != nil {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("recreate directory for %q: %w", action.target, err))
				continue
			}
			if err := files.Rename(action.backup, action.target); err != nil {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore %q: %w", action.target, err))
			}
		}
	}
	return rollbackErr
}

func digest(body []byte) string {
	value := sha256.Sum256(body)
	return hex.EncodeToString(value[:])
}

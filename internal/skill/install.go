// Package skill installs the Cardamom runtime skill embedded in release
// binaries.
package skill

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var embeddedArchive []byte

// ExistingMode selects how installation handles a different destination.
type ExistingMode uint8

const (
	// PreserveExisting rejects a different destination without changing it.
	PreserveExisting ExistingMode = iota

	// ReplaceExisting replaces a different destination with the embedded skill.
	ReplaceExisting
)

// InstallStatus reports the effect of one installation.
type InstallStatus string

const (
	// StatusInstalled reports publication to a missing destination.
	StatusInstalled InstallStatus = "installed"

	// StatusUnchanged reports that the destination already matched.
	StatusUnchanged InstallStatus = "unchanged"

	// StatusReplaced reports replacement of a different destination.
	StatusReplaced InstallStatus = "replaced"
)

// InstallRequest selects the parent skills directory and existing-content
// policy for one installation.
type InstallRequest struct {
	// SkillsDirectory is the parent directory that will contain cardamom.
	SkillsDirectory string // required

	// Existing selects whether a different cardamom destination is preserved or
	// replaced.
	Existing ExistingMode
}

// InstallResult identifies the installed destination and operation effect.
type InstallResult struct {
	// Destination is the absolute cardamom skill directory.
	Destination string `json:"destination"`

	// Status reports whether the destination was installed, unchanged, or
	// replaced.
	Status InstallStatus `json:"status"`
}

// Installer publishes the immutable skill payload embedded in the running
// binary.
type Installer struct {
	archive []byte
}

// NewInstaller constructs an Installer from the process build's embedded
// payload.
func NewInstaller() *Installer {
	return &Installer{archive: embeddedArchive}
}

// Install validates and stages the complete embedded skill before publishing
// it beneath the requested skills directory.
func (i *Installer) Install(
	ctx context.Context,
	request InstallRequest,
) (InstallResult, error) {
	if ctx == nil {
		return InstallResult{}, errors.New("context is required")
	}
	if request.SkillsDirectory == "" {
		return InstallResult{}, errors.New("skills directory is required")
	}
	if request.Existing != PreserveExisting &&
		request.Existing != ReplaceExisting {
		return InstallResult{}, fmt.Errorf(
			"unsupported existing destination mode %d",
			request.Existing,
		)
	}
	if len(i.archive) == 0 {
		return InstallResult{}, errors.New(
			"embedded skill assets not found; run an asset-bearing build",
		)
	}

	skillsDirectory, err := filepath.Abs(request.SkillsDirectory)
	if err != nil {
		return InstallResult{}, fmt.Errorf(
			"resolve skills directory %q: %w",
			request.SkillsDirectory,
			err,
		)
	}
	if err := os.MkdirAll(skillsDirectory, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf(
			"create skills directory %q: %w",
			skillsDirectory,
			err,
		)
	}

	// Complete and validate the new tree beside its destination before
	// inspecting or changing an existing installation.
	staging, err := os.MkdirTemp(skillsDirectory, ".cardamom-install-*")
	if err != nil {
		return InstallResult{}, fmt.Errorf(
			"create skill staging directory in %q: %w",
			skillsDirectory,
			err,
		)
	}
	defer func() { _ = os.RemoveAll(staging) }()

	if err := extractArchive(ctx, i.archive, staging); err != nil {
		return InstallResult{}, err
	}

	destination := filepath.Join(skillsDirectory, "cardamom")
	result := InstallResult{Destination: destination}
	_, err = os.Lstat(destination)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.Rename(staging, destination); err != nil {
			return InstallResult{}, fmt.Errorf(
				"publish skill destination %q: %w",
				destination,
				err,
			)
		}
		result.Status = StatusInstalled
		return result, nil
	}
	if err != nil {
		return InstallResult{}, fmt.Errorf(
			"inspect skill destination %q: %w",
			destination,
			err,
		)
	}

	// A full tree comparison makes extra files and changed executable bits part
	// of the destination conflict instead of leaving stale runtime behavior.
	identical, err := treesEqual(ctx, staging, destination)
	if err != nil {
		return InstallResult{}, fmt.Errorf(
			"compare skill destination %q: %w",
			destination,
			err,
		)
	}
	if identical {
		result.Status = StatusUnchanged
		return result, nil
	}
	if request.Existing == PreserveExisting {
		return InstallResult{}, fmt.Errorf(
			"skill destination %q differs from the embedded Cardamom skill; "+
				"rerun with --force to replace it",
			destination,
		)
	}

	// Retain the previous tree until the staged tree reaches the public path.
	// A failed publication restores the previous destination before returning.
	backup := staging + ".previous"
	if err := os.Rename(destination, backup); err != nil {
		return InstallResult{}, fmt.Errorf(
			"preserve skill destination %q for replacement: %w",
			destination,
			err,
		)
	}
	if err := os.Rename(staging, destination); err != nil {
		rollbackErr := os.Rename(backup, destination)
		publishErr := fmt.Errorf("publish replacement at %q: %w", destination, err)
		if rollbackErr == nil {
			return InstallResult{}, publishErr
		}
		return InstallResult{}, errors.Join(publishErr, fmt.Errorf(
			"restore prior skill destination %q from %q: %w",
			destination,
			backup,
			rollbackErr,
		))
	}
	if err := os.RemoveAll(backup); err != nil {
		return InstallResult{}, fmt.Errorf(
			"remove prior skill destination %q: %w",
			backup,
			err,
		)
	}
	result.Status = StatusReplaced
	return result, nil
}

func extractArchive(ctx context.Context, archive []byte, destination string) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open embedded skill archive: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()

	files := make(map[string]struct{})
	tarReader := tar.NewReader(gzipReader)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read embedded skill archive: %w", err)
		}
		if err := validateArchiveHeader(header, files); err != nil {
			return err
		}

		name := filepath.FromSlash(header.Name)
		output := filepath.Join(destination, name)
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return fmt.Errorf("create parent for embedded skill path %q: %w", header.Name, err)
		}
		file, err := os.OpenFile(
			output,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			fs.FileMode(header.Mode).Perm(),
		)
		if err != nil {
			return fmt.Errorf("create embedded skill path %q: %w", header.Name, err)
		}
		_, copyErr := io.Copy(file, tarReader)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return fmt.Errorf("write embedded skill path %q: %w", header.Name, err)
		}
	}
	if _, ok := files["SKILL.md"]; !ok {
		return errors.New("embedded skill archive does not contain SKILL.md")
	}
	if !containsDirectory(files, "references") {
		return errors.New("embedded skill archive does not contain references")
	}
	if !containsDirectory(files, "scripts") {
		return errors.New("embedded skill archive does not contain scripts")
	}
	if _, err := io.Copy(io.Discard, gzipReader); err != nil {
		return fmt.Errorf("finish embedded skill archive: %w", err)
	}
	if err := gzipReader.Close(); err != nil {
		return fmt.Errorf("close embedded skill archive: %w", err)
	}
	return nil
}

func validateArchiveHeader(
	header *tar.Header,
	files map[string]struct{},
) error {
	name := strings.TrimSuffix(header.Name, "/")
	if name == "" || strings.ContainsRune(name, '\\') ||
		path.Clean(name) != name || !fs.ValidPath(name) {
		return fmt.Errorf("invalid embedded skill archive path %q", header.Name)
	}
	top, _, _ := strings.Cut(name, "/")
	if top != "SKILL.md" && top != "references" && top != "scripts" {
		return fmt.Errorf("unexpected embedded skill archive path %q", header.Name)
	}
	if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
		return fmt.Errorf(
			"embedded skill archive path %q is not a regular file",
			header.Name,
		)
	}
	if _, exists := files[name]; exists {
		return fmt.Errorf("duplicate embedded skill archive path %q", header.Name)
	}
	files[name] = struct{}{}
	return nil
}

func containsDirectory(files map[string]struct{}, directory string) bool {
	prefix := directory + "/"
	for name := range files {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

type treeEntry struct {
	mode    fs.FileMode
	content []byte
}

func treesEqual(ctx context.Context, left, right string) (bool, error) {
	leftTree, err := readTree(ctx, left)
	if err != nil {
		return false, err
	}
	rightTree, err := readTree(ctx, right)
	if err != nil {
		return false, err
	}
	if len(leftTree) != len(rightTree) {
		return false, nil
	}
	for name, leftEntry := range leftTree {
		rightEntry, ok := rightTree[name]
		if !ok || leftEntry.mode != rightEntry.mode ||
			!bytes.Equal(leftEntry.content, rightEntry.content) {
			return false, nil
		}
	}
	return true, nil
}

func readTree(ctx context.Context, root string) (map[string]treeEntry, error) {
	entries := make(map[string]treeEntry)
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return fmt.Errorf("resolve relative path for %q: %w", name, err)
		}
		if relative == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect %q: %w", name, err)
		}
		treeEntry := treeEntry{mode: comparableMode(info.Mode())}
		if info.Mode().IsRegular() {
			treeEntry.content, err = os.ReadFile(name)
			if err != nil {
				return fmt.Errorf("read %q: %w", name, err)
			}
		}
		entries[filepath.ToSlash(relative)] = treeEntry
		return nil
	})
	return entries, err
}

func comparableMode(mode fs.FileMode) fs.FileMode {
	return mode.Type() | mode.Perm()
}

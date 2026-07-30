// Command skillarchive packages the Cardamom runtime skill for release builds.
package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
)

var runtimeEntries = []string{"SKILL.md", "references", "scripts"}

func main() {
	source := flag.String(
		"source",
		"plugins/cardamom/skills/cardamom",
		"runtime skill source directory",
	)
	output := flag.String(
		"output",
		"internal/skill/cardamom.tar.gz",
		"generated archive path",
	)
	flag.Parse()
	if err := run(*source, *output); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(source, output string) error {
	if err := validateSource(source); err != nil {
		return err
	}
	outputDirectory := filepath.Dir(output)
	file, err := os.CreateTemp(outputDirectory, ".cardamom-skill-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create archive beside %q: %w", output, err)
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()

	err = writeArchive(file, os.DirFS(source))
	err = errors.Join(err, file.Close())
	if err != nil {
		return fmt.Errorf("write %q: %w", output, err)
	}
	if err := os.Rename(temporary, output); err != nil {
		return fmt.Errorf("publish %q: %w", output, err)
	}
	return nil
}

func validateSource(source string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read skill source %q: %w", source, err)
	}
	allowed := append([]string(nil), runtimeEntries...)
	allowed = append(allowed, "tests")
	for _, entry := range entries {
		if !slices.Contains(allowed, entry.Name()) {
			return fmt.Errorf("unexpected skill source entry %q", entry.Name())
		}
	}
	for _, name := range runtimeEntries {
		info, err := os.Stat(filepath.Join(source, name))
		if err != nil {
			return fmt.Errorf("inspect required skill source %q: %w", name, err)
		}
		if name == "SKILL.md" && !info.Mode().IsRegular() {
			return fmt.Errorf("required skill source %q is not a regular file", name)
		}
		if name != "SKILL.md" && !info.IsDir() {
			return fmt.Errorf("required skill source %q is not a directory", name)
		}
	}
	return nil
}

func writeArchive(destination io.Writer, source fs.FS) error {
	gzipWriter := gzip.NewWriter(destination)
	gzipWriter.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)

	var archiveErr error
	for _, root := range runtimeEntries {
		archiveErr = fs.WalkDir(
			source,
			root,
			func(name string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				info, err := entry.Info()
				if err != nil {
					return fmt.Errorf("inspect %q: %w", name, err)
				}
				if !info.Mode().IsRegular() {
					return fmt.Errorf("skill source path %q is not a regular file", name)
				}
				file, err := source.Open(name)
				if err != nil {
					return fmt.Errorf("open %q: %w", name, err)
				}
				mode := int64(0o644)
				if info.Mode().Perm()&0o111 != 0 {
					mode = 0o755
				}
				header := &tar.Header{
					Name:   path.Clean(filepath.ToSlash(name)),
					Mode:   mode,
					Size:   info.Size(),
					Format: tar.FormatUSTAR,
				}
				if err := tarWriter.WriteHeader(header); err != nil {
					return errors.Join(
						fmt.Errorf("write header for %q: %w", name, err),
						file.Close(),
					)
				}
				_, copyErr := io.Copy(tarWriter, file)
				closeErr := file.Close()
				if err := errors.Join(copyErr, closeErr); err != nil {
					return fmt.Errorf("write %q: %w", name, err)
				}
				return nil
			},
		)
		if archiveErr != nil {
			break
		}
	}
	return errors.Join(archiveErr, tarWriter.Close(), gzipWriter.Close())
}

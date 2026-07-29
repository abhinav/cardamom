// Command webarchive packages generated browser assets for release builds.
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
)

func main() {
	source := flag.String("source", "web/dist", "Vite output directory")
	output := flag.String("output", "internal/web/server/static.tar.gz", "generated archive path")
	flag.Parse()
	if err := run(*source, *output); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(source, output string) error {
	file, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("create %q: %w", output, err)
	}
	err = writeArchive(file, os.DirFS(source))
	err = errors.Join(err, file.Close())
	if err != nil {
		return fmt.Errorf("write %q: %w", output, err)
	}
	return nil
}

// writeArchive emits files in fs.WalkDir order with normalized metadata so the
// same Vite output produces the same gzip and tar bytes on every build host.
func writeArchive(destination io.Writer, source fs.FS) error {
	gzipWriter := gzip.NewWriter(destination)
	gzipWriter.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)

	err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, err error) error {
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
			return fmt.Errorf("archive path %q is not a regular file", name)
		}
		file, err := source.Open(name)
		if err != nil {
			return fmt.Errorf("open %q: %w", name, err)
		}
		header := &tar.Header{
			Name:   name,
			Mode:   0o644,
			Size:   info.Size(),
			Format: tar.FormatUSTAR,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return errors.Join(fmt.Errorf("write header for %q: %w", name, err), file.Close())
		}
		_, copyErr := io.Copy(tarWriter, file)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return fmt.Errorf("write %q: %w", name, err)
		}
		return nil
	})
	err = errors.Join(err, tarWriter.Close(), gzipWriter.Close())
	return err
}

// Package server owns the local browser application's HTTP and process
// lifetime, including embedded release assets and live Vite development.
package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// embeddedApplicationArchive holds the generated release payload.
// It is empty when the current build does not contain browser assets.
var embeddedApplicationArchive []byte

// newApplicationHandler validates the release payload once and combines its
// immutable files with the generated Connect route prefix.
func newApplicationHandler(
	archive []byte,
	connectPath string,
	connectHandler http.Handler,
	attachmentContentPattern string,
	attachmentContentHandler http.Handler,
) (http.Handler, error) {
	if err := validateBackendHandlers(
		connectPath,
		connectHandler,
		attachmentContentPattern,
		attachmentContentHandler,
	); err != nil {
		return nil, err
	}

	files, err := readArchive(archive)
	if err != nil {
		return nil, err
	}
	index, ok := files["index.html"]
	if !ok {
		return nil, errors.New("web archive does not contain index.html")
	}

	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		content, ok := files[name]
		if ok {
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
			return
		}
		if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	})
	return composeApplicationHandler(
		connectPath,
		connectHandler,
		attachmentContentPattern,
		attachmentContentHandler,
		fallback,
	), nil
}

func validateBackendHandlers(
	connectPath string,
	connectHandler http.Handler,
	attachmentContentPattern string,
	attachmentContentHandler http.Handler,
) error {
	if connectHandler == nil {
		return errors.New("connect handler is required")
	}
	if !strings.HasPrefix(connectPath, "/") || connectPath == "/" {
		return fmt.Errorf("connect handler path %q must be an absolute non-root prefix", connectPath)
	}
	if attachmentContentHandler == nil {
		return errors.New("attachment content handler is required")
	}
	if !strings.HasPrefix(attachmentContentPattern, "/") ||
		attachmentContentPattern == "/" {
		return fmt.Errorf(
			"attachment content handler pattern %q must be an absolute non-root pattern",
			attachmentContentPattern,
		)
	}
	return nil
}

func composeApplicationHandler(
	connectPath string,
	connectHandler http.Handler,
	attachmentContentPattern string,
	attachmentContentHandler http.Handler,
	fallback http.Handler,
) http.Handler {
	routes := http.NewServeMux()
	routes.Handle(attachmentContentPattern, attachmentContentHandler)
	routes.Handle("/", fallback)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, connectPath) {
			connectHandler.ServeHTTP(w, r)
			return
		}
		routes.ServeHTTP(w, r)
	})
}

// readArchive converts the generated payload into immutable path content while
// rejecting names or entry kinds that could escape a filesystem extraction.
func readArchive(archive []byte) (map[string][]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open web archive: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()

	files := make(map[string][]byte)
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read web archive: %w", err)
		}

		name := strings.TrimSuffix(header.Name, "/")
		if name == "" || strings.ContainsRune(name, '\\') ||
			path.Clean(name) != name || !fs.ValidPath(name) {
			return nil, fmt.Errorf("invalid archive path %q", header.Name)
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
			return nil, fmt.Errorf("archive path %q is not a regular file", header.Name)
		}
		if _, exists := files[name]; exists {
			return nil, fmt.Errorf("duplicate archive path %q", header.Name)
		}
		content, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("read archive path %q: %w", header.Name, err)
		}
		files[name] = content
	}
	if _, err := io.Copy(io.Discard, gzipReader); err != nil {
		return nil, fmt.Errorf("finish web archive: %w", err)
	}
	if err := gzipReader.Close(); err != nil {
		return nil, fmt.Errorf("close web archive: %w", err)
	}
	return files, nil
}

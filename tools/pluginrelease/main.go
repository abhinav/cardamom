// Package main synchronizes version metadata for the Cardamom plugin package.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
)

var manifestPaths = []string{
	"plugins/cardamom/.claude-plugin/plugin.json",
	"plugins/cardamom/.codex-plugin/plugin.json",
}

func main() {
	if err := run(".", os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pluginrelease:", err)
		os.Exit(1)
	}
}

func run(root string, stdout io.Writer, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: pluginrelease (check [v<version>] | materialize v<version>)")
	}

	switch args[0] {
	case "check":
		if len(args) > 2 {
			return errors.New("usage: pluginrelease check [v<version>]")
		}
		var expected string
		if len(args) == 2 {
			var err error
			expected, err = normalizeReleaseVersion(args[1])
			if err != nil {
				return err
			}
		}
		version, err := checkMetadata(root, expected)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(
			stdout,
			"plugin version %s is consistent\n",
			version,
		)
		if err != nil {
			return fmt.Errorf("write check result: %w", err)
		}
		return nil

	case "materialize":
		if len(args) != 2 {
			return errors.New("usage: pluginrelease materialize v<version>")
		}
		version, err := normalizeReleaseVersion(args[1])
		if err != nil {
			return err
		}
		if err := materialize(root, version); err != nil {
			return err
		}
		_, err = fmt.Fprintf(
			stdout,
			"materialized plugin version %s\n",
			version,
		)
		if err != nil {
			return fmt.Errorf("write materialize result: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("unknown operation %q", args[0])
	}
}

func normalizeReleaseVersion(value string) (string, error) {
	if !strings.HasPrefix(value, "v") {
		return "", fmt.Errorf("release version %q must start with \"v\"", value)
	}
	if !semver.IsValid(value) {
		return "", fmt.Errorf("release version %q is not valid SemVer", value)
	}
	return strings.TrimPrefix(value, "v"), nil
}

type manifest struct {
	path   string
	mode   fs.FileMode
	fields map[string]json.RawMessage
}

func loadManifests(root string) ([]manifest, error) {
	manifests := make([]manifest, 0, len(manifestPaths))
	for _, relativePath := range manifestPaths {
		path := filepath.Join(root, relativePath)
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", relativePath, err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			return nil, fmt.Errorf("parse %q: %w", relativePath, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %q: %w", relativePath, err)
		}
		manifests = append(manifests, manifest{
			path:   path,
			mode:   info.Mode(),
			fields: fields,
		})
	}
	return manifests, nil
}

func manifestVersion(value manifest) (string, error) {
	rawVersion, ok := value.fields["version"]
	if !ok {
		return "", fmt.Errorf("%q has no version field", value.path)
	}
	var version string
	if err := json.Unmarshal(rawVersion, &version); err != nil {
		return "", fmt.Errorf("parse version in %q: %w", value.path, err)
	}
	if !semver.IsValid("v" + version) {
		return "", fmt.Errorf("%q has invalid plugin version %q", value.path, version)
	}
	return version, nil
}

func readVersionFile(root string) (string, error) {
	path := filepath.Join(root, "plugins/cardamom/VERSION")
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", path, err)
	}
	version := strings.TrimSpace(string(body))
	if !semver.IsValid("v" + version) {
		return "", fmt.Errorf("%q has invalid plugin version %q", path, version)
	}
	return version, nil
}

func checkMetadata(root string, expected string) (string, error) {
	version, err := readVersionFile(root)
	if err != nil {
		return "", err
	}
	if expected != "" && version != expected {
		return "", fmt.Errorf(
			"expected version %q, but VERSION contains %q",
			expected,
			version,
		)
	}

	manifests, err := loadManifests(root)
	if err != nil {
		return "", err
	}
	for _, manifest := range manifests {
		manifestVersion, err := manifestVersion(manifest)
		if err != nil {
			return "", err
		}
		if manifestVersion != version {
			return "", fmt.Errorf(
				"%q has version %q, but VERSION contains %q",
				manifest.path,
				manifestVersion,
				version,
			)
		}
	}
	return version, nil
}

func materialize(root string, version string) error {
	manifests, err := loadManifests(root)
	if err != nil {
		return err
	}
	if _, err := readVersionFile(root); err != nil {
		return err
	}
	for _, manifest := range manifests {
		if _, err := manifestVersion(manifest); err != nil {
			return err
		}
	}

	versionJSON, err := json.Marshal(version)
	if err != nil {
		return fmt.Errorf("encode version: %w", err)
	}
	for _, manifest := range manifests {
		manifest.fields["version"] = versionJSON
		body, err := json.MarshalIndent(manifest.fields, "", "  ")
		if err != nil {
			return fmt.Errorf("encode %q: %w", manifest.path, err)
		}
		body = append(body, '\n')
		if err := writeIfChanged(
			manifest.path,
			body,
			manifest.mode.Perm(),
		); err != nil {
			return err
		}
	}

	versionPath := filepath.Join(root, "plugins/cardamom/VERSION")
	info, err := os.Stat(versionPath)
	if err != nil {
		return fmt.Errorf("stat %q: %w", versionPath, err)
	}
	return writeIfChanged(versionPath, []byte(version+"\n"), info.Mode().Perm())
}

func writeIfChanged(path string, body []byte, mode fs.FileMode) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}
	if bytes.Equal(current, body) {
		return nil
	}

	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temporary file for %q: %w", path, err)
	}
	tempPath := file.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return fmt.Errorf("set permissions for %q: %w", path, err)
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temporary file for %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file for %q: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace %q: %w", path, err)
	}
	return nil
}

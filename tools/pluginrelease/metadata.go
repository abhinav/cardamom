package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
)

const versionFilePath = "plugins/cardamom/VERSION"

var manifestPaths = []string{
	"plugins/cardamom/.claude-plugin/plugin.json",
	"plugins/cardamom/.codex-plugin/plugin.json",
}

// pluginVersion is a validated SemVer string without a leading "v".
type pluginVersion string

func parseReleaseVersion(value string) (pluginVersion, error) {
	normalized := strings.TrimPrefix(value, "v")
	if !semver.IsValid("v" + normalized) {
		return "", fmt.Errorf("release version %q is not valid SemVer", value)
	}
	return pluginVersion(normalized), nil
}

// pluginMetadata owns the version metadata at the fixed paths under root.
type pluginMetadata struct {
	root string
}

func (m *pluginMetadata) check(
	expected *pluginVersion,
) (pluginVersion, error) {
	state, err := m.load()
	if err != nil {
		return "", err
	}
	if expected != nil && state.version != *expected {
		return "", fmt.Errorf(
			"expected version %q, but %s contains %q",
			*expected,
			versionFilePath,
			state.version,
		)
	}

	for _, manifest := range state.manifests {
		if manifest.version != state.version {
			return "", fmt.Errorf(
				"%q has version %q, but %s contains %q",
				manifest.relativePath,
				manifest.version,
				versionFilePath,
				state.version,
			)
		}
	}
	return state.version, nil
}

func (m *pluginMetadata) materialize(version pluginVersion) error {
	state, err := m.load()
	if err != nil {
		return err
	}

	updates := make([]fileUpdate, 0, len(state.manifests)+1)
	for i := range state.manifests {
		manifest := &state.manifests[i]
		body, err := manifest.withVersion(version)
		if err != nil {
			return err
		}
		updates = append(updates, fileUpdate{
			relativePath: manifest.relativePath,
			path:         manifest.path,
			mode:         manifest.mode,
			body:         body,
		})
	}
	updates = append(updates, fileUpdate{
		relativePath: versionFilePath,
		path:         state.versionPath,
		mode:         state.versionMode,
		body:         []byte(version + "\n"),
	})

	for _, update := range updates {
		if err := update.apply(); err != nil {
			return err
		}
	}
	return nil
}

// metadataState is one fully loaded and validated repository snapshot.
type metadataState struct {
	version     pluginVersion
	versionPath string
	versionMode fs.FileMode
	manifests   []manifest
}

func (m *pluginMetadata) load() (metadataState, error) {
	versionPath := filepath.Join(m.root, versionFilePath)
	versionBody, err := os.ReadFile(versionPath)
	if err != nil {
		return metadataState{}, fmt.Errorf("read %q: %w", versionFilePath, err)
	}
	version, err := parsePersistedVersion(
		versionFilePath,
		strings.TrimSpace(string(versionBody)),
	)
	if err != nil {
		return metadataState{}, err
	}
	versionInfo, err := os.Stat(versionPath)
	if err != nil {
		return metadataState{}, fmt.Errorf("stat %q: %w", versionFilePath, err)
	}

	state := metadataState{
		version:     version,
		versionPath: versionPath,
		versionMode: versionInfo.Mode().Perm(),
		manifests:   make([]manifest, 0, len(manifestPaths)),
	}
	for _, relativePath := range manifestPaths {
		path := filepath.Join(m.root, relativePath)
		body, err := os.ReadFile(path)
		if err != nil {
			return metadataState{}, fmt.Errorf("read %q: %w", relativePath, err)
		}

		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			return metadataState{}, fmt.Errorf("parse %q: %w", relativePath, err)
		}
		rawVersion, ok := fields["version"]
		if !ok {
			return metadataState{}, fmt.Errorf(
				"%q has no version field",
				relativePath,
			)
		}
		var versionValue string
		if err := json.Unmarshal(rawVersion, &versionValue); err != nil {
			return metadataState{}, fmt.Errorf(
				"parse version in %q: %w",
				relativePath,
				err,
			)
		}
		manifestVersion, err := parsePersistedVersion(
			relativePath,
			versionValue,
		)
		if err != nil {
			return metadataState{}, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return metadataState{}, fmt.Errorf("stat %q: %w", relativePath, err)
		}
		state.manifests = append(state.manifests, manifest{
			relativePath: relativePath,
			path:         path,
			mode:         info.Mode().Perm(),
			fields:       fields,
			version:      manifestVersion,
		})
	}
	return state, nil
}

func parsePersistedVersion(
	source string,
	value string,
) (pluginVersion, error) {
	if !semver.IsValid("v" + value) {
		return "", fmt.Errorf(
			"%q has invalid plugin version %q",
			source,
			value,
		)
	}
	return pluginVersion(value), nil
}

// manifest is one plugin host's versioned JSON manifest.
//
// The fields map retains every top-level member as raw JSON so materialization
// can change only the version's semantic value before canonical re-encoding.
type manifest struct {
	// relativePath identifies the manifest in operator-facing diagnostics.
	relativePath string
	// path is the filesystem location used for reads and atomic replacement.
	path string
	// mode is the permission mode preserved during atomic replacement.
	mode fs.FileMode
	// fields contains every top-level manifest member, including unknown ones.
	fields map[string]json.RawMessage
	// version is the validated version decoded from fields.
	version pluginVersion
}

func (m *manifest) withVersion(version pluginVersion) ([]byte, error) {
	versionJSON, err := json.Marshal(version)
	if err != nil {
		return nil, fmt.Errorf("encode version for %q: %w", m.relativePath, err)
	}
	m.fields["version"] = versionJSON

	body, err := json.MarshalIndent(m.fields, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode %q: %w", m.relativePath, err)
	}
	return append(body, '\n'), nil
}

// fileUpdate is one complete file replacement prepared by materialize.
type fileUpdate struct {
	// relativePath identifies the file in operator-facing diagnostics.
	relativePath string
	// path is the filesystem location replaced by apply.
	path string
	// mode is the permission mode assigned to a changed file.
	mode fs.FileMode
	// body is the complete replacement content.
	body []byte
}

func (u *fileUpdate) apply() error {
	current, err := os.ReadFile(u.path)
	if err != nil {
		return fmt.Errorf("read %q: %w", u.relativePath, err)
	}
	if bytes.Equal(current, u.body) {
		return nil
	}

	file, err := os.CreateTemp(filepath.Dir(u.path), "."+filepath.Base(u.path)+".*")
	if err != nil {
		return fmt.Errorf(
			"create temporary file for %q: %w",
			u.relativePath,
			err,
		)
	}
	tempPath := file.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if err := file.Chmod(u.mode); err != nil {
		_ = file.Close()
		return fmt.Errorf("set permissions for %q: %w", u.relativePath, err)
	}
	if _, err := file.Write(u.body); err != nil {
		_ = file.Close()
		return fmt.Errorf(
			"write temporary file for %q: %w",
			u.relativePath,
			err,
		)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf(
			"close temporary file for %q: %w",
			u.relativePath,
			err,
		)
	}
	if err := os.Rename(tempPath, u.path); err != nil {
		return fmt.Errorf("replace %q: %w", u.relativePath, err)
	}
	return nil
}

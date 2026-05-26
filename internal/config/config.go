// Package config loads and validates the per-project `config.yaml` that
// lives at <dir>/config.yaml (default <dir> = ".clu").
//
// The file is human-edited and committed to git alongside workflow
// templates. Local-only state (the SQLite database, WAL files) lives in
// the same directory but is gitignored — see the snippet that
// `clu init` writes alongside the config.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Config is the parsed shape of <dir>/config.yaml. New top-level fields
// MUST round-trip through yaml.v3 with strict unmarshalling (unknown
// keys are an error — see Load) so typos surface immediately.
type Config struct {
	IDPrefix string `yaml:"id_prefix"`
}

// Default returns a Config with the safe defaults Load uses when a key
// is missing. Callers wanting Init-time defaults should start here.
func Default() Config {
	return Config{
		IDPrefix: "clu-",
	}
}

// idPrefixRe enforces the allowed shape: lowercase + digits + dashes,
// must END in '-' so the boundary between prefix and hex suffix is
// unambiguous when reading IDs back. Maximum 16 chars total so IDs stay
// readable.
var idPrefixRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*-$`)

// Validate enforces the rules described on each field. Returned errors
// are user-facing and meant to land directly in CLI stderr.
func (c Config) Validate() error {
	if c.IDPrefix == "" {
		return errors.New("id_prefix: required")
	}
	if len(c.IDPrefix) > 16 {
		return fmt.Errorf("id_prefix %q: too long (max 16 chars)", c.IDPrefix)
	}
	if !idPrefixRe.MatchString(c.IDPrefix) {
		return fmt.Errorf("id_prefix %q: must be lowercase a-z, 0-9, dashes; must end with `-`", c.IDPrefix)
	}
	return nil
}

// Path returns the conventional config path inside a project directory.
func Path(dir string) string {
	return filepath.Join(dir, "config.yaml")
}

// Load reads and parses <dir>/config.yaml. Missing file → returns Default
// (no error); present file → strict-parse (unknown keys = error) and
// validate. Errors include the file path for easy navigation.
func Load(dir string) (Config, error) {
	out := Default()
	path := Path(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, fmt.Errorf("%s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // typos are loud, not silent
	var parsed Config
	if err := dec.Decode(&parsed); err != nil {
		return out, fmt.Errorf("%s: %w", path, err)
	}
	if parsed.IDPrefix == "" {
		parsed.IDPrefix = out.IDPrefix
	}
	if err := parsed.Validate(); err != nil {
		return out, fmt.Errorf("%s: %w", path, err)
	}
	return parsed, nil
}

// Write serialises c to <dir>/config.yaml with leading comments
// explaining each knob. Intended for `clu init` on a fresh project.
// Refuses to overwrite an existing file — callers handle the "already
// exists" case as a no-op or warning.
func Write(dir string, c Config) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := Path(dir)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	// Build the YAML by hand so we can include comments. The content
	// is small enough that hand-rolling beats yaml.v3 Marshal+annotate.
	body := fmt.Sprintf(`# clu project configuration.
#
# This file is committed to git alongside workflow templates. Local-only
# state (the SQLite database, WAL files) lives in this same directory
# but should be gitignored — see .gitignore guidance in the README.

# Prefix for newly-allocated issue IDs. Existing IDs are unaffected if
# you change this. Must be lowercase a-z, digits, dashes, ending with
# "-". Max 16 chars. Example: "acme-" → acme-a3f8.
id_prefix: %s
`, c.IDPrefix)
	return os.WriteFile(path, []byte(body), 0o644)
}


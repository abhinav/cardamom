// Command migration creates timestamped Cardamom migration templates.
package main

import (
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type migrationKind int

const (
	migrationKindUnknown migrationKind = iota
	migrationKindSQL
	migrationKindGo
)

var (
	migrationNamePattern       = regexp.MustCompile(`^[A-Za-z0-9]+(?:[ _-]+[A-Za-z0-9]+)*$`)
	migrationNameSeparators    = regexp.MustCompile(`[ _-]+`)
	goMigrationFilenamePattern = regexp.MustCompile(
		`^migration_([0-9]{14})_[a-z0-9_]+[.]go$`,
	)
)

func main() {
	path, err := run(os.Args[1:], time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(path)
}

func run(args []string, now time.Time) (string, error) {
	if len(args) != 2 {
		return "", errors.New("usage: migration <sql|go> <name>")
	}
	kind, err := parseMigrationKind(args[0])
	if err != nil {
		return "", err
	}
	return generateMigration(".", kind, args[1], now)
}

func parseMigrationKind(raw string) (migrationKind, error) {
	switch raw {
	case "sql":
		return migrationKindSQL, nil
	case "go":
		return migrationKindGo, nil
	default:
		return migrationKindUnknown, fmt.Errorf("invalid migration kind %q", raw)
	}
}

func generateMigration(
	root string,
	kind migrationKind,
	rawName string,
	now time.Time,
) (string, error) {
	name, err := normalizeMigrationName(rawName)
	if err != nil {
		return "", err
	}

	version := now.UTC().Format("20060102150405")
	storeDir := filepath.Join(root, "internal", "repository", "store")
	collision, err := findVersionCollision(storeDir, version)
	if err != nil {
		return "", err
	}
	if collision != "" {
		return "", fmt.Errorf(
			"migration version %s already exists as %q",
			version,
			collision,
		)
	}

	path, body, err := migrationTemplate(storeDir, kind, version, name)
	if err != nil {
		return "", err
	}
	var registryPath string
	var previousRegistryBody []byte
	var registryBody []byte
	if kind == migrationKindGo {
		registryPath = filepath.Join(storeDir, "go_migrations.go")
		previousRegistryBody, err = os.ReadFile(registryPath)
		if err != nil {
			return "", fmt.Errorf("read Go migration registry: %w", err)
		}
		versions, err := goMigrationVersions(storeDir)
		if err != nil {
			return "", err
		}
		registryBody, err = goMigrationRegistry(append(versions, version))
		if err != nil {
			return "", err
		}
	}
	// Publish the registry first so interruption causes a compile failure
	// instead of leaving a silently unregistered migration callback.
	if registryPath != "" {
		if err := replaceFile(registryPath, registryBody); err != nil {
			return "", err
		}
	}
	if err := writeNewFile(path, body); err != nil {
		if registryPath != "" {
			if restoreErr := replaceFile(
				registryPath,
				previousRegistryBody,
			); restoreErr != nil {
				err = errors.Join(
					err,
					fmt.Errorf("restore Go migration registry: %w", restoreErr),
				)
			}
		}
		return "", err
	}
	relativePath, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("resolve generated migration path: %w", err)
	}
	return relativePath, nil
}

func normalizeMigrationName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errors.New("migration name is required")
	}
	if !migrationNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid migration name %q", raw)
	}
	name = migrationNameSeparators.ReplaceAllString(name, "_")
	return strings.ToLower(name), nil
}

func findVersionCollision(storeDir string, version string) (string, error) {
	sqlEntries, err := os.ReadDir(filepath.Join(storeDir, "migrations"))
	if err != nil {
		return "", fmt.Errorf("read SQL migrations: %w", err)
	}
	sqlPrefix := version + "_"
	for _, entry := range sqlEntries {
		if !entry.IsDir() &&
			strings.HasPrefix(entry.Name(), sqlPrefix) &&
			filepath.Ext(entry.Name()) == ".sql" {
			return entry.Name(), nil
		}
	}

	goEntries, err := os.ReadDir(storeDir)
	if err != nil {
		return "", fmt.Errorf("read Go migrations: %w", err)
	}
	goPrefix := "migration_" + version + "_"
	for _, entry := range goEntries {
		if !entry.IsDir() &&
			strings.HasPrefix(entry.Name(), goPrefix) &&
			filepath.Ext(entry.Name()) == ".go" {
			return entry.Name(), nil
		}
	}
	return "", nil
}

func migrationTemplate(
	storeDir string,
	kind migrationKind,
	version string,
	name string,
) (string, []byte, error) {
	switch kind {
	case migrationKindSQL:
		path := filepath.Join(
			storeDir,
			"migrations",
			version+"_"+name+".sql",
		)
		return path, []byte(`-- +goose Up

-- Replace this comment with the forward schema migration.
`), nil
	case migrationKindGo:
		path := filepath.Join(
			storeDir,
			"migration_"+version+"_"+name+".go",
		)
		body, err := format.Source(fmt.Appendf(nil, `package store

import (
	"context"
	"database/sql"
	"errors"
)

func migrate%s(context.Context, *sql.Tx) error {
	return errors.New("implement data migration")
}
`, version))
		if err != nil {
			return "", nil, fmt.Errorf("format Go migration: %w", err)
		}
		return path, body, nil
	default:
		return "", nil, fmt.Errorf("invalid migration kind %d", kind)
	}
}

func goMigrationVersions(storeDir string) ([]string, error) {
	entries, err := os.ReadDir(storeDir)
	if err != nil {
		return nil, fmt.Errorf("read Go migrations: %w", err)
	}
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := goMigrationFilenamePattern.FindStringSubmatch(entry.Name())
		if match != nil {
			versions = append(versions, match[1])
		}
	}
	return versions, nil
}

func goMigrationRegistry(versions []string) ([]byte, error) {
	sort.Strings(versions)

	body := []byte(`// Code generated by the migration authoring workflow. DO NOT EDIT.

package store

import "github.com/pressly/goose/v3"

// goMigrations is the complete data-only migration registry used during
// package initialization. Schema changes remain in SQL so the migration
// directory stays authoritative for sqlc.
var goMigrations = []*goose.Migration{
`)
	for _, registeredVersion := range versions {
		body = fmt.Appendf(body, `	goose.NewGoMigration(
		%s,
		&goose.GoFunc{RunTx: migrate%s},
		nil,
	),
`, registeredVersion, registeredVersion)
	}
	body = append(body, '}', '\n')
	formatted, err := format.Source(body)
	if err != nil {
		return nil, fmt.Errorf("format Go migration registry: %w", err)
	}
	return formatted, nil
}

func writeNewFile(path string, body []byte) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create migration %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close migration %q: %w", path, closeErr))
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(body); err != nil {
		return fmt.Errorf("write migration %q: %w", path, err)
	}
	return nil
}

func replaceFile(path string, body []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect Go migration registry: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".go-migrations-*")
	if err != nil {
		return fmt.Errorf("create temporary Go migration registry: %w", err)
	}
	tempPath := file.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		_ = file.Close()
		return fmt.Errorf("set Go migration registry permissions: %w", err)
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return fmt.Errorf("write Go migration registry: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close Go migration registry: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace Go migration registry: %w", err)
	}
	return nil
}

package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
)

const settingsFilename = "config.yaml"

// settingsDocument is the optional versioned YAML representation at the
// physical-store boundary. Domain code receives validated typed overrides.
type settingsDocument struct {
	Version    *int                        `yaml:"version,omitempty"`
	Issue      *settingsIssueDocument      `yaml:"issue,omitempty"`
	Attachment *settingsAttachmentDocument `yaml:"attachment,omitempty"`
	Board      *settingsBoardDocument      `yaml:"board,omitempty"`
}

type settingsIssueDocument struct {
	ID      *settingsIssueIDDocument `yaml:"id,omitempty"`
	Summary *settingsSummaryDocument `yaml:"summary,omitempty"`
}

type settingsIssueIDDocument struct {
	Prefix   *string `yaml:"prefix,omitempty"`
	Strategy *string `yaml:"strategy,omitempty"`
}

type settingsSummaryDocument struct {
	MaxBytes *uint64 `yaml:"max_bytes,omitempty"`
}

type settingsAttachmentDocument struct {
	MaxBytes *uint64 `yaml:"max_bytes,omitempty"`
}

type settingsBoardDocument struct {
	Pins *settingsPinsDocument `yaml:"pins,omitempty"`
}

type settingsPinsDocument struct {
	MaxCount *uint64 `yaml:"max_count,omitempty"`
}

// settingsStore adapts the optional YAML file to configuration.Store.
type settingsStore struct{ directory string }

func (s settingsStore) ReadStoreConfiguration(
	context.Context,
) (configuration.Overrides, error) {
	return readSettings(s.directory)
}

func (s settingsStore) UpdateStoreConfiguration(
	_ context.Context,
	patch configuration.Patch,
) (configuration.Overrides, error) {
	current, err := readSettings(s.directory)
	if err != nil {
		return configuration.Overrides{}, err
	}
	updated := patch.Apply(current)
	path := settingsPath(s.directory)
	if updated.Empty() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return configuration.Overrides{}, fmt.Errorf("remove %q: %w", path, err)
		}
		return updated, nil
	}
	if err := writeSettings(path, updated); err != nil {
		return configuration.Overrides{}, err
	}
	return updated, nil
}

// writeSettings atomically writes active nested YAML or a fully commented
// initialization template when overrides is empty.
func writeSettings(path string, overrides configuration.Overrides) (err error) {
	if err := overrides.Validate(); err != nil {
		return err
	}
	body := renderSettings(overrides)
	temporary, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temporary configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, temporary.Close())
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("set temporary configuration permissions: %w", err)
	}
	if _, err := temporary.Write(body); err != nil {
		return fmt.Errorf("write temporary configuration: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary configuration: %w", err)
	}
	closed = true
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace %q: %w", path, err)
	}
	return nil
}

func renderSettings(overrides configuration.Overrides) []byte {
	var body bytes.Buffer
	_, _ = fmt.Fprintln(&body, "# config.yaml contains local overrides for this physical Cardamom store.")
	_, _ = fmt.Fprintln(&body, "# Missing or commented values inherit built-in, project, or board values.")
	_, _ = fmt.Fprintln(&body, "# Use card config set --scope store KEY VALUE to set one value.")
	_, _ = fmt.Fprintln(&body, "# Use card config unset --scope store KEY to restore inheritance.")
	_, _ = fmt.Fprintln(&body, "#")
	_, _ = fmt.Fprintln(&body, "# Built-in defaults are shown below.")
	comment := ""
	if overrides.Empty() {
		comment = "# "
	}
	_, _ = fmt.Fprintf(&body, "%sversion: 1\n", comment)

	issueActive := overrides.Issue.ID.Prefix != nil ||
		overrides.Issue.ID.Strategy != nil ||
		overrides.Issue.Summary.MaxBytes != nil
	issueComment := "# "
	if issueActive {
		issueComment = ""
	}
	_, _ = fmt.Fprintf(&body, "%sissue:\n", issueComment)

	idActive := overrides.Issue.ID.Prefix != nil || overrides.Issue.ID.Strategy != nil
	idComment := "# "
	if idActive {
		idComment = ""
	}
	_, _ = fmt.Fprintf(&body, "%s  id:\n", idComment)
	if overrides.Issue.ID.Prefix == nil {
		_, _ = fmt.Fprintln(&body, "#     prefix: cm-")
	} else {
		_, _ = fmt.Fprintf(&body, "    prefix: %s\n", overrides.Issue.ID.Prefix.String())
	}
	if overrides.Issue.ID.Strategy == nil {
		_, _ = fmt.Fprintln(&body, "#     strategy: random")
	} else {
		_, _ = fmt.Fprintf(&body, "    strategy: %s\n", overrides.Issue.ID.Strategy.String())
	}

	summaryComment := "# "
	if overrides.Issue.Summary.MaxBytes != nil {
		summaryComment = ""
	}
	_, _ = fmt.Fprintf(&body, "%s  summary:\n", summaryComment)
	if overrides.Issue.Summary.MaxBytes == nil {
		_, _ = fmt.Fprintf(&body, "#     max_bytes: %d\n", configuration.DefaultSummaryMaxBytes)
	} else {
		_, _ = fmt.Fprintf(
			&body,
			"    max_bytes: %d\n",
			overrides.Issue.Summary.MaxBytes.Uint64(),
		)
	}

	attachmentComment := "# "
	if overrides.Attachment.MaxBytes != nil {
		attachmentComment = ""
	}
	_, _ = fmt.Fprintf(&body, "%sattachment:\n", attachmentComment)
	if overrides.Attachment.MaxBytes == nil {
		_, _ = fmt.Fprintf(&body, "#   max_bytes: %d\n", configuration.DefaultAttachmentMaxBytes)
	} else {
		_, _ = fmt.Fprintf(
			&body,
			"  max_bytes: %d\n",
			overrides.Attachment.MaxBytes.Uint64(),
		)
	}

	boardComment := "# "
	if overrides.Board.Pins.MaxCount != nil {
		boardComment = ""
	}
	_, _ = fmt.Fprintf(&body, "%sboard:\n", boardComment)
	_, _ = fmt.Fprintf(&body, "%s  pins:\n", boardComment)
	if overrides.Board.Pins.MaxCount == nil {
		_, _ = fmt.Fprintf(&body, "#     max_count: %d\n", configuration.DefaultPinMaxCount)
	} else {
		_, _ = fmt.Fprintf(
			&body,
			"    max_count: %d\n",
			overrides.Board.Pins.MaxCount.Uint64(),
		)
	}
	return body.Bytes()
}

func readSettings(directory string) (configuration.Overrides, error) {
	path := settingsPath(directory)
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return configuration.Overrides{}, nil
	}
	if err != nil {
		return configuration.Overrides{}, fmt.Errorf("read %q: %w", path, err)
	}
	var document settingsDocument
	if err := yaml.UnmarshalWithOptions(body, &document, yaml.Strict()); err != nil {
		return configuration.Overrides{}, fmt.Errorf("%s: %w", path, err)
	}
	overrides, err := document.overrides()
	if err != nil {
		return configuration.Overrides{}, fmt.Errorf("%s: %w", path, err)
	}
	return overrides, nil
}

func settingsPath(storeDirectory string) string {
	return filepath.Join(storeDirectory, settingsFilename)
}

func (d settingsDocument) overrides() (configuration.Overrides, error) {
	var overrides configuration.Overrides
	active := false
	if d.Issue != nil {
		if d.Issue.ID != nil {
			if d.Issue.ID.Prefix != nil {
				prefix, err := configuration.NewPrefix(*d.Issue.ID.Prefix)
				if err != nil {
					return overrides, err
				}
				overrides.Issue.ID.Prefix = &prefix
				active = true
			}
			if d.Issue.ID.Strategy != nil {
				strategy, err := configuration.NewIDStrategy(*d.Issue.ID.Strategy)
				if err != nil {
					return overrides, err
				}
				overrides.Issue.ID.Strategy = &strategy
				active = true
			}
		}
		if d.Issue.Summary != nil && d.Issue.Summary.MaxBytes != nil {
			limit, err := configuration.NewByteLimit(*d.Issue.Summary.MaxBytes)
			if err != nil {
				return overrides, fmt.Errorf("issue summary: %w", err)
			}
			overrides.Issue.Summary.MaxBytes = &limit
			active = true
		}
	}
	if d.Attachment != nil && d.Attachment.MaxBytes != nil {
		limit, err := configuration.NewByteLimit(*d.Attachment.MaxBytes)
		if err != nil {
			return overrides, fmt.Errorf("attachment: %w", err)
		}
		overrides.Attachment.MaxBytes = &limit
		active = true
	}
	if d.Board != nil && d.Board.Pins != nil && d.Board.Pins.MaxCount != nil {
		limit, err := board.NewPinLimit(*d.Board.Pins.MaxCount)
		if err != nil {
			return overrides, fmt.Errorf("board pins: %w", err)
		}
		overrides.Board.Pins.MaxCount = &limit
		active = true
	}
	if active && (d.Version == nil || *d.Version != 1) {
		return overrides, errors.New("version must be 1 when configuration values are active")
	}
	if d.Version != nil && *d.Version != 1 {
		return overrides, fmt.Errorf("unsupported configuration version %d", *d.Version)
	}
	return overrides, nil
}

func storeConfiguration(overrides configuration.Overrides) configuration.Configuration {
	defaults := configuration.Defaults()
	if overrides.Issue.ID.Prefix != nil {
		defaults.Issue.ID.Prefix = *overrides.Issue.ID.Prefix
	}
	if overrides.Issue.ID.Strategy != nil {
		defaults.Issue.ID.Strategy = *overrides.Issue.ID.Strategy
	}
	if overrides.Issue.Summary.MaxBytes != nil {
		defaults.Issue.Summary.MaxBytes = *overrides.Issue.Summary.MaxBytes
	}
	if overrides.Attachment.MaxBytes != nil {
		defaults.Attachment.MaxBytes = *overrides.Attachment.MaxBytes
	}
	if overrides.Board.Pins.MaxCount != nil {
		defaults.Board.Pins.MaxCount = *overrides.Board.Pins.MaxCount
	}
	return defaults
}

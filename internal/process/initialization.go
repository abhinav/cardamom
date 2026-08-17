package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/configuration/yamlstore"
	"go.abhg.dev/cardamom/internal/project"
	repositoryproject "go.abhg.dev/cardamom/internal/repository/project"
	"go.abhg.dev/cardamom/internal/storelocation"
)

type initializer struct{ config Config }

func provideInitializer(config *Config) cli.InitOperation {
	return &initializer{config: *config}
}

func (i *initializer) Initialize(
	ctx context.Context,
	request cli.InitRequest,
) (cli.InitResult, error) {
	displayDirectory, err := storelocation.InitTarget(request.Store, request.ProjectPath)
	if err != nil {
		return cli.InitResult{}, err
	}
	directory, err := absolutePath(displayDirectory, i.config.CWD)
	if err != nil {
		return cli.InitResult{}, err
	}
	projectDirectory := i.config.CWD
	if request.Store == "" && request.ProjectPath != "" {
		projectDirectory, err = absolutePath(request.ProjectPath, i.config.CWD)
		if err != nil {
			return cli.InitResult{}, err
		}
	}
	projectName := filepath.Base(filepath.Clean(projectDirectory))

	if err := os.MkdirAll(directory, 0o755); err != nil {
		return cli.InitResult{}, fmt.Errorf("create store directory %q: %w", directory, err)
	}
	settings, err := i.ensureSettings(
		ctx,
		directory,
		projectName,
		request.IDPrefix,
		request.ConfigMode,
	)
	if err != nil {
		return cli.InitResult{}, err
	}
	var boardName *string
	if !request.NoBoard {
		name := projectName
		if request.BoardName != nil {
			name = *request.BoardName
		}
		boardName = &name
	}
	storeInitializer := repositoryproject.NewInitializer(repositoryproject.Config{
		Clock: i.config.Clock, IDSource: i.config.ProjectIDs,
	})
	initialized, err := storeInitializer.InitializeStore(ctx, project.StoreInitializationRequest{
		Dir: directory, ProjectName: projectName, BoardName: boardName,
		FreshProjectIDPrefix:    prefixString(settings.prefix.FreshProject),
		RetainedProjectIDPrefix: prefixString(settings.prefix.RetainedProject),
	})
	if err != nil {
		return cli.InitResult{}, err
	}
	effective := resolveStoreConfiguration(settings.storeOverrides)
	if initialized.ProjectIDPrefix != nil {
		prefix, err := configuration.NewPrefix(*initialized.ProjectIDPrefix)
		if err != nil {
			return cli.InitResult{}, fmt.Errorf(
				"load initialized project prefix: %w",
				err,
			)
		}
		effective.Issue.ID.Prefix = prefix
	}

	result := cli.InitResult{
		Dir: displayDirectory, IDPrefix: effective.Issue.ID.Prefix.String(),
		IDStrategy: effective.Issue.ID.Strategy.String(), SchemaVersion: initialized.SchemaVersion,
		ConfigWritten: settings.configWritten, DatabaseWritten: initialized.DatabaseWritten,
		AlreadyInitialized: !initialized.DatabaseWritten,
	}
	if initialized.Namespace != nil {
		projectID := initialized.Namespace.Project.ID().String()
		result.ProjectCreated = true
		result.ProjectID = &projectID
		projectName := initialized.Namespace.Project.Name()
		result.ProjectName = &projectName
		if initialized.Namespace.Board != nil {
			boardID := initialized.Namespace.Board.ID().String()
			name := initialized.Namespace.Board.Name()
			result.BoardCreated = true
			result.BoardID = &boardID
			result.BoardName = &name
		}
	}
	if !i.config.DisableGitIgnore {
		result.IgnoreOutcome = configureGitIgnore(projectDirectory, directory)
	}
	return result, nil
}

// initializationSettings carries initialization configuration across store-file
// and project-database publication.
type initializationSettings struct {
	// storeOverrides contains the active physical-store overrides.
	storeOverrides configuration.Overrides

	// prefix contains the selected effective prefix and project-layer writes.
	prefix configuration.InitializationPrefix

	// configWritten reports that this invocation published config.yaml.
	configWritten bool
}

func (i *initializer) ensureSettings(
	ctx context.Context,
	directory string,
	projectName string,
	requested *string,
	mode cli.InitConfigMode,
) (initializationSettings, error) {
	store := &yamlstore.Store{Directory: directory}
	overrides, err := store.ReadStoreConfiguration(ctx)
	if err != nil {
		return initializationSettings{}, err
	}
	hasDocument, err := store.HasDocument()
	if err != nil {
		return initializationSettings{}, err
	}
	if !hasDocument {
		overrides = configuration.Overrides{}
	}
	prefix, err := configuration.SelectInitializationPrefix(
		projectName,
		requested,
		overrides,
	)
	if err != nil {
		return initializationSettings{}, err
	}
	settings := initializationSettings{storeOverrides: overrides, prefix: prefix}
	if hasDocument {
		return settings, nil
	}
	switch mode {
	case cli.InitConfigWriteMissing:
		if err := store.WriteInitializationTemplate(); err != nil {
			return initializationSettings{}, err
		}
		settings.configWritten = true
		return settings, nil
	case cli.InitConfigSkipMissing:
		return settings, nil
	default:
		return initializationSettings{}, fmt.Errorf(
			"unsupported initialization config mode %d",
			mode,
		)
	}
}

func prefixString(prefix *configuration.Prefix) *string {
	if prefix == nil {
		return nil
	}
	return new(prefix.String())
}

func resolveStoreConfiguration(overrides configuration.Overrides) configuration.Configuration {
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

func configureGitIgnore(projectDirectory, storeDirectory string) cli.InitIgnoreOutcome {
	command := exec.Command(
		"git", "-C", projectDirectory, "rev-parse", "--show-toplevel", "--git-path", "info/exclude",
	)
	output, err := command.Output()
	if err != nil {
		return cli.InitIgnoreManual
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 2 {
		return cli.InitIgnoreManual
	}
	root := filepath.Clean(lines[0])
	excludePath := filepath.Clean(lines[1])
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return cli.InitIgnoreManual
	}
	storeDirectory, err = filepath.EvalSymlinks(storeDirectory)
	if err != nil {
		return cli.InitIgnoreManual
	}
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(projectDirectory, excludePath)
	}
	relativeStore, err := filepath.Rel(root, storeDirectory)
	if err != nil || relativeStore == ".." || strings.HasPrefix(relativeStore, ".."+string(filepath.Separator)) {
		return cli.InitIgnoreManual
	}
	patterns := []string{
		filepath.ToSlash(filepath.Join(relativeStore, "board.sqlite3")) + "*",
		filepath.ToSlash(filepath.Join(relativeStore, "blobs")) + "/",
	}
	body, err := os.ReadFile(excludePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return cli.InitIgnoreManual
	}
	existing := strings.Split(string(body), "\n")
	changed := false
	for _, pattern := range patterns {
		if slices.Contains(existing, pattern) {
			continue
		}
		if len(body) > 0 && body[len(body)-1] != '\n' {
			body = append(body, '\n')
		}
		body = append(body, pattern...)
		body = append(body, '\n')
		changed = true
	}
	if !changed {
		return cli.InitIgnoreUnchanged
	}
	if err := os.WriteFile(excludePath, body, 0o644); err != nil {
		return cli.InitIgnoreManual
	}
	return cli.InitIgnoreStoragePatternsAdded
}

func absolutePath(path, base string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return filepath.Abs(path)
}

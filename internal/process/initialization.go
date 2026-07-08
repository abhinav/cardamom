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
		directory,
		request.IDPrefix,
		request.ConfigMode,
	)
	if err != nil {
		return cli.InitResult{}, err
	}
	effective := storeConfiguration(settings.effective)
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
		ProjectIDPrefix: settings.projectIDPrefix,
	})
	if err != nil {
		return cli.InitResult{}, err
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

// initializationSettings routes initialization configuration to the store
// file or initial project row while retaining one effective result.
type initializationSettings struct {
	// effective contains the overrides applied by this initialization.
	effective configuration.Overrides

	// projectIDPrefix persists a requested prefix when config.yaml stays
	// absent.
	projectIDPrefix *string

	// configWritten reports that this invocation published config.yaml.
	configWritten bool
}

func (i *initializer) ensureSettings(
	directory string,
	requested *string,
	mode cli.InitConfigMode,
) (initializationSettings, error) {
	path := settingsPath(directory)
	overrides, err := readSettings(directory)
	if err != nil {
		return initializationSettings{}, err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		return initializationSettings{effective: overrides}, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return initializationSettings{}, fmt.Errorf("stat %q: %w", path, statErr)
	}
	overrides = configuration.Overrides{}
	if requested != nil {
		prefix, err := configuration.NewPrefix(*requested)
		if err != nil {
			return initializationSettings{}, err
		}
		overrides.Issue.ID.Prefix = &prefix
	}
	switch mode {
	case cli.InitConfigWriteMissing:
		if err := writeSettings(path, overrides); err != nil {
			return initializationSettings{}, err
		}
		return initializationSettings{
			effective: overrides, configWritten: true,
		}, nil
	case cli.InitConfigSkipMissing:
		var projectIDPrefix *string
		if overrides.Issue.ID.Prefix != nil {
			projectIDPrefix = new(overrides.Issue.ID.Prefix.String())
		}
		return initializationSettings{
			effective:       overrides,
			projectIDPrefix: projectIDPrefix,
		}, nil
	default:
		return initializationSettings{}, fmt.Errorf(
			"unsupported initialization config mode %d",
			mode,
		)
	}
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

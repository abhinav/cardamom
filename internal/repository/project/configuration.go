package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ReadConfigurationLayers returns coherent project and board overrides for
// one selected board.
func (r *Repository) ReadConfigurationLayers(
	ctx context.Context,
	boardID board.ID,
) (out configuration.DatabaseLayers, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	row, err := query.New(view).ProjectGetConfigurationLayers(ctx, boardID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return out, errkind.Errorf(errkind.NotFound, "board not found")
	}
	if err != nil {
		return out, err
	}
	return loadConfigurationLayers(row)
}

// ReadProjectConfiguration returns one project's overrides without requiring
// that the project contain a board.
// A missing project returns NotFound.
func (r *Repository) ReadProjectConfiguration(
	ctx context.Context,
	projectID project.ID,
) (out configuration.Overrides, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	row, err := query.New(view).ProjectGetProjectConfiguration(
		ctx, projectID.String(),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return out, errkind.Errorf(errkind.NotFound, "project not found")
	}
	if err != nil {
		return out, err
	}
	return (nullableConfiguration{
		prefix:             row.IssueIDPrefix,
		strategy:           row.IssueIDStrategy,
		summaryMaxBytes:    row.IssueSummaryMaxBytes,
		attachmentMaxBytes: row.AttachmentMaxBytes,
	}).overrides()
}

// UpdateProjectConfiguration atomically applies one typed project patch.
func (r *Repository) UpdateProjectConfiguration(
	ctx context.Context,
	projectID project.ID,
	patch configuration.Patch,
) (out configuration.Overrides, err error) {
	if err := patch.Validate(); err != nil {
		return out, err
	}
	change, err := r.store.Change(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	queries := query.New(change)
	row, err := queries.ProjectGetProjectConfiguration(ctx, projectID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return out, errkind.Errorf(errkind.NotFound, "project not found")
	}
	if err != nil {
		return out, err
	}
	current, err := (nullableConfiguration{
		prefix:             row.IssueIDPrefix,
		strategy:           row.IssueIDStrategy,
		summaryMaxBytes:    row.IssueSummaryMaxBytes,
		attachmentMaxBytes: row.AttachmentMaxBytes,
	}).overrides()
	if err != nil {
		return out, err
	}
	out = patch.Apply(current)
	if out.Equal(current) {
		return out, change.Commit()
	}
	reservation, err := change.ReserveRevision(ctx)
	if err != nil {
		return out, err
	}
	values := nullableConfigurationFromOverrides(out)
	if err := queries.ProjectUpdateProjectConfiguration(
		ctx,
		query.ProjectUpdateProjectConfigurationParams{
			IssueIDPrefix:        values.prefix,
			IssueIDStrategy:      values.strategy,
			IssueSummaryMaxBytes: values.summaryMaxBytes,
			AttachmentMaxBytes:   values.attachmentMaxBytes,
			ID:                   projectID.String(),
		},
	); err != nil {
		return out, fmt.Errorf("update project configuration: %w", err)
	}
	if err := change.PublishRevision(ctx, reservation); err != nil {
		return out, err
	}
	return out, change.Commit()
}

// UpdateBoardConfiguration atomically applies one typed board patch.
func (r *Repository) UpdateBoardConfiguration(
	ctx context.Context,
	boardID board.ID,
	patch configuration.Patch,
) (out configuration.Overrides, err error) {
	if err := patch.Validate(); err != nil {
		return out, err
	}
	change, err := r.store.Change(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	queries := query.New(change)
	row, err := queries.ProjectGetBoardConfiguration(ctx, boardID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return out, errkind.Errorf(errkind.NotFound, "board not found")
	}
	if err != nil {
		return out, err
	}
	current, err := (nullableConfiguration{
		prefix:             row.IssueIDPrefix,
		strategy:           row.IssueIDStrategy,
		summaryMaxBytes:    row.IssueSummaryMaxBytes,
		attachmentMaxBytes: row.AttachmentMaxBytes,
	}).overrides()
	if err != nil {
		return out, err
	}
	out = patch.Apply(current)
	if out.Equal(current) {
		return out, change.Commit()
	}
	reservation, err := change.ReserveRevision(ctx)
	if err != nil {
		return out, err
	}
	values := nullableConfigurationFromOverrides(out)
	if err := queries.ProjectUpdateBoardConfiguration(
		ctx,
		query.ProjectUpdateBoardConfigurationParams{
			IssueIDPrefix:        values.prefix,
			IssueIDStrategy:      values.strategy,
			IssueSummaryMaxBytes: values.summaryMaxBytes,
			AttachmentMaxBytes:   values.attachmentMaxBytes,
			ID:                   boardID.String(),
		},
	); err != nil {
		return out, fmt.Errorf("update board configuration: %w", err)
	}
	if err := change.PublishRevision(ctx, reservation); err != nil {
		return out, err
	}
	return out, change.Commit()
}

func loadConfigurationLayers(
	row query.ProjectGetConfigurationLayersRow,
) (configuration.DatabaseLayers, error) {
	parsedProjectID, err := project.NewID(row.ProjectID)
	if err != nil {
		return configuration.DatabaseLayers{}, err
	}
	projectOverrides, err := (nullableConfiguration{
		prefix:             row.ProjectIssueIDPrefix,
		strategy:           row.ProjectIssueIDStrategy,
		summaryMaxBytes:    row.ProjectIssueSummaryMaxBytes,
		attachmentMaxBytes: row.ProjectAttachmentMaxBytes,
	}).overrides()
	if err != nil {
		return configuration.DatabaseLayers{}, err
	}
	boardOverrides, err := (nullableConfiguration{
		prefix:             row.BoardIssueIDPrefix,
		strategy:           row.BoardIssueIDStrategy,
		summaryMaxBytes:    row.BoardIssueSummaryMaxBytes,
		attachmentMaxBytes: row.BoardAttachmentMaxBytes,
	}).overrides()
	if err != nil {
		return configuration.DatabaseLayers{}, err
	}
	return configuration.DatabaseLayers{
		ProjectID: parsedProjectID,
		Project:   projectOverrides,
		Board:     boardOverrides,
	}, nil
}

type nullableConfiguration struct {
	prefix             *string
	strategy           *string
	summaryMaxBytes    *int64
	attachmentMaxBytes *int64
}

func (v nullableConfiguration) overrides() (configuration.Overrides, error) {
	var overrides configuration.Overrides
	if v.prefix != nil {
		prefix, err := configuration.NewPrefix(*v.prefix)
		if err != nil {
			return overrides, err
		}
		overrides.Issue.ID.Prefix = &prefix
	}
	if v.strategy != nil {
		strategy, err := configuration.NewIDStrategy(*v.strategy)
		if err != nil {
			return overrides, err
		}
		overrides.Issue.ID.Strategy = &strategy
	}
	if v.summaryMaxBytes != nil {
		limit, err := configuration.NewByteLimit(uint64(*v.summaryMaxBytes))
		if err != nil {
			return overrides, err
		}
		overrides.Issue.Summary.MaxBytes = &limit
	}
	if v.attachmentMaxBytes != nil {
		limit, err := configuration.NewByteLimit(uint64(*v.attachmentMaxBytes))
		if err != nil {
			return overrides, err
		}
		overrides.Attachment.MaxBytes = &limit
	}
	return overrides, nil
}

func nullableConfigurationFromOverrides(overrides configuration.Overrides) nullableConfiguration {
	return nullableConfiguration{
		prefix:             nullablePrefix(overrides.Issue.ID.Prefix),
		strategy:           nullableStrategy(overrides.Issue.ID.Strategy),
		summaryMaxBytes:    nullableLimit(overrides.Issue.Summary.MaxBytes),
		attachmentMaxBytes: nullableLimit(overrides.Attachment.MaxBytes),
	}
}

func nullablePrefix(value *configuration.Prefix) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func nullableStrategy(value *configuration.IDStrategy) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func nullableLimit(value *configuration.ByteLimit) *int64 {
	if value == nil {
		return nil
	}
	limit := int64(value.Uint64())
	return &limit
}

var _ configuration.Repository = (*Repository)(nil)

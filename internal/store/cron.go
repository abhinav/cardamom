package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// CronJobAdd inserts a new scheduled job. Returns ErrCronJobExists if a job
// with the same name already exists.
func (s *Store) CronJobAdd(ctx context.Context, j CronJob) error {
	if j.Name == "" {
		return errors.New("name required")
	}
	_, err := s.db.NewInsert().Model(&j).Exec(ctx)
	if isUniqueErr(err) {
		return fmt.Errorf("%w: %s", ErrCronJobExists, j.Name)
	}
	return err
}

// CronJobGet looks up one job by name.
func (s *Store) CronJobGet(ctx context.Context, name string) (CronJob, error) {
	var j CronJob
	err := s.db.NewSelect().Model(&j).Where("name = ?", name).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return CronJob{}, ErrCronJobNotFound
	}
	return j, err
}

// CronJobList returns every job, alphabetised by name.
func (s *Store) CronJobList(ctx context.Context) ([]CronJob, error) {
	var jobs []CronJob
	err := s.db.NewSelect().Model(&jobs).OrderExpr("name ASC").Scan(ctx)
	return jobs, err
}

// CronJobDelete removes a job by name. Returns ErrCronJobNotFound if absent.
func (s *Store) CronJobDelete(ctx context.Context, name string) error {
	res, err := s.db.NewDelete().Model((*CronJob)(nil)).Where("name = ?", name).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCronJobNotFound
	}
	return nil
}

// CronJobSetEnabled flips the enabled bit. Returns ErrCronJobNotFound if absent.
func (s *Store) CronJobSetEnabled(ctx context.Context, name string, enabled bool) error {
	res, err := s.db.NewUpdate().
		Model((*CronJob)(nil)).
		Set("enabled = ?", enabled).
		Where("name = ?", name).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCronJobNotFound
	}
	return nil
}

// CronJobsDue returns the enabled jobs whose next_run is now or earlier,
// in deterministic (name) order.
func (s *Store) CronJobsDue(ctx context.Context, asOf int64) ([]CronJob, error) {
	var jobs []CronJob
	err := s.db.NewSelect().
		Model(&jobs).
		Where("enabled = 1 AND next_run <= ?", asOf).
		OrderExpr("name ASC").
		Scan(ctx)
	return jobs, err
}

// CronJobRecordRun stores the outcome of one execution and advances next_run.
// status/output are nullable: pass empty strings to mean "no value".
func (s *Store) CronJobRecordRun(ctx context.Context, name string, ranAt, nextRun int64, status, output string) error {
	var statusP, outputP *string
	if status != "" {
		statusP = &status
	}
	if output != "" {
		outputP = &output
	}
	res, err := s.db.NewUpdate().
		Model((*CronJob)(nil)).
		Set("last_run = ?", ranAt).
		Set("last_status = ?", statusP).
		Set("last_output = ?", outputP).
		Set("next_run = ?", nextRun).
		Where("name = ?", name).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCronJobNotFound
	}
	return nil
}

// CronJobUpsert inserts or replaces a job, preserving the same name. Used by
// import; bypasses the duplicate-name check in CronJobAdd.
func (s *Store) CronJobUpsert(ctx context.Context, j CronJob) error {
	_, err := s.db.NewInsert().Model(&j).
		On("CONFLICT (name) DO UPDATE").
		Set("schedule = EXCLUDED.schedule").
		Set("job = EXCLUDED.job").
		Set("enabled = EXCLUDED.enabled").
		Set("next_run = EXCLUDED.next_run").
		Set("last_run = EXCLUDED.last_run").
		Set("last_status = EXCLUDED.last_status").
		Set("last_output = EXCLUDED.last_output").
		Exec(ctx)
	return err
}

// CronJobUpdateSchedule changes a job's schedule and recomputes next_run as
// the caller specified. Use when a user edits an existing entry.
func (s *Store) CronJobUpdateSchedule(ctx context.Context, name, schedule string, nextRun int64) error {
	res, err := s.db.NewUpdate().
		Model((*CronJob)(nil)).
		Set("schedule = ?", schedule).
		Set("next_run = ?", nextRun).
		Where("name = ?", name).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCronJobNotFound
	}
	return nil
}

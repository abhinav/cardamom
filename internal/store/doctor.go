package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/uptrace/bun"
)

// DBVersion returns the PRAGMA user_version of the underlying database.
func (s *Store) DBVersion(ctx context.Context) (int, error) {
	var v int
	err := s.db.NewRaw("PRAGMA user_version").Scan(ctx, &v)
	return v, err
}

// DoctorReport is the result of a health check.
type DoctorReport struct {
	DBSchemaVersion   int      `json:"db_schema_version"`
	CodeSchemaVersion int      `json:"code_schema_version"`
	ForeignKeyOK      bool     `json:"foreign_key_ok"`
	ForeignKeyErrors  []string `json:"foreign_key_errors,omitempty"`
	OrphanedLabels    int      `json:"orphaned_labels"`
	OrphanedDeps      int      `json:"orphaned_deps"`
	StuckInProgress   int      `json:"stuck_in_progress"`
	StuckThresholdH   int      `json:"stuck_threshold_hours"`
	ClosedButDeferred int      `json:"closed_but_deferred"`
	InvalidStatus     int      `json:"invalid_status"`
	InvalidType       int      `json:"invalid_type"`
	InvalidPriority   int      `json:"invalid_priority"`
	CronJobsFailing   int      `json:"cron_jobs_failing"`
}

// OK returns true when nothing is wrong.
func (d DoctorReport) OK() bool {
	return d.DBSchemaVersion == d.CodeSchemaVersion &&
		d.ForeignKeyOK &&
		d.OrphanedLabels == 0 &&
		d.OrphanedDeps == 0 &&
		d.StuckInProgress == 0 &&
		d.ClosedButDeferred == 0 &&
		d.InvalidStatus == 0 &&
		d.InvalidType == 0 &&
		d.InvalidPriority == 0 &&
		d.CronJobsFailing == 0
}

// Doctor runs integrity checks against the live database. Issues
// "stuck" longer than stuckThresholdHours are flagged. Pass 0 to
// disable the stuck check.
func (s *Store) Doctor(ctx context.Context, stuckThresholdHours int) (DoctorReport, error) {
	r := DoctorReport{
		CodeSchemaVersion: SchemaVersion(),
		StuckThresholdH:   stuckThresholdHours,
	}
	v, err := s.DBVersion(ctx)
	if err != nil {
		return r, err
	}
	r.DBSchemaVersion = v

	// PRAGMA foreign_key_check returns one row per violation; empty = OK.
	rows, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var table, rowid, parent, fkid sql.NullString
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err == nil {
			r.ForeignKeyErrors = append(r.ForeignKeyErrors,
				fmt.Sprintf("%s rowid=%s ref=%s fk=%s", table.String, rowid.String, parent.String, fkid.String))
		}
	}
	r.ForeignKeyOK = len(r.ForeignKeyErrors) == 0

	// FKs with CASCADE should prevent orphans, but check anyway as a belt-and-braces.
	if err := s.db.NewRaw("SELECT COUNT(*) FROM issue_labels WHERE issue_id NOT IN (SELECT id FROM issues)").Scan(ctx, &r.OrphanedLabels); err != nil {
		return r, err
	}
	if err := s.db.NewRaw("SELECT COUNT(*) FROM deps WHERE child_id NOT IN (SELECT id FROM issues) OR parent_id NOT IN (SELECT id FROM issues)").Scan(ctx, &r.OrphanedDeps); err != nil {
		return r, err
	}

	if stuckThresholdHours > 0 {
		cutoff := now() - int64(stuckThresholdHours)*3600
		if err := s.db.NewRaw("SELECT COUNT(*) FROM issues WHERE status = 'in_progress' AND updated < ?", cutoff).Scan(ctx, &r.StuckInProgress); err != nil {
			return r, err
		}
	}

	if err := s.db.NewRaw("SELECT COUNT(*) FROM issues WHERE status = 'closed' AND defer_until IS NOT NULL").Scan(ctx, &r.ClosedButDeferred); err != nil {
		return r, err
	}

	// Pre-existing rows that escape current validation. The store
	// itself rejects bad values at write time, but older rows (or
	// imports that bypass Create/Update) may still be in violation.
	if r.InvalidStatus, err = s.db.NewSelect().Model((*Issue)(nil)).
		Where("i.status NOT IN (?)", bun.In(ValidStatuses)).Count(ctx); err != nil {
		return r, err
	}
	if r.InvalidType, err = s.db.NewSelect().Model((*Issue)(nil)).
		Where("i.type NOT IN (?)", bun.In(ValidTypes)).Count(ctx); err != nil {
		return r, err
	}
	if r.InvalidPriority, err = s.db.NewSelect().Model((*Issue)(nil)).
		Where("i.priority < ? OR i.priority > ?", MinPriority, MaxPriority).Count(ctx); err != nil {
		return r, err
	}
	if r.CronJobsFailing, err = s.db.NewSelect().Model((*CronJob)(nil)).
		Where("last_status IS NOT NULL AND last_status != 'ok'").Count(ctx); err != nil {
		return r, err
	}
	return r, nil
}

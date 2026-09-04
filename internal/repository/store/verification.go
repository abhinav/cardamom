package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// verifyStore checks physical and canonical invariants that must hold before a
// Store is published. An empty project catalog is valid until product
// initialization completes it.
func verifyStore(
	ctx context.Context,
	db *sql.DB,
	databaseSchemaVersion int64,
) error {
	// Establish physical page integrity and owner-spanning references first;
	// later phases must be able to trust every row they interpret.
	if err := verifyQuickCheck(ctx, db); err != nil {
		return err
	}
	queries := query.New(db)
	var foreignKeyErrors int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM pragma_foreign_key_check
	`).Scan(&foreignKeyErrors); err != nil {
		return fmt.Errorf("verify foreign keys: %w", err)
	}
	if foreignKeyErrors != 0 {
		return fmt.Errorf("verify foreign keys: found %d violations", foreignKeyErrors)
	}

	// Establish binary/schema agreement and the canonical scalar head before
	// repositories receive scopes that depend on that layout and revision.
	if databaseSchemaVersion != SchemaVersion() {
		return fmt.Errorf(
			"verify schema version: database has %d; binary has %d",
			databaseSchemaVersion,
			SchemaVersion(),
		)
	}

	stateRows, err := queries.StoreCountStateRows(ctx)
	if err != nil {
		return fmt.Errorf("verify store state: count rows: %w", err)
	}
	if stateRows != 1 {
		return fmt.Errorf("verify store state: found %d rows; want 1", stateRows)
	}

	state, err := queries.StoreGetVerificationState(ctx)
	if err != nil {
		return fmt.Errorf("verify store state: read singleton: %w", err)
	}
	if state.CurrentRevision < 0 {
		return fmt.Errorf("verify store state: current revision %d is negative", state.CurrentRevision)
	}
	if state.NextIssueNumber < 1 {
		return fmt.Errorf(
			"verify store state: next issue number %d; want at least 1",
			state.NextIssueNumber,
		)
	}
	invalidProjectionRevisions, err := queries.StoreCountInvalidProjectionRevisions(
		ctx,
		state.CurrentRevision,
	)
	if err != nil {
		return fmt.Errorf("verify projection revisions: %w", err)
	}
	if invalidProjectionRevisions != 0 {
		return fmt.Errorf(
			"verify projection revisions: found %d invalid rows",
			invalidProjectionRevisions,
		)
	}

	invalidSearchDocuments, err := queries.StoreCountInvalidIssueSearchDocuments(ctx)
	if err != nil {
		return fmt.Errorf("verify issue search documents: %w", err)
	}
	if invalidSearchDocuments != 0 {
		return fmt.Errorf(
			"verify issue search documents: found %d inconsistent rows",
			invalidSearchDocuments,
		)
	}
	if err := queries.StoreCheckIssueSearchIndex(ctx); err != nil {
		return fmt.Errorf("verify issue search index: %w", err)
	}

	// Claim eligibility spans issue workflow and current custody projections.
	invalidClaims, err := queries.StoreCountInvalidClaims(ctx)
	if err != nil {
		return fmt.Errorf("verify active claims: %w", err)
	}
	if invalidClaims != 0 {
		return fmt.Errorf("verify active claims: found %d invalid claims", invalidClaims)
	}
	return nil
}

func verifyQuickCheck(ctx context.Context, db *sql.DB) (err error) {
	rows, err := db.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return fmt.Errorf("verify SQLite quick check: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("verify SQLite quick check result: %w", err)
		}
		if result != "ok" {
			return fmt.Errorf("verify SQLite quick check: %s", result)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("verify SQLite quick check: %w", err)
	}
	return nil
}

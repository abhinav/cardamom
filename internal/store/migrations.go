package store

import (
	"database/sql"
	"fmt"
)

// migrations are applied in order; PRAGMA user_version tracks progress.
// Append new migrations, never edit existing ones.
var migrations = []string{
	// v1: initial schema.
	`
    CREATE TABLE IF NOT EXISTS issues (
        id        TEXT PRIMARY KEY,
        title     TEXT NOT NULL,
        type      TEXT NOT NULL DEFAULT 'task',
        status    TEXT NOT NULL DEFAULT 'open',
        priority  INTEGER NOT NULL DEFAULT 2,
        assignee  TEXT,
        created   INTEGER NOT NULL,
        updated   INTEGER NOT NULL,
        closed    INTEGER
    );
    CREATE INDEX IF NOT EXISTS idx_issues_status ON issues(status);

    CREATE TABLE IF NOT EXISTS deps (
        child_id  TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
        parent_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
        PRIMARY KEY (child_id, parent_id)
    );
    CREATE INDEX IF NOT EXISTS idx_deps_parent ON deps(parent_id);
    `,
	// v2: agent lane.
	`
    ALTER TABLE issues ADD COLUMN agent TEXT;
    CREATE INDEX IF NOT EXISTS idx_issues_agent ON issues(agent);
    `,
	// v3: labels.
	`
    CREATE TABLE IF NOT EXISTS issue_labels (
        issue_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
        label    TEXT NOT NULL,
        PRIMARY KEY (issue_id, label)
    );
    CREATE INDEX IF NOT EXISTS idx_issue_labels_label ON issue_labels(label);
    `,
	// v4: defer_until.
	`
    ALTER TABLE issues ADD COLUMN defer_until INTEGER;
    CREATE INDEX IF NOT EXISTS idx_issues_defer_until ON issues(defer_until);
    `,
	// v5: description.
	`
    ALTER TABLE issues ADD COLUMN description TEXT;
    `,
	// v6: notes.
	`
    ALTER TABLE issues ADD COLUMN notes TEXT;
    `,
	// v7: comments.
	`
    CREATE TABLE IF NOT EXISTS comments (
        id       INTEGER PRIMARY KEY AUTOINCREMENT,
        issue_id TEXT    NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
        author   TEXT    NOT NULL,
        body     TEXT    NOT NULL,
        created  INTEGER NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_comments_issue ON comments(issue_id, id);
    `,
	// v8: kv (generic key-value store).
	`
    CREATE TABLE IF NOT EXISTS kv (
        key   TEXT PRIMARY KEY,
        value TEXT NOT NULL
    );
    `,
	// v9: cron jobs (scheduled invocations).
	//
	// job is a JSON-encoded tagged union so the schema doesn't have to
	// grow as we add new kinds. For v1 only kind="cli" exists:
	//   {"kind":"cli","args":["create","-a","infra-agent","Check CI"]}
	`
    CREATE TABLE IF NOT EXISTS cron_jobs (
        name        TEXT PRIMARY KEY,
        schedule    TEXT NOT NULL,
        job         TEXT NOT NULL,
        enabled     INTEGER NOT NULL DEFAULT 1,
        next_run    INTEGER NOT NULL,
        last_run    INTEGER,
        last_status TEXT,
        last_output TEXT
    );
    CREATE INDEX IF NOT EXISTS idx_cron_jobs_due ON cron_jobs(enabled, next_run);
    `,
	// v10: active agents (heartbeat from --wait/--watch loops).
	`
    CREATE TABLE IF NOT EXISTS active_agents (
        name         TEXT PRIMARY KEY,
        pid          INTEGER NOT NULL,
        host         TEXT NOT NULL,
        capabilities TEXT NOT NULL,
        started_at   INTEGER NOT NULL,
        last_seen    INTEGER NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_active_agents_last_seen ON active_agents(last_seen);
    `,
	// v11: named locks for ad-hoc coordination.
	//
	// `expires` is a required wall-clock TTL — there is no infinite
	// lock. The acquire path overwrites a row whose expires has passed,
	// so a crashed holder can't wedge the lock forever. PID + host are
	// recorded for `clu locks` diagnostics, not enforcement.
	`
    CREATE TABLE IF NOT EXISTS locks (
        name     TEXT PRIMARY KEY,
        holder   TEXT NOT NULL,
        pid      INTEGER NOT NULL,
        host     TEXT NOT NULL,
        acquired INTEGER NOT NULL,
        expires  INTEGER NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_locks_expires ON locks(expires);
    `,
	// v12: mailbox for fire-and-forget inter-agent messages (clu ping
	// + clu inbox). Distinct from issues + comments: ephemeral, expires
	// after a TTL, doesn't pollute the work log. read_at NULL = unread.
	`
    CREATE TABLE IF NOT EXISTS mailbox (
        id        INTEGER PRIMARY KEY AUTOINCREMENT,
        sender    TEXT    NOT NULL,
        recipient TEXT    NOT NULL,
        body      TEXT    NOT NULL,
        created   INTEGER NOT NULL,
        expires   INTEGER NOT NULL,
        read_at   INTEGER
    );
    CREATE INDEX IF NOT EXISTS idx_mailbox_recipient_read ON mailbox(recipient, read_at);
    CREATE INDEX IF NOT EXISTS idx_mailbox_expires ON mailbox(expires);
    `,
	// v13: collapse `agent` (lane) into `assignee`. Pre-routing and
	// claiming were two columns serving one concept in our single-user
	// model; the split caused user-visible drift (`clu assign <id> X`
	// set assignee but not agent, so `clu list -a X` returned empty).
	// Coalesce existing data, drop the agent column.
	//
	// SQLite ≥ 3.35 supports DROP COLUMN; modernc.org/sqlite is recent
	// enough. The index drop comes first so dropping the column doesn't
	// fail on a dependent index.
	`
    UPDATE issues SET assignee = agent WHERE assignee IS NULL AND agent IS NOT NULL;
    DROP INDEX IF EXISTS idx_issues_agent;
    ALTER TABLE issues DROP COLUMN agent;
    CREATE INDEX IF NOT EXISTS idx_issues_assignee ON issues(assignee);
    `,
	// v14: topic subscriptions for ping fan-out. A subscription is
	// "listener X is interested in any ping whose recipient matches
	// pattern P." At send time, `PingSend` delivers to the literal
	// recipient *and* to every subscription whose pattern matches.
	// Required TTL like locks — no infinite subscriptions; dead
	// listeners (no refresh) drop off.
	`
    CREATE TABLE IF NOT EXISTS subscriptions (
        id       INTEGER PRIMARY KEY AUTOINCREMENT,
        listener TEXT    NOT NULL,
        pattern  TEXT    NOT NULL,
        pid      INTEGER NOT NULL,
        host     TEXT    NOT NULL,
        created  INTEGER NOT NULL,
        expires  INTEGER NOT NULL,
        UNIQUE (listener, pattern)
    );
    CREATE INDEX IF NOT EXISTS idx_subscriptions_listener ON subscriptions(listener);
    CREATE INDEX IF NOT EXISTS idx_subscriptions_expires  ON subscriptions(expires);
    `,
	// v15: append-only audit log for issue + comment history. Issue
	// write paths (create, claim, close, reopen, cancel, update, defer,
	// notes, label add/rm, dep add/rm) and comment writes record who
	// changed what, when. Deliberately NOT recorded: ephemeral
	// coordination state (locks, mailbox/ping, agent heartbeats,
	// subscriptions) and the side stores (kv, cron) — heartbeats alone
	// would flood the log. These events are local only and are not
	// included in export/import (which carries portable state, not
	// audit history).
	//
	// `payload` is JSON holding *only the fields that changed* (e.g.
	// {"status":"closed"}), not a full snapshot — the row `id` is the
	// handle for richer lookups. issue_id / actor are nullable: actor is
	// whatever identity the CLI resolved ($USER or --agent). No FK on
	// issue_id: events outlive the issues they describe (history
	// survives a hard delete).
	`
    CREATE TABLE IF NOT EXISTS events (
        id       INTEGER PRIMARY KEY AUTOINCREMENT,
        issue_id TEXT,
        actor    TEXT,
        kind     TEXT    NOT NULL,
        payload  TEXT,
        ts       INTEGER NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_events_issue ON events(issue_id, ts);
    CREATE INDEX IF NOT EXISTS idx_events_ts    ON events(ts);
    `,
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for ; version < len(migrations); version++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[version]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", version+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// SchemaVersion is the number of migrations applied to a fresh DB.
// Equal to PRAGMA user_version after Open() completes.
func SchemaVersion() int { return len(migrations) }

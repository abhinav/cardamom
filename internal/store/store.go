// Package store is the SQLite-backed persistence layer for clu.
//
// The file layout splits the package by domain:
//
//	store.go       core: Store, Open, options, tiny helpers
//	models.go      bun model types (Issue, Dep, Comment, …)
//	migrations.go  migration SQL + migrate(); SchemaVersion()
//	validate.go    ValidStatuses/ValidTypes/MinPriority + Validate*
//	errors.go      sentinel errors + isUniqueErr
//	issues.go      issue CRUD: Create, Get, MarkClosed, Reopen, Cancel,
//	               Update, SetDefer, SetNotes, AppendNote, UpsertIssue
//	list.go        ListFilter + List/Count/Stats/Blocked
//	claim.go       Ready/Claim/ClaimByID/WaitReady + lane matching
//	labels.go      AddLabels/RemoveLabels/LabelsForIssue/LoadLabels
//	deps.go        AddDep/RemoveDep/Deps/IDsBlocked/AllDeps + cycle check
//	comments.go    AddComment/Comments/RemoveComment/UpsertComment
//	kv.go          KVSet/Get/Delete/List
//	cron.go        CronJob* (add/get/list/delete/setEnabled/due/recordRun)
//	agents.go      ActiveAgent heartbeat: AgentTouch/Remove/List/Get
//	doctor.go      Doctor + DoctorReport + DBVersion
//	ids.go         DefaultIDPrefix + newID()
package store

import (
	"database/sql"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	_ "modernc.org/sqlite"
)

type Store struct {
	db       *bun.DB
	idPrefix string
}

// Option configures Open. Use the With* helpers; the option struct
// itself isn't exported.
type Option func(*openOpts)

type openOpts struct {
	idPrefix string
}

// WithIDPrefix overrides the prefix newly-created issues get. Passing
// "" or omitting the option uses DefaultIDPrefix.
func WithIDPrefix(p string) Option {
	return func(o *openOpts) { o.idPrefix = p }
}

func Open(path string, opts ...Option) (*Store, error) {
	o := openOpts{idPrefix: DefaultIDPrefix}
	for _, fn := range opts {
		fn(&o)
	}
	if o.idPrefix == "" {
		o.idPrefix = DefaultIDPrefix
	}
	sqldb, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if err := migrate(sqldb); err != nil {
		sqldb.Close()
		return nil, err
	}
	db := bun.NewDB(sqldb, sqlitedialect.New())
	return &Store{db: db, idPrefix: o.idPrefix}, nil
}

// IDPrefix returns the prefix this Store uses for newly-created issues.
// Surfaced by `clu info`.
func (s *Store) IDPrefix() string { return s.idPrefix }

func (s *Store) Close() error { return s.db.Close() }

func now() int64 { return time.Now().Unix() }

// placeholders returns "?, ?, ?, ..." with n placeholders.
// Used by raw-SQL paths in claim.go and issues.go (Cancel) that can't
// rely on bun.List() expansion.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?, ", n-1) + "?"
}

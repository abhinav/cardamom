package store

import (
	"errors"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	ErrNotFound        = errors.New("issue not found")
	ErrAlreadyClosed   = errors.New("issue already closed")
	ErrAlreadyOpen     = errors.New("issue already open")
	ErrNotClaimable    = errors.New("issue not claimable")
	ErrCycle           = errors.New("dependency would create a cycle")
	ErrSelfDep         = errors.New("issue cannot depend on itself")
	ErrKVNotFound      = errors.New("key not found")
	ErrCommentNotFound = errors.New("comment not found")
	ErrDepNotFound     = errors.New("dependency edge not found")
	ErrNotDeferred     = errors.New("issue is not deferred")
	ErrCronJobNotFound = errors.New("cron job not found")
	ErrCronJobExists   = errors.New("cron job already exists")
	ErrLockHeld        = errors.New("lock is currently held")
	ErrLockNotFound    = errors.New("lock not found")
	ErrLockNotHolder   = errors.New("lock is held by someone else")
)

func isUniqueErr(err error) bool {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return false
	}
	switch se.Code() {
	case sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY,
		sqlite3.SQLITE_CONSTRAINT_UNIQUE:
		return true
	}
	return false
}

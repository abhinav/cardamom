package store

import "github.com/uptrace/bun"

type Issue struct {
	bun.BaseModel `bun:"table:issues,alias:i" json:"-"`

	ID          string  `bun:"id,pk" json:"id"`
	Title       string  `bun:"title,notnull" json:"title"`
	Type        string  `bun:"type,notnull" json:"type"`
	Status      string  `bun:"status,notnull" json:"status"`
	Priority    int     `bun:"priority,notnull" json:"priority"`
	Assignee    *string `bun:"assignee" json:"assignee,omitempty"`
	Created     int64   `bun:"created,notnull" json:"created"`
	Updated     int64   `bun:"updated,notnull" json:"updated"`
	StartedAt   *int64  `bun:"started_at" json:"started_at,omitempty"` // when it last entered in_progress
	Closed      *int64  `bun:"closed" json:"closed,omitempty"`
	DeferUntil  *int64  `bun:"defer_until" json:"defer_until,omitempty"`
	Description *string `bun:"description" json:"description,omitempty"`
	Notes       *string `bun:"notes" json:"notes,omitempty"`
}

type Dep struct {
	bun.BaseModel `bun:"table:deps"`

	ChildID  string `bun:"child_id,pk"`
	ParentID string `bun:"parent_id,pk"`
}

type IssueLabel struct {
	bun.BaseModel `bun:"table:issue_labels"`

	IssueID string `bun:"issue_id,pk"`
	Label   string `bun:"label,pk"`
}

type Comment struct {
	bun.BaseModel `bun:"table:comments,alias:c" json:"-"`

	ID      int64  `bun:"id,pk,autoincrement" json:"id"`
	IssueID string `bun:"issue_id,notnull" json:"issue_id"`
	Author  string `bun:"author,notnull" json:"author"`
	Body    string `bun:"body,notnull" json:"body"`
	Created int64  `bun:"created,notnull" json:"created"`
}

// KV is one entry in the generic key-value store. Independent of issues —
// use for feature flags, config snippets, persistent agent scratch data, etc.
type KV struct {
	bun.BaseModel `bun:"table:kv" json:"-"`

	Key   string `bun:"key,pk" json:"key"`
	Value string `bun:"value,notnull" json:"value"`
}

// ActiveAgent is the heartbeat row for one currently-running agent —
// some Claude Code session (or other process) sitting in a `claim --wait`
// or `list --watch` poll loop. The loop upserts last_seen each tick;
// queries filter out rows older than a freshness threshold so crashed
// processes drop off without explicit cleanup.
type ActiveAgent struct {
	bun.BaseModel `bun:"table:active_agents" json:"-"`

	Name         string `bun:"name,pk" json:"name"`
	PID          int    `bun:"pid,notnull" json:"pid"`
	Host         string `bun:"host,notnull" json:"host"`
	Capabilities string `bun:"capabilities,notnull" json:"capabilities"` // JSON array
	StartedAt    int64  `bun:"started_at,notnull" json:"started_at"`
	LastSeen     int64  `bun:"last_seen,notnull" json:"last_seen"`
}

// Subscription is one listener's interest in pings whose recipient
// matches `Pattern` (a SQLite GLOB / filepath.Match pattern — typically
// dotted topic paths like `release.*` but bare names work too).
//
// At ping time, the sender always delivers to the literal recipient
// AND fans out to every subscription whose pattern matches that
// recipient. `Expires` is required (no infinite subscriptions); dead
// listeners stop refreshing and drop off via opportunistic GC.
type Subscription struct {
	bun.BaseModel `bun:"table:subscriptions" json:"-"`

	ID       int64  `bun:"id,pk,autoincrement" json:"id"`
	Listener string `bun:"listener,notnull" json:"listener"`
	Pattern  string `bun:"pattern,notnull" json:"pattern"`
	PID      int    `bun:"pid,notnull" json:"pid"`
	Host     string `bun:"host,notnull" json:"host"`
	Created  int64  `bun:"created,notnull" json:"created"`
	Expires  int64  `bun:"expires,notnull" json:"expires"`
}

// Event is one row in the append-only audit log. Payload holds a JSON
// object of only the changed fields (or relevant context for non-update
// kinds); it's nil when there's nothing to record beyond kind+actor+ts.
// IssueID and Actor are nullable — see the v15 migration comment.
type Event struct {
	bun.BaseModel `bun:"table:events,alias:e" json:"-"`

	ID      int64   `bun:"id,pk,autoincrement" json:"id"`
	IssueID *string `bun:"issue_id" json:"issue_id,omitempty"`
	Actor   *string `bun:"actor" json:"actor,omitempty"`
	Kind    string  `bun:"kind,notnull" json:"kind"`
	Payload *string `bun:"payload" json:"payload,omitempty"`
	TS      int64   `bun:"ts,notnull" json:"ts"`
}

// CronJob is one scheduled invocation. The `Job` field carries a
// JSON-encoded payload whose shape depends on its discriminator —
// see the v9 migration comment for the current vocabulary.
type CronJob struct {
	bun.BaseModel `bun:"table:cron_jobs" json:"-"`

	Name       string  `bun:"name,pk" json:"name"`
	Schedule   string  `bun:"schedule,notnull" json:"schedule"`
	Job        string  `bun:"job,notnull" json:"job"` // JSON; opaque at this layer
	Enabled    bool    `bun:"enabled,notnull" json:"enabled"`
	NextRun    int64   `bun:"next_run,notnull" json:"next_run"`
	LastRun    *int64  `bun:"last_run" json:"last_run,omitempty"`
	LastStatus *string `bun:"last_status" json:"last_status,omitempty"`
	LastOutput *string `bun:"last_output" json:"last_output,omitempty"`
}

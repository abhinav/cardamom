package issue

import (
	"context"
	"regexp"
	"slices"

	"go.abhg.dev/cardamom/internal/must"
)

//go:generate go tool mockgen -destination mocks_test.go -package issue -typed -write_package_comment=false . QueryReader

// QueryReader supplies the coherent board snapshots used by issue queries.
type QueryReader interface {
	// ResolveExternalKey returns the issue identified by one exact producer key.
	ResolveExternalKey(context.Context, string) (ID, error)

	// ListIssues reads issue summaries from one coherent board snapshot.
	ListIssues(context.Context, ListRequest) ([]Summary, error)

	// ListIssuesSnapshot reads issue summaries and their canonical board cursor.
	ListIssuesSnapshot(context.Context, ListRequest) (ListSnapshot, error)

	// ReadIssue reads one issue and its optional inherited context.
	ReadIssue(context.Context, ReadRequest) (View, error)
}

// Queries owns the finite issue read operations shared by CLI commands and
// Connect services.
type Queries struct {
	reader QueryReader
}

// NewQueries constructs issue queries over one board-scoped snapshot reader.
func NewQueries(reader QueryReader) *Queries {
	must.NotBeNilf(reader, "issue QueryReader is required")
	return &Queries{reader: reader}
}

// ResolveExternalKey returns the issue identified by one exact producer key.
func (q *Queries) ResolveExternalKey(ctx context.Context, key string) (ID, error) {
	return q.reader.ResolveExternalKey(ctx, key)
}

// ListIssues returns issue summaries matching one board-scoped query.
func (q *Queries) ListIssues(ctx context.Context, req ListRequest) ([]Summary, error) {
	req, err := normalizeListRequestLabels(req)
	if err != nil {
		return nil, err
	}
	return q.reader.ListIssues(ctx, req)
}

// ListIssuesSnapshot returns issue summaries and the observed board cursor.
func (q *Queries) ListIssuesSnapshot(ctx context.Context, req ListRequest) (ListSnapshot, error) {
	req, err := normalizeListRequestLabels(req)
	if err != nil {
		return ListSnapshot{}, err
	}
	return q.reader.ListIssuesSnapshot(ctx, req)
}

// normalizeListRequestLabels validates and normalizes every label group before
// query readers receive it.
func normalizeListRequestLabels(req ListRequest) (ListRequest, error) {
	req.LabelsAll = slices.Clone(req.LabelsAll)
	req.LabelsAny = slices.Clone(req.LabelsAny)
	req.LabelsNone = slices.Clone(req.LabelsNone)
	groups := [][]string{req.LabelsAll, req.LabelsAny, req.LabelsNone}
	for _, values := range groups {
		for index, value := range values {
			label, err := NewLabel(value)
			if err != nil {
				return ListRequest{}, err
			}
			values[index] = label.String()
		}
	}
	return req, nil
}

// ReadIssue returns one issue and optional inherited current context.
func (q *Queries) ReadIssue(ctx context.Context, req ReadRequest) (View, error) {
	return q.reader.ReadIssue(ctx, req)
}

// ViewReader reads one issue and its optional inherited context from a
// coherent board snapshot.
type ViewReader interface {
	// ReadIssue reads one issue and its optional inherited context.
	ReadIssue(context.Context, ReadRequest) (View, error)
}

// Reader exposes complete board reads to CLI and HTTP callers. Each method
// must materialize its result from one coherent board snapshot.
type Reader interface {
	ViewReader
	// ListIssues reads issue summaries from one coherent board snapshot.
	ListIssues(context.Context, ListRequest) ([]Summary, error)
	// ListReadyIssues reads ready non-routine issues from one coherent board snapshot.
	ListReadyIssues(context.Context, ListReadyRequest) ([]Summary, error)
	// ListBlockedIssues reads blocked non-routine issues from one coherent board snapshot.
	ListBlockedIssues(context.Context, ListBlockedRequest) ([]Summary, error)
	// ListLogEntries reads one issue's log in the requested chronological order.
	ListLogEntries(context.Context, LogListRequest) ([]LogEntry, error)
	// ReadResult reads one issue's durable result.
	ReadResult(context.Context, ResultRequest) (Result, error)
	// ReadChangeCursor reads the board's current change cursor.
	ReadChangeCursor(context.Context) (ChangeCursor, error)
	// ListLabels reads distinct board labels.
	ListLabels(context.Context) ([]string, error)
	// ListActionableCheckpoints reads unresolved checkpoints eligible for action.
	ListActionableCheckpoints(context.Context) ([]CheckpointView, error)
}

// CompletionReader supplies the board values used by command completion.
type CompletionReader interface {
	// ListIssueIDs reads every issue ID in default list order without
	// materializing issue-summary metadata.
	ListIssueIDs(context.Context) ([]string, error)
	// ListLabels reads distinct board labels.
	ListLabels(context.Context) ([]string, error)
	// ListActors reads distinct board actors.
	ListActors(context.Context) ([]string, error)
}

// ListRequest selects and orders issue summaries. Empty fields do not
// constrain their dimension.
type ListRequest struct {
	// UnderID limits results to strict containment descendants of one issue.
	UnderID string

	// Statuses matches any requested presentation status.
	Statuses []string

	// Lifecycles matches any requested lifecycle.
	Lifecycles []string

	// Assignee matches issues with active custody by the named actor.
	Assignee *string

	// Type matches one issue type.
	Type string

	// Types matches any requested issue type.
	Types []string

	// LabelsAll requires every requested label.
	LabelsAll []string

	// LabelsAny requires at least one requested label.
	LabelsAny []string

	// LabelsNone excludes issues carrying any requested label.
	LabelsNone []string

	// NoAssignee matches issues without active custody.
	NoAssignee bool

	// TitleContains performs a case-insensitive title substring match.
	TitleContains string

	// TitleRegexp performs a Go regular-expression match against the title.
	TitleRegexp *regexp.Regexp

	// Sort selects a supported issue field. Empty uses creation order.
	Sort string

	// Reverse reverses the selected order.
	Reverse bool

	// Limit is the maximum result count. Zero returns every match.
	Limit int
}

// ListSnapshot contains one ordered issue collection and the canonical
// board revision observed by the same coherent repository read.
type ListSnapshot struct {
	// Issues contains the filtered and ordered summaries requested by the caller.
	Issues []Summary
	// Total is the number of matching summaries before the request limit.
	Total int
	// Cursor is the scalar board revision visible in the same read scope.
	Cursor ChangeCursor
}

// ListReadyRequest limits ready issue results.
type ListReadyRequest struct{ Limit int }

// ListBlockedRequest limits blocked issue results.
type ListBlockedRequest struct{ Limit int }

// ReadRequest identifies one issue by ID or exact board-scoped producer key
// and optionally requests inherited current context.
// A nil depth omits inherited context; zero requests complete ancestry.
type ReadRequest struct {
	IssueID      string
	Key          string
	ContextDepth *int
}

// ResultRequest identifies one issue for a finite read.
type ResultRequest struct{ IssueID string }

// LogListRequest selects and orders one issue's log entries.
type LogListRequest struct {
	// IssueID identifies the issue whose log entries are requested.
	IssueID string

	// Reverse selects newest-to-oldest durable log order.
	Reverse bool

	// Limit is the maximum result count after ordering. Zero returns every entry.
	Limit int
}

// Summary is one issue projection with labels and blocking state.
type Summary struct {
	Issue   Issue
	Labels  []string
	Blocked bool
}

// View is one issue detail and optional inherited context.
type View struct {
	Detail  Detail
	Context *Context
}

// Context is the selected issue's shared and inherited current context.
type Context struct {
	Board             BoardDescription
	Ancestors         []ContextEntry
	DependencyResults []DependencyResult
	Pins              []PinnedIssue
}

// PinnedIssue is one board pin exposed through inherited issue context.
type PinnedIssue struct {
	// ID is the pinned issue's stable identity.
	ID string

	// Title is the pinned issue's current title.
	Title string
}

// ContextEntry is one ancestor issue and metadata about authored log entries.
type ContextEntry struct {
	Issue      Issue
	LogSummary LogSummary
	// DetailsBytes reports expanded stable material available on demand.
	DetailsBytes int
}

// BoardDescription is the selected board's optional shared description.
type BoardDescription struct{ Description *string }

// Result is one related issue's current durable outcome.
type Result struct {
	IssueID string
	Title   string
	Body    string
}

// DependencyResult combines one prerequisite's current reference and durable
// outcome from the same issue snapshot.
type DependencyResult struct {
	// Issue identifies the prerequisite that produced Body.
	Issue Reference
	// Body is the prerequisite's current durable result.
	Body string
}

// ChangeCursor identifies the scalar revision persisted for a selected board.
type ChangeCursor struct {
	// Revision is zero when the board has no committed issue changes.
	Revision int64
}

// CheckpointView is one unresolved checkpoint and its readiness context.
type CheckpointView struct {
	Issue
	// Blocks identifies the direct dependents waiting on this checkpoint.
	Blocks []Reference
	// Labels are the checkpoint's inert classification values.
	Labels []string
}

// Issue is the caller-facing projection of one durable issue.
type Issue struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	// Lifecycle is the persisted open, closed, or cancelled state.
	Lifecycle string  `json:"lifecycle"`
	Status    string  `json:"status"`
	Priority  int     `json:"priority"`
	Assignee  *string `json:"assignee,omitempty"`
	// ActiveClaim is null when the issue has no current custody.
	ActiveClaim *ActiveClaim `json:"active_claim"`
	Created     int64        `json:"created"`
	Updated     int64        `json:"updated"`
	StartedAt   *int64       `json:"started_at,omitempty"`
	Closed      *int64       `json:"closed,omitempty"`
	Waiting     *Waiting     `json:"waiting,omitempty"`
	Summary     *string      `json:"summary,omitempty"`
	Details     *string      `json:"details,omitempty"`
	State       *string      `json:"state,omitempty"`
	NextAction  *string      `json:"next_action,omitempty"`
	Revision    int64        `json:"revision"`
}

// Waiting is the caller-facing reason and time for waiting status.
type Waiting struct {
	// Reason names the directed continuation, acceptance,
	// or condition required next.
	Reason string `json:"reason"`

	// Since is the waiting transition time as Unix seconds.
	Since int64 `json:"since"`
}

// ActiveClaim is the current execution custody exposed to structured callers.
type ActiveClaim struct {
	// Actor owns custody until release or a terminal lifecycle transition.
	Actor string `json:"actor"`
	// StartedAt is the claim attempt start as Unix seconds.
	StartedAt int64 `json:"started_at"`
}

// LogSummary identifies authored chronology without loading entry bodies.
type LogSummary struct {
	// Count is the number of entries available through ListLogEntries.
	Count int
	// LatestID is the newest log identity, or nil when Count is zero.
	LatestID *LogID
}

// Detail is one issue and its current relationships and records.
type Detail struct {
	Issue Issue
	// Keys contains every exact producer identity associated with Issue
	// in lexical order.
	Keys   []string
	Labels []string
	// State is the complete attributed mutable recovery record.
	State *RecoveryState
	// DependsOn contains every direct prerequisite from the same snapshot.
	DependsOn []Reference
	// Blocks contains every direct dependent from the same snapshot.
	Blocks             []Reference
	LogSummary         LogSummary
	ParentID           *string
	CurrentResult      *Result
	CheckpointDecision *CheckpointDecisionView
	Story              Story
	Blocked            bool
}

// Reference identifies one related issue and the current fields needed
// to present and navigate issue relationships.
type Reference struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Priority int    `json:"priority"`
}

// ContainmentNode is one issue in the selected issue's connected containment
// tree.
type ContainmentNode struct {
	// Reference is the issue shown at this tree position.
	Reference
	// ParentID is nil for the tree's root issue.
	ParentID *string `json:"parent_id"`
}

// Story is the current containment and direct dependency neighborhood
// around one selected issue.
type Story struct {
	// Containment holds every ancestor, the immediate siblings of each issue on
	// the selected path, and every descendant of the selected issue.
	Containment []ContainmentNode `json:"containment"`
	// DependsOn holds the selected issue's open direct prerequisites.
	DependsOn []Reference `json:"depends_on"`
	// Blocks holds the selected issue's open direct dependents.
	Blocks []Reference `json:"blocks"`
}

// LogEntry is one caller-facing attributed chronological record.
type LogEntry struct {
	// ID is the stable log identity.
	ID LogID `json:"id"`

	// IssueID identifies the issue that owns the log entry.
	IssueID string `json:"issue_id"`

	// Kind selects the finite Log payload contract.
	Kind string `json:"kind"`

	// Author is absent when the event has no body attribution.
	Author *string `json:"author,omitempty"`

	// Committer is absent when the preserving actor is unknown.
	Committer *string `json:"committer,omitempty"`

	// Body is the immutable Markdown source.
	Body string `json:"body"`

	// NextAction is the planned transition preserved with a State snapshot.
	NextAction *string `json:"next_action,omitempty"`

	// Created is absent when the event has no recorded time.
	Created *int64 `json:"created,omitempty"`
}

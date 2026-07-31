import type { ReactNode } from "react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ListFilter, Plus, SlidersHorizontal } from "lucide-react";
import { Link, useNavigate } from "react-router";

import type { AttachmentClient } from "./api.ts";
import {
  scopeKey,
  toBoardScopeMessage,
  type BoardScopeSelection,
} from "./board-scope.ts";
import { CollectionSearchControl } from "./collection-search-control.tsx";
import {
  CreateIssueDialog,
  issueCreationBoardId,
} from "./create-issue.tsx";
import {
  IssueStatus,
  IssueType,
  type IssueSummary,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import type { BoardSummary } from "./gen/cardamom/private/v1/project_pb.ts";
import {
  clearIssueFilters,
  issueTypeLabel,
  type BoardViewPreferences,
  type IssueFilters,
  type IssueGrouping,
  type IssueLifecycleFilter,
  type IssueSortPreference,
  type IssueViewPreferences,
  type SortDirectionPreference,
} from "./issue-collection.ts";
import { IssueLabel, type SelectLabel } from "./issue-label.tsx";
import { IssueStatusBadge } from "./issue-status.tsx";
import {
  buildIssueStreams,
  issueLoadControl,
  issuePageIssues,
  useIssueQueries,
  type IssuePageState,
  type IssuePageStream,
} from "./issue-pages.ts";

interface SharedRouteProps {
  attachmentClient: AttachmentClient;
  boards: readonly BoardSummary[];
  canMutateServer: boolean;
  selection: BoardScopeSelection;
  actor: string;
  selectLabel: SelectLabel;
}

interface BoardRouteProps extends SharedRouteProps {
  view: BoardViewPreferences;
  updateView: (view: BoardViewPreferences) => void;
}

export function BoardRoute({ view, updateView, ...shared }: BoardRouteProps) {
  return (
    <IssueCollectionRoute
      {...shared}
      mode="board"
      view={view}
      grouping={view.grouping}
      showEmptyColumns={view.showEmptyColumns}
      updateGrouping={(grouping) => updateView({ ...view, grouping })}
      updateView={(next) => updateView({ ...view, ...next })}
    />
  );
}

interface ListRouteProps extends SharedRouteProps {
  view: IssueViewPreferences;
  updateView: (view: IssueViewPreferences) => void;
}

export function ListRoute({ view, updateView, ...shared }: ListRouteProps) {
  return (
    <IssueCollectionRoute
      {...shared}
      mode="list"
      view={view}
      updateView={updateView}
    />
  );
}

interface IssueCollectionRouteProps extends SharedRouteProps {
  mode: "board" | "list";
  view: IssueViewPreferences;
  grouping?: IssueGrouping;
  showEmptyColumns?: boolean;
  updateGrouping?: (grouping: IssueGrouping) => void;
  updateView: (view: IssueViewPreferences) => void;
}

function IssueCollectionRoute(props: IssueCollectionRouteProps) {
  const selectionIdentity = scopeKey(props.selection);
  const scope = useMemo(
    () => toBoardScopeMessage(props.selection),
    [selectionIdentity],
  );
  if (scope === undefined) {
    return (
      <section
        className="route-state"
        aria-label={props.mode === "board" ? "Issue board" : "Issue list"}
      >
        <p>Select a board to load issues.</p>
      </section>
    );
  }
  return <LoadedIssueCollection {...props} scope={scope} />;
}

interface LoadedIssueCollectionProps extends IssueCollectionRouteProps {
  scope: NonNullable<ReturnType<typeof toBoardScopeMessage>>;
}

function LoadedIssueCollection(props: LoadedIssueCollectionProps) {
  const viewKey = JSON.stringify(props.view);
  const grouping = props.grouping ?? "status";
  const streams = useMemo(
    () => buildIssueStreams(props.scope, props.mode, props.view, grouping),
    [props.scope, props.mode, grouping, viewKey],
  );
  const pages = useIssueQueries(streams);
  const initialLoad = !pages.state.hasCompletedInitialLoad &&
    pages.state.streams.length > 0 &&
    pages.state.streams.every(
      (stream) =>
        stream.pageCount === 0 &&
        (stream.status === "idle" || stream.status === "loading"),
    );
  if (initialLoad) {
    return (
      <RouteState
        title={props.mode === "board" ? "Board" : "List"}
        message="Loading issues"
      />
    );
  }
  const listStream = pages.state.streams[0];
  if (
    props.mode === "list" &&
    listStream?.status === "error" &&
    listStream.pageCount === 0
  ) {
    return (
      <RouteState
        title="List"
        message="Issues could not be loaded"
        detail={listStream.error?.message}
      >
        <button type="button" onClick={() => pages.loadMore(listStream.key)}>
          Retry
        </button>
      </RouteState>
    );
  }

  return (
    <IssueCollectionSurface
      {...props}
      pages={pages.state}
      loadMore={pages.loadMore}
    />
  );
}

interface IssueCollectionSurfaceProps extends IssueCollectionRouteProps {
  pages: IssuePageState;
  loadMore: (key: string) => void;
}

function IssueCollectionSurface(props: IssueCollectionSurfaceProps) {
  const navigate = useNavigate();
  const [creating, setCreating] = useState(false);
  const creationBoardId = issueCreationBoardId(props.selection);
  const canCreateIssue = props.canMutateServer && creationBoardId !== undefined;
  const loadedIssues = issuePageIssues(props.pages);
  const totalIssues = props.pages.streams.reduce(
    (total, stream) => total + stream.totalCount,
    0,
  );

  return (
    <section
      className={`issue-view issue-view-${props.mode}`}
      aria-label={props.mode === "board" ? "Issue board" : "Issue list"}
    >
      <header className="issue-view-header">
        <IssueControls
          mode={props.mode}
          view={props.view}
          grouping={props.grouping}
          updateGrouping={props.updateGrouping}
          updateView={props.updateView}
          loadedCount={loadedIssues.length}
          totalCount={totalIssues}
          readOnly={props.selection.kind === "all"}
          createIssue={canCreateIssue ? () => setCreating(true) : undefined}
        />
      </header>

      {props.mode === "board" ? (
        <KanbanBoard
          boards={props.boards}
          grouping={props.grouping ?? "status"}
          streams={props.pages.streams}
          loadMore={props.loadMore}
          selectLabel={props.selectLabel}
          showBoard={props.selection.kind === "all"}
          canCreateIssue={canCreateIssue}
          showCreationGuidance={props.canMutateServer}
          showEmptyColumns={props.showEmptyColumns}
        />
      ) : (
        <IssueList
          boards={props.boards}
          stream={props.pages.streams[0]}
          loadMore={props.loadMore}
          selectLabel={props.selectLabel}
          showBoard={props.selection.kind === "all"}
        />
      )}

      {creating && creationBoardId !== undefined && (
        <CreateIssueDialog
          actor={props.actor}
          attachmentClient={props.attachmentClient}
          boardId={creationBoardId}
          onCreated={(issueId) => {
            setCreating(false);
            navigate(`/issues/${encodeURIComponent(issueId)}`);
          }}
          onDismiss={() => setCreating(false)}
        />
      )}
    </section>
  );
}

interface IssueControlsProps {
  mode: "board" | "list";
  view: IssueViewPreferences;
  loadedCount?: number;
  totalCount?: number;
  readOnly?: boolean;
  grouping?: IssueGrouping;
  updateGrouping?: (grouping: IssueGrouping) => void;
  updateView: (view: IssueViewPreferences) => void;
  createIssue?: () => void;
}

export function IssueControls(props: IssueControlsProps) {
  const setFilters = (filters: IssueFilters) =>
    props.updateView({ ...props.view, filters });
  const defaultLifecycle = props.mode === "board" ? "current" : "all";
  const activeFilterCount = [
    props.view.filters.lifecycle !== defaultLifecycle,
    props.view.filters.status !== "all",
    props.view.filters.type !== "all",
    props.view.filters.actor.trim() !== "",
    props.view.filters.label.trim() !== "",
  ].filter(Boolean).length;
  const canClearFilters =
    activeFilterCount > 0 || props.view.filters.query.trim() !== "";
  const filterLabel = activeFilterCount === 0
    ? "Filters"
    : `Filters, ${activeFilterCount} active`;
  return (
    <div
      className="issue-controls"
      role="toolbar"
      aria-label="Issue collection controls"
    >
      {props.loadedCount !== undefined && props.totalCount !== undefined && (
        <span className="collection-count" aria-live="polite">
          {props.loadedCount}/{props.totalCount} issues
        </span>
      )}
      <CollectionSearchControl
        filters={props.view.filters}
        setFilters={setFilters}
      />
      <details className="collection-options" name="issue-collection-options">
        <summary className="icon-control" aria-label={filterLabel} title="Filters">
          <ListFilter aria-hidden="true" />
          {activeFilterCount > 0 && (
            <span className="control-count" aria-hidden="true">
              {activeFilterCount}
            </span>
          )}
        </summary>
        <div className="collection-options-panel">
          <FilterFields filters={props.view.filters} setFilters={setFilters} />
          <div className="filter-actions">
            <button
              type="button"
              className="secondary-button"
              disabled={!canClearFilters}
              onClick={() => setFilters(clearIssueFilters(props.mode))}
            >
              Clear filters
            </button>
          </div>
        </div>
      </details>
      <details className="collection-options" name="issue-collection-options">
        <summary
          className="icon-control"
          aria-label="View options"
          title="View options"
        >
          <SlidersHorizontal aria-hidden="true" />
        </summary>
        <div className="collection-options-panel view-options">
          {props.mode === "board" && props.updateGrouping !== undefined && (
            <label>
              <span>Group</span>
              <select
                value={props.grouping}
                onChange={(event) =>
                  props.updateGrouping?.(event.currentTarget.value as IssueGrouping)
                }
              >
                <option value="status">Status</option>
                <option value="type">Type</option>
              </select>
            </label>
          )}
          <label>
            <span>Sort</span>
            <select
              value={props.view.sort}
              onChange={(event) =>
                props.updateView({
                  ...props.view,
                  sort: event.currentTarget.value as IssueSortPreference,
                })
              }
            >
              {props.mode === "board" && <option value="natural">Natural</option>}
              <option value="priority">Priority</option>
              <option value="updated">Updated</option>
              <option value="created">Created</option>
              <option value="title">Title</option>
            </select>
          </label>
          {props.view.sort !== "natural" && (
            <label>
              <span>Direction</span>
              <select
                value={props.view.direction}
                onChange={(event) =>
                  props.updateView({
                    ...props.view,
                    direction: event.currentTarget.value as SortDirectionPreference,
                  })
                }
              >
                <option value="ascending">Ascending</option>
                <option value="descending">Descending</option>
              </select>
            </label>
          )}
        </div>
      </details>
      {props.createIssue !== undefined && (
        <button
          type="button"
          className="icon-control create-issue-control"
          aria-label="Create issue"
          title="Create issue"
          onClick={props.createIssue}
        >
          <Plus aria-hidden="true" />
        </button>
      )}
      {props.readOnly && (
        <span className="read-only-badge">All boards · read-only</span>
      )}
    </div>
  );
}

function FilterFields({
  filters,
  setFilters,
}: {
  filters: IssueFilters;
  setFilters: (filters: IssueFilters) => void;
}) {
  return (
    <div className="filter-fields">
      <label>
        <span>Lifecycle</span>
        <select
          value={filters.lifecycle}
          onChange={(event) =>
            setFilters({
              ...filters,
              lifecycle: event.currentTarget.value as IssueLifecycleFilter,
            })
          }
        >
          <option value="current">Open + closed</option>
          <option value="open">Open</option>
          <option value="closed">Closed</option>
          <option value="cancelled">Cancelled</option>
          <option value="all">All</option>
        </select>
      </label>
      <label>
        <span>Status</span>
        <select
          value={filters.status}
          onChange={(event) =>
            setFilters({
              ...filters,
              status: enumFilter(event.currentTarget.value) as IssueStatus | "all",
            })
          }
        >
          <option value="all">All</option>
          <option value={IssueStatus.READY}>Ready</option>
          <option value={IssueStatus.BLOCKED}>Blocked</option>
          <option value={IssueStatus.IN_PROGRESS}>In progress</option>
          <option value={IssueStatus.WAITING}>Waiting</option>
          <option value={IssueStatus.CLOSED}>Closed</option>
          <option value={IssueStatus.CANCELLED}>Cancelled</option>
        </select>
      </label>
      <label>
        <span>Type</span>
        <select
          value={filters.type}
          onChange={(event) =>
            setFilters({
              ...filters,
              type: enumFilter(event.currentTarget.value) as IssueType | "all",
            })
          }
        >
          <option value="all">All</option>
          <option value={IssueType.WORKSTREAM}>Workstream</option>
          <option value={IssueType.TASK}>Task</option>
          <option value={IssueType.CHECKPOINT}>Checkpoint</option>
          <option value={IssueType.ROUTINE}>Routine</option>
        </select>
      </label>
      <label>
        <span>Actor</span>
        <input
          type="search"
          value={filters.actor}
          placeholder="Any actor"
          onInput={(event) => setFilters({ ...filters, actor: event.currentTarget.value })}
        />
      </label>
      <label>
        <span>Label</span>
        <input
          type="search"
          value={filters.label}
          placeholder="Any label"
          onInput={(event) => setFilters({ ...filters, label: event.currentTarget.value })}
        />
      </label>
    </div>
  );
}

export function KanbanBoard({
  boards,
  canCreateIssue = true,
  grouping,
  streams,
  loadMore,
  selectLabel,
  showBoard,
  showCreationGuidance = true,
  showEmptyColumns = false,
}: {
  boards: readonly BoardSummary[];
  canCreateIssue?: boolean;
  grouping: IssueGrouping;
  streams: readonly IssuePageStream[];
  loadMore: (key: string) => void;
  selectLabel: SelectLabel;
  showBoard: boolean;
  showCreationGuidance?: boolean;
  showEmptyColumns?: boolean;
}) {
  const visibleStreams = showEmptyColumns
    ? streams
    : streams.filter((stream) => stream.totalCount > 0);
  if (visibleStreams.length === 0) {
    return (
      <div className="kanban-empty" role="status">
        {canCreateIssue
          ? "No issues here. Create a new issue to get started."
          : showCreationGuidance && showBoard
            ? "No issues here. Select a board to create a new issue."
            : "No issues here."}
      </div>
    );
  }
  return (
    <div className="kanban-scroll" tabIndex={0} aria-label="Issue board columns">
      <div className="kanban-board">
        {visibleStreams.map((stream) => (
          <section className="kanban-column" key={stream.key} aria-labelledby={`${stream.key}-title`}>
            <header>
              <h2 id={`${stream.key}-title`}>{stream.label}</h2>
              <span>{stream.issues.length}/{stream.totalCount}</span>
            </header>
            <div
              className="kanban-cards"
              tabIndex={0}
              aria-label={`${stream.label} issues`}
            >
              {stream.issues.length === 0 &&
              stream.status === "ready" ? (
                <p className="empty-column">No issues</p>
              ) : (
                stream.issues.map((issue) => (
                  <IssueCard
                    boards={boards}
                    issue={issue}
                    key={issue.id}
                    selectLabel={selectLabel}
                    showBoard={showBoard}
                    showStatus={grouping !== "status"}
                  />
                ))
              )}
              <IssueStreamLoad stream={stream} loadMore={loadMore} />
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}

function IssueCard({
  boards,
  issue,
  selectLabel,
  showBoard,
  showStatus,
}: {
  boards: readonly BoardSummary[];
  issue: IssueSummary;
  selectLabel: SelectLabel;
  showBoard: boolean;
  showStatus: boolean;
}) {
  return (
    <article className="issue-card" data-status={issue.status}>
      <div className="issue-card-topline">
        <Link
          className="issue-id"
          to={`/issues/${encodeURIComponent(issue.id)}`}
        >
          {issue.id}
        </Link>
        <span className="issue-priority">P{issue.priority}</span>
      </div>
      <h3>
        <Link to={`/issues/${encodeURIComponent(issue.id)}`}>
          {issue.title}
        </Link>
      </h3>
      <IssueMetadata
        boards={boards}
        issue={issue}
        selectLabel={selectLabel}
        showBoard={showBoard}
        showStatus={showStatus}
      />
    </article>
  );
}

function IssueList({
  boards,
  stream,
  loadMore,
  selectLabel,
  showBoard,
}: {
  boards: readonly BoardSummary[];
  stream: IssuePageStream | undefined;
  loadMore: (key: string) => void;
  selectLabel: SelectLabel;
  showBoard: boolean;
}) {
  if (stream === undefined ||
    (stream.issues.length === 0 && stream.status === "ready")) {
    return <EmptyCollection />;
  }
  const issues = stream.issues;
  return (
    <div className="issue-list-scroll" tabIndex={0} aria-label="Issues">
      <div className="issue-table-wrap">
        <table className="issue-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>Title</th>
              {showBoard && <th>Board</th>}
              <th>Status</th>
              <th>Type</th>
              <th>Priority</th>
              <th>Actor</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {issues.map((issue) => (
              <tr key={issue.id}>
                <td>
                  <Link
                    className="issue-id"
                    to={`/issues/${encodeURIComponent(issue.id)}`}
                  >
                    {issue.id}
                  </Link>
                </td>
                <td className="issue-title-cell">
                  <Link to={`/issues/${encodeURIComponent(issue.id)}`}>
                    {issue.title}
                  </Link>
                  {issue.labels.length > 0 && (
                    <span className="table-labels">
                      {issue.labels.map((label) => (
                        <IssueLabel key={label} label={label} select={selectLabel} />
                      ))}
                    </span>
                  )}
                </td>
                {showBoard && <td>{boardName(boards, issue.boardId)}</td>}
                <td><IssueStatusBadge status={issue.status} /></td>
                <td>{issueTypeLabel(issue.type)}</td>
                <td>{issue.priority}</td>
                <td>{issue.activeClaim?.actor ?? "—"}</td>
                <td><IssueTime issue={issue} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ul className="issue-records">
        {issues.map((issue) => (
          <li key={issue.id}>
            <div className="issue-record-heading">
              <Link
                className="issue-id"
                to={`/issues/${encodeURIComponent(issue.id)}`}
              >
                {issue.id}
              </Link>
              <IssueStatusBadge status={issue.status} />
              <span className="issue-priority">P{issue.priority}</span>
            </div>
            <Link
              className="issue-record-title"
              to={`/issues/${encodeURIComponent(issue.id)}`}
            >
              {issue.title}
            </Link>
            <IssueMetadata
              boards={boards}
              issue={issue}
              selectLabel={selectLabel}
              showBoard={showBoard}
              showStatus={false}
            />
          </li>
        ))}
      </ul>
      <IssueStreamLoad stream={stream} loadMore={loadMore} />
    </div>
  );
}

function IssueStreamLoad({
  stream,
  loadMore,
}: {
  stream: IssuePageStream;
  loadMore: (key: string) => void;
}) {
  const control = issueLoadControl(stream);
  const sentinel = useRef<HTMLDivElement>(null);
  const load = useCallback(() => loadMore(stream.key), [loadMore, stream.key]);

  useEffect(() => {
    const element = sentinel.current;
    if (
      element === null ||
      control.kind !== "load" ||
      typeof IntersectionObserver === "undefined"
    ) {
      return;
    }
    const columnRoot = element.parentElement?.classList.contains("kanban-cards")
      ? element.parentElement
      : null;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        load();
      }
    }, {
      root: columnRoot,
      rootMargin: columnRoot === null
        ? "0px 0px 600px 0px"
        : "0px 0px 300px 0px",
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [control.kind, load]);

  if (control.kind === "exhausted" && stream.issues.length === 0) {
    return null;
  }
  return (
    <div className="collection-load-state" ref={sentinel} aria-live="polite">
      {control.kind === "load" && (
        <button type="button" aria-label={control.label} onClick={load}>
          Load more
        </button>
      )}
      {control.kind === "loading" && (
        <span role="status">{control.label}</span>
      )}
      {control.kind === "retry" && (
        <>
          <span role="alert">{control.message}</span>
          <button type="button" aria-label={control.label} onClick={load}>
            Retry
          </button>
        </>
      )}
      {control.kind === "exhausted" && <span>{control.label}</span>}
    </div>
  );
}

function IssueMetadata({
  boards,
  issue,
  selectLabel,
  showBoard,
  showStatus,
}: {
  boards: readonly BoardSummary[];
  issue: IssueSummary;
  selectLabel: SelectLabel;
  showBoard: boolean;
  showStatus: boolean;
}) {
  return (
    <div className="issue-metadata">
      {showStatus && <IssueStatusBadge status={issue.status} />}
      <span>{issueTypeLabel(issue.type)}</span>
      {issue.activeClaim !== undefined && <span>{issue.activeClaim.actor}</span>}
      {showBoard && <span>{boardName(boards, issue.boardId)}</span>}
      <IssueTime issue={issue} compact labeled />
      {issue.labels.map((label) => (
        <IssueLabel key={label} label={label} select={selectLabel} />
      ))}
    </div>
  );
}

function IssueTime({
  issue,
  compact = false,
  labeled = false,
}: {
  issue: IssueSummary;
  compact?: boolean;
  labeled?: boolean;
}) {
  const timestamp = issue.updatedAt ?? issue.createdAt;
  if (timestamp === undefined) {
    return <>—</>;
  }
  const date = new Date(Number(timestamp.seconds) * 1000 + timestamp.nanos / 1_000_000);
  const label = formatIssueTime(
    issue,
    compact ? compactDateTime : tableDateTime,
  );
  const time = <time dateTime={date.toISOString()}>{label}</time>;
  return labeled ? <span className="issue-updated">Updated {time}</span> : time;
}

export function formatIssueTime(
  issue: IssueSummary,
  formatter: Intl.DateTimeFormat = tableDateTime,
): string | undefined {
  const timestamp = issue.updatedAt ?? issue.createdAt;
  if (timestamp === undefined) {
    return undefined;
  }
  const date = new Date(Number(timestamp.seconds) * 1000 + timestamp.nanos / 1_000_000);
  return formatter.format(date);
}

function RouteState({
  children,
  detail,
  message,
  title,
}: {
  children?: ReactNode;
  detail?: string;
  message: string;
  title: string;
}) {
  return (
    <section className="route-state" aria-label={title}>
      <div role={detail === undefined ? "status" : "alert"}>
        <p>{message}</p>
        {detail !== undefined && <p>{detail}</p>}
      </div>
      {children}
    </section>
  );
}

function EmptyCollection() {
  return <p className="empty-collection">No issues match these filters.</p>;
}

function enumFilter(value: string): number | "all" {
  return value === "all" ? value : Number(value);
}

function boardName(boards: readonly BoardSummary[], boardId: string): string {
  return boards.find((board) => board.id === boardId)?.name ?? boardId;
}

const compactDateTime = new Intl.DateTimeFormat(undefined, {
  month: "short",
  day: "numeric",
  hour: "numeric",
  minute: "2-digit",
});

const tableDateTime = new Intl.DateTimeFormat(undefined, {
  dateStyle: "medium",
  timeStyle: "short",
});

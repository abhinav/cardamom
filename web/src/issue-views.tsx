import { useTransport } from "@connectrpc/connect-query";
import { useQuery } from "@tanstack/react-query";
import { ListFilter, Plus, SlidersHorizontal } from "lucide-react";
import type { ReactNode } from "react";
import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

import type { AttachmentClient } from "./api.ts";
import {
  issuePath,
  scopeKey,
  toBoardScopeMessage,
  type BoardScopeSelection,
} from "./board-scope.ts";
import { CollectionSearchControl } from "./collection-search-control.tsx";
import type { IssueFilterNavigation } from "./collection-route.ts";
import {
  CreateIssueDialog,
  issueCreationBoardId,
} from "./create-issue.tsx";
import {
  IssueStatus,
  IssueService,
  IssueType,
  type RelatedIssue,
  type IssueSummary,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import type { BoardSummary } from "./gen/cardamom/private/v1/project_pb.ts";
import type { Project } from "./gen/cardamom/private/v1/project_pb.ts";
import type { SourceCatalogEntry } from "./gen/cardamom/private/v1/source_pb.ts";
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
import { IssueWaitingReason } from "./issue-waiting.tsx";
import { issueIdentity, visibleIssueProvenance } from "./provenance.ts";
import { unaryScopeQueryOptions } from "./query-runtime.ts";
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
  aggregateMode: boolean;
  boards: readonly BoardSummary[];
  projects: readonly Project[];
  sources?: readonly SourceCatalogEntry[];
  canMutateServer: boolean;
  selection: BoardScopeSelection;
  actor: string;
  selectLabel: SelectLabel;
}

interface BoardRouteProps extends SharedRouteProps {
  view: BoardViewPreferences;
  updateFilters: (
    filters: IssueFilters,
    navigation: IssueFilterNavigation,
  ) => void;
  updateView: (view: BoardViewPreferences) => void;
}

export function BoardRoute({
  view,
  updateFilters,
  updateView,
  ...shared
}: BoardRouteProps) {
  return (
    <IssueCollectionRoute
      {...shared}
      mode="board"
      view={view}
      grouping={view.grouping}
      showEmptyColumns={view.showEmptyColumns}
      updateFilters={updateFilters}
      updateGrouping={(grouping) => updateView({ ...view, grouping })}
      updateView={(next) => updateView({ ...view, ...next })}
    />
  );
}

interface ListRouteProps extends SharedRouteProps {
  view: IssueViewPreferences;
  updateFilters: (
    filters: IssueFilters,
    navigation: IssueFilterNavigation,
  ) => void;
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
  updateFilters: (
    filters: IssueFilters,
    navigation: IssueFilterNavigation,
  ) => void;
  updateGrouping?: (grouping: IssueGrouping) => void;
  updateView: (view: IssueViewPreferences) => void;
}

function IssueCollectionRoute(props: IssueCollectionRouteProps) {
  const selectionIdentity = scopeKey(props.selection);
  const scope = useMemo(
    () =>
      toBoardScopeMessage(props.selection, {
        aggregate: props.aggregateMode,
        boards: props.boards,
        projects: props.projects,
        sources: props.sources ?? [],
      }),
    [
      props.aggregateMode,
      props.boards,
      props.projects,
      props.sources,
      selectionIdentity,
    ],
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
  const initialPinsBoardId = boardPinsBoardId(props.mode, props.selection);
  if (initialLoad) {
    return (
      <section
        className={`issue-view issue-view-${props.mode}`}
        aria-label={props.mode === "board" ? "Issue board" : "Issue list"}
      >
        {props.mode === "board" && (
          <div className="kanban-surface">
            {initialPinsBoardId !== undefined && (
              <BoardPins boardId={initialPinsBoardId} />
            )}
            <RouteState title="Board" message="Loading issues" />
          </div>
        )}
        {props.mode === "list" && (
          <RouteState title="List" message="Loading issues" />
        )}
      </section>
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
        <Button type="button" onClick={() => pages.loadMore(listStream.key)}>
          Retry
        </Button>
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
  const pinsBoardId = boardPinsBoardId(props.mode, props.selection);
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
        <AggregateReadStatus streams={props.pages.streams} />
        <IssueControls
          mode={props.mode}
          view={props.view}
          grouping={props.grouping}
          updateFilters={props.updateFilters}
          updateGrouping={props.updateGrouping}
          updateView={props.updateView}
          loadedCount={loadedIssues.length}
          totalCount={totalIssues}
          readOnly={props.selection.kind === "all"}
          createIssue={canCreateIssue ? () => setCreating(true) : undefined}
        />
      </header>

      {props.mode === "board" ? (
        <div className="kanban-surface">
          {pinsBoardId !== undefined && <BoardPins boardId={pinsBoardId} />}
          <KanbanBoard
            boards={props.boards}
            projects={props.projects}
            grouping={props.grouping ?? "status"}
            streams={props.pages.streams}
            loadMore={props.loadMore}
            selectLabel={props.selectLabel}
            showBoard={props.selection.kind === "all"}
            scope={props.selection}
            canCreateIssue={canCreateIssue}
            showCreationGuidance={props.canMutateServer}
            showEmptyColumns={props.showEmptyColumns}
          />
        </div>
      ) : (
        <IssueList
          boards={props.boards}
          projects={props.projects}
          stream={props.pages.streams[0]}
          loadMore={props.loadMore}
          selectLabel={props.selectLabel}
          showBoard={props.selection.kind === "all"}
          scope={props.selection}
        />
      )}

      {creating && creationBoardId !== undefined && (
        <CreateIssueDialog
          actor={props.actor}
          attachmentClient={props.attachmentClient}
          boardId={creationBoardId}
          onCreated={(issueId) => {
            setCreating(false);
            navigate(issuePath(creationBoardId, issueId));
          }}
          onDismiss={() => setCreating(false)}
        />
      )}
    </section>
  );
}

/** boardPinsBoardId selects the one collection scope that owns a pin row. */
export function boardPinsBoardId(
  mode: "board" | "list",
  selection: BoardScopeSelection,
): string | undefined {
  return mode === "board" && selection.kind === "board"
    ? selection.boardId
    : undefined;
}

function BoardPins({ boardId }: { boardId: string }) {
  const transport = useTransport();
  const request = useQuery({
    ...unaryScopeQueryOptions(
      IssueService.method.listBoardPins,
      { boardId },
      transport,
    ),
    select: (response) => response.issues,
  });
  if (request.data === undefined && request.isError) {
    return (
      <BoardPinsError retry={() => void request.refetch()} />
    );
  }
  return (
    <>
      <BoardPinCarousel boardId={boardId} issues={request.data ?? []} />
      {request.data !== undefined && request.isError && (
        <BoardPinsError retry={() => void request.refetch()} refresh />
      )}
    </>
  );
}

function BoardPinsError({
  refresh = false,
  retry,
}: {
  refresh?: boolean;
  retry: () => void;
}) {
  return (
    <p className="board-pins-error" role="alert">
      {refresh
        ? "Pinned issues could not be refreshed."
        : "Pinned issues could not be loaded."}{" "}
      <Button type="button" variant="link" className="link-button" onClick={retry}>
        Retry
      </Button>
    </p>
  );
}

/** BoardPinCarousel renders one board's nonempty ordered pin collection. */
export function BoardPinCarousel({
  boardId,
  issues,
}: {
  boardId: string;
  issues: readonly RelatedIssue[];
}) {
  if (issues.length === 0) {
    return null;
  }
  return (
    <section className="board-pins" aria-labelledby="board-pins-title">
      <header>
        <h2 id="board-pins-title">Pinned</h2>
        <span>{issues.length}</span>
      </header>
      <div className="board-pins-scroll" tabIndex={0} aria-label="Pinned issues">
        <div className="board-pins-track">
          {issues.map((issue) => (
            <article className="board-pin-card" key={issue.id}>
              <div className="board-pin-card-heading">
                <Link className="issue-id" to={issuePath(boardId, issue.id)}>
                  {issue.id}
                </Link>
                <IssueStatusBadge status={issue.status} />
              </div>
              <h3>
                <Link title={issue.title} to={issuePath(boardId, issue.id)}>
                  {issue.title}
                </Link>
              </h3>
            </article>
          ))}
        </div>
      </div>
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
  updateFilters: (
    filters: IssueFilters,
    navigation: IssueFilterNavigation,
  ) => void;
  updateGrouping?: (grouping: IssueGrouping) => void;
  updateView: (view: IssueViewPreferences) => void;
  createIssue?: () => void;
}

export function IssueControls(props: IssueControlsProps) {
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
        setFilters={props.updateFilters}
      />
      <Popover>
        <PopoverTrigger
          render={
            <Button
              aria-label={filterLabel}
              title="Filters"
              variant="outline"
              size="icon"
            />
          }
        >
          <ListFilter aria-hidden="true" />
          {activeFilterCount > 0 && (
            <span className="control-count" aria-hidden="true">
              {activeFilterCount}
            </span>
          )}
        </PopoverTrigger>
        <PopoverContent
          align="end"
          className="w-[min(30rem,calc(100vw-2rem))] gap-4 p-4"
        >
          <FilterFields
            filters={props.view.filters}
            setFilters={(filters) => props.updateFilters(filters, "push")}
          />
          <div className="filter-actions">
            <Button
              type="button"
              variant="outline"
              disabled={!canClearFilters}
              onClick={() =>
                props.updateFilters(clearIssueFilters(props.mode), "push")}
            >
              Clear filters
            </Button>
          </div>
        </PopoverContent>
      </Popover>
      <Popover>
        <PopoverTrigger
          render={
            <Button
              aria-label="View options"
              title="View options"
              variant="outline"
              size="icon"
            />
          }
        >
          <SlidersHorizontal aria-hidden="true" />
        </PopoverTrigger>
        <PopoverContent align="end" className="w-64 gap-4 p-4">
          {props.mode === "board" && props.updateGrouping !== undefined && (
            <IssueSelectField
              label="Group"
              value={props.grouping ?? "status"}
              onValueChange={(value) =>
                props.updateGrouping?.(value as IssueGrouping)}
            >
              <SelectItem value="status">Status</SelectItem>
              <SelectItem value="type">Type</SelectItem>
            </IssueSelectField>
          )}
          <IssueSelectField
            label="Sort"
            value={props.view.sort}
            onValueChange={(value) =>
              props.updateView({
                ...props.view,
                sort: value as IssueSortPreference,
              })}
          >
            {props.mode === "board" && (
              <SelectItem value="natural">Natural</SelectItem>
            )}
            <SelectItem value="priority">Priority</SelectItem>
            <SelectItem value="updated">Updated</SelectItem>
            <SelectItem value="created">Created</SelectItem>
            <SelectItem value="title">Title</SelectItem>
          </IssueSelectField>
          {props.view.sort !== "natural" && (
            <IssueSelectField
              label="Direction"
              value={props.view.direction}
              onValueChange={(value) =>
                props.updateView({
                  ...props.view,
                  direction: value as SortDirectionPreference,
                })}
            >
              <SelectItem value="ascending">Ascending</SelectItem>
              <SelectItem value="descending">Descending</SelectItem>
            </IssueSelectField>
          )}
        </PopoverContent>
      </Popover>
      {props.createIssue !== undefined && (
        <Button
          type="button"
          size="icon"
          aria-label="Create issue"
          title="Create issue"
          onClick={props.createIssue}
        >
          <Plus aria-hidden="true" />
        </Button>
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
      <IssueSelectField
        label="Lifecycle"
        value={filters.lifecycle}
        onValueChange={(value) =>
          setFilters({
            ...filters,
            lifecycle: value as IssueLifecycleFilter,
          })}
      >
        <SelectItem value="current">Open + closed</SelectItem>
        <SelectItem value="open">Open</SelectItem>
        <SelectItem value="closed">Closed</SelectItem>
        <SelectItem value="cancelled">Cancelled</SelectItem>
        <SelectItem value="all">All</SelectItem>
      </IssueSelectField>
      <IssueSelectField
        label="Status"
        value={String(filters.status)}
        onValueChange={(value) =>
          setFilters({
            ...filters,
            status: enumFilter(value) as IssueStatus | "all",
          })}
      >
        <SelectItem value="all">All</SelectItem>
        <SelectItem value={String(IssueStatus.READY)}>Ready</SelectItem>
        <SelectItem value={String(IssueStatus.BLOCKED)}>Blocked</SelectItem>
        <SelectItem value={String(IssueStatus.IN_PROGRESS)}>In progress</SelectItem>
        <SelectItem value={String(IssueStatus.WAITING)}>Waiting</SelectItem>
        <SelectItem value={String(IssueStatus.CLOSED)}>Closed</SelectItem>
        <SelectItem value={String(IssueStatus.CANCELLED)}>Cancelled</SelectItem>
      </IssueSelectField>
      <IssueSelectField
        label="Type"
        value={String(filters.type)}
        onValueChange={(value) =>
          setFilters({
            ...filters,
            type: enumFilter(value) as IssueType | "all",
          })}
      >
        <SelectItem value="all">All</SelectItem>
        <SelectItem value={String(IssueType.WORKSTREAM)}>Workstream</SelectItem>
        <SelectItem value={String(IssueType.TASK)}>Task</SelectItem>
        <SelectItem value={String(IssueType.CHECKPOINT)}>Checkpoint</SelectItem>
        <SelectItem value={String(IssueType.ROUTINE)}>Routine</SelectItem>
      </IssueSelectField>
      <IssueTextFilterField
        label="Actor"
        value={filters.actor}
        placeholder="Any actor"
        onValueChange={(actor) => setFilters({ ...filters, actor })}
      />
      <IssueTextFilterField
        label="Label"
        value={filters.label}
        placeholder="Any label"
        onValueChange={(label) => setFilters({ ...filters, label })}
      />
    </div>
  );
}

function IssueSelectField({
  children,
  label,
  onValueChange,
  value,
}: {
  children: ReactNode;
  label: string;
  onValueChange: (value: string) => void;
  value: string;
}) {
  const id = useId();
  return (
    <div className="filter-field">
      <Label htmlFor={id}>{label}</Label>
      <Select
        value={value}
        onValueChange={(nextValue) => {
          if (nextValue !== null) {
            onValueChange(nextValue);
          }
        }}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>{children}</SelectContent>
      </Select>
    </div>
  );
}

function IssueTextFilterField({
  label,
  onValueChange,
  placeholder,
  value,
}: {
  label: string;
  onValueChange: (value: string) => void;
  placeholder: string;
  value: string;
}) {
  const id = useId();
  return (
    <div className="filter-field">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type="search"
        value={value}
        placeholder={placeholder}
        onChange={(event) => onValueChange(event.currentTarget.value)}
      />
    </div>
  );
}

export function KanbanBoard({
  boards,
  projects = [],
  canCreateIssue = true,
  grouping,
  streams,
  loadMore,
  selectLabel,
  showBoard,
  scope = { kind: "board", boardId: "" },
  showCreationGuidance = true,
  showEmptyColumns = false,
}: {
  boards: readonly BoardSummary[];
  projects?: readonly Project[];
  canCreateIssue?: boolean;
  grouping: IssueGrouping;
  streams: readonly IssuePageStream[];
  loadMore: (key: string) => void;
  selectLabel: SelectLabel;
  showBoard: boolean;
  scope?: BoardScopeSelection;
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
                    projects={projects}
                    issue={issue}
                    key={issueIdentity(issue)}
                    selectLabel={selectLabel}
                    showBoard={showBoard}
                    scope={scope}
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
  projects,
  issue,
  selectLabel,
  showBoard,
  scope,
  showStatus,
}: {
  boards: readonly BoardSummary[];
  projects: readonly Project[];
  issue: IssueSummary;
  selectLabel: SelectLabel;
  showBoard: boolean;
  scope: BoardScopeSelection;
  showStatus: boolean;
}) {
  return (
    <article className="issue-card" data-status={issue.status}>
      <div className="issue-card-topline">
        <Link
          className="issue-id"
          to={issuePath(issue.boardId, issue.id)}
        >
          {issue.id}
        </Link>
        <span className="issue-priority">P{issue.priority}</span>
      </div>
      <h3>
        <Link to={issuePath(issue.boardId, issue.id)}>
          {issue.title}
        </Link>
      </h3>
      <IssueMetadata
        boards={boards}
        projects={projects}
        issue={issue}
        selectLabel={selectLabel}
        showBoard={showBoard}
        scope={scope}
        showStatus={showStatus}
      />
    </article>
  );
}

export function IssueList({
  boards,
  projects,
  stream,
  loadMore,
  selectLabel,
  showBoard,
  scope,
}: {
  boards: readonly BoardSummary[];
  projects: readonly Project[];
  stream: IssuePageStream | undefined;
  loadMore: (key: string) => void;
  selectLabel: SelectLabel;
  showBoard: boolean;
  scope: BoardScopeSelection;
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
              <tr key={issueIdentity(issue)}>
                <td>
                  <Link
                    className="issue-id"
                    to={issuePath(issue.boardId, issue.id)}
                  >
                    {issue.id}
                  </Link>
                </td>
                <td className="issue-title-cell">
                  <Link to={issuePath(issue.boardId, issue.id)}>
                    {issue.title}
                  </Link>
                  <IssueWaitingReason issue={issue} />
                  {issue.labels.length > 0 && (
                    <span className="table-labels">
                      {issue.labels.map((label) => (
                        <IssueLabel key={label} label={label} select={selectLabel} />
                      ))}
                    </span>
                  )}
                </td>
                {showBoard && (
                  <td>
                    {visibleIssueProvenance(issue, { boards, projects }, scope) ??
                      boardName(boards, issue.boardId)}
                  </td>
                )}
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
          <li key={issueIdentity(issue)}>
            <div className="issue-record-heading">
              <Link
                className="issue-id"
                to={issuePath(issue.boardId, issue.id)}
              >
                {issue.id}
              </Link>
              <IssueStatusBadge status={issue.status} />
              <span className="issue-priority">P{issue.priority}</span>
            </div>
            <Link
              className="issue-record-title"
              to={issuePath(issue.boardId, issue.id)}
            >
              {issue.title}
            </Link>
            <IssueMetadata
              boards={boards}
              projects={projects}
              issue={issue}
              selectLabel={selectLabel}
              showBoard={showBoard}
              scope={scope}
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
        <Button type="button" variant="outline" aria-label={control.label} onClick={load}>
          Load more
        </Button>
      )}
      {control.kind === "loading" && (
        <span role="status">{control.label}</span>
      )}
      {control.kind === "retry" && (
        <>
          <span role="alert">{control.message}</span>
          <Button type="button" variant="outline" aria-label={control.label} onClick={load}>
            Retry
          </Button>
        </>
      )}
      {control.kind === "exhausted" && <span>{control.label}</span>}
    </div>
  );
}

function IssueMetadata({
  boards,
  projects,
  issue,
  selectLabel,
  scope,
  showBoard,
  showStatus,
}: {
  boards: readonly BoardSummary[];
  projects: readonly Project[];
  issue: IssueSummary;
  selectLabel: SelectLabel;
  scope: BoardScopeSelection;
  showBoard: boolean;
  showStatus: boolean;
}) {
  return (
    <div className="issue-metadata">
      {showStatus && <IssueStatusBadge status={issue.status} />}
      <span>{issueTypeLabel(issue.type)}</span>
      {issue.activeClaim !== undefined && <span>{issue.activeClaim.actor}</span>}
      {showBoard && (
        <span>
          {visibleIssueProvenance(issue, { boards, projects }, scope) ??
            boardName(boards, issue.boardId)}
        </span>
      )}
      <IssueTime issue={issue} compact labeled />
      {issue.labels.map((label) => (
        <IssueLabel key={label} label={label} select={selectLabel} />
      ))}
      <IssueWaitingReason issue={issue} />
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

function AggregateReadStatus({
  streams,
}: {
  streams: readonly IssuePageStream[];
}) {
  const status = streams.find((stream) => stream.aggregateStatus !== undefined)
    ?.aggregateStatus;
  if (status?.complete !== false) {
    return null;
  }
  const problems = status.problems.map((problem) => problem.sourceId).join(", ");
  return (
    <p className="aggregate-read-status" role="status">
      Partial data{problems === "" ? "" : ` · unavailable: ${problems}`}
    </p>
  );
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

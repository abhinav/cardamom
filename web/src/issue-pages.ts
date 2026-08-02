import { Code, ConnectError } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { skipToken, useTransport } from "@connectrpc/connect-query";
import {
  useInfiniteQuery,
  useQueryClient,
  type QueryClient,
  type QueryKey,
} from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef } from "react";

import {
  IssueSort,
  IssueStatus,
  IssueType,
  SortDirection,
  type IssueSummary,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import {
  ListIssuesRequestSchema,
  type ListIssuesRequest,
  type ListIssuesResponse,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import type { BoardScope } from "./gen/cardamom/private/v1/scope_pb.ts";
import type { AggregateStatus } from "./gen/cardamom/private/v1/source_pb.ts";
import {
  buildIssueQuery,
  issueTypeLabel,
  visibleIssueStatuses,
  visibleIssueTypes,
  type IssueGrouping,
  type IssueViewPreferences,
} from "./issue-collection.ts";
import { issueStatusPresentation } from "./issue-status.tsx";
import { issueCollectionQueryOptions } from "./query-runtime.ts";

export type IssueCollectionMode = "board" | "list";

/** IssueStream describes one independently continued issue collection. */
export interface IssueStream {
  key: string;
  label: string;
  request: ListIssuesRequest;
}

export type IssuePageStatus = "idle" | "loading" | "ready" | "error";

/** IssuePageStream is the browser's loaded state for one continuation stream. */
export interface IssuePageStream extends IssueStream {
  issues: readonly IssueSummary[];
  /** totalCount is the complete server-side count for this filtered stream. */
  totalCount: number;
  status: IssuePageStatus;
  /** pageCount distinguishes an untouched stream from an exhausted first page. */
  pageCount: number;
  /** nextPageToken is absent after the server exhausts the stream. */
  nextPageToken?: string;
  error?: Error;
  aggregateStatus?: AggregateStatus;
}

/** IssuePageState retains independent streams in their display order. */
export interface IssuePageState {
  streams: readonly IssuePageStream[];
  /** hasCompletedInitialLoad keeps the collection shell mounted across later queries. */
  hasCompletedInitialLoad: boolean;
}

/** buildIssueStreams translates one route view into its continuation streams. */
export function buildIssueStreams(
  scope: BoardScope,
  mode: IssueCollectionMode,
  view: IssueViewPreferences,
  grouping: IssueGrouping = "status",
): IssueStream[] {
  const base = buildIssueQuery(scope, view);
  if (mode === "list") {
    return [{
      key: "list",
      label: "",
      request: pageRequest(base, {}, 100),
    }];
  }
  if (grouping === "status") {
    return visibleIssueStatuses(
      view.filters.lifecycle,
      view.filters.status,
    ).map((status) => ({
      key: `status:${status}`,
      label: issueStatusPresentation(status).label,
      request: pageRequest(base, {
        statuses: [status],
        ...statusOrder(status, view),
      }),
    }));
  }
  return visibleIssueTypes(view.filters.type).map((type) => ({
    key: `type:${type}`,
    label: issueTypeLabel(type),
    request: pageRequest(base, {
      types: [type],
      ...(view.sort === "natural"
        ? {
            sort: IssueSort.UPDATED_AT,
            direction: SortDirection.DESCENDING,
          }
        : {}),
    }),
  }));
}

interface PageRequestOverrides {
  statuses?: IssueStatus[];
  types?: ListIssuesRequest["types"];
  sort?: IssueSort;
  direction?: SortDirection;
}

function pageRequest(
  base: ListIssuesRequest,
  overrides: PageRequestOverrides = {},
  limit = 20,
): ListIssuesRequest {
  return create(ListIssuesRequestSchema, {
    ...base,
    ...overrides,
    limit,
    pageToken: undefined,
  });
}

function statusOrder(
  status: IssueStatus,
  view: IssueViewPreferences,
): Pick<ListIssuesRequest, "sort" | "direction"> {
  if (view.sort !== "natural") {
    return { sort: viewSort(view), direction: baseDirection(view) };
  }
  if (status === IssueStatus.READY || status === IssueStatus.BLOCKED) {
    return { sort: IssueSort.PRIORITY, direction: SortDirection.ASCENDING };
  }
  return { sort: IssueSort.UPDATED_AT, direction: SortDirection.DESCENDING };
}

function viewSort(view: IssueViewPreferences): IssueSort {
  switch (view.sort) {
    case "natural":
    case "updated":
      return IssueSort.UPDATED_AT;
    case "priority":
      return IssueSort.PRIORITY;
    case "created":
      return IssueSort.CREATED_AT;
    case "title":
      return IssueSort.TITLE;
  }
}

function baseDirection(view: IssueViewPreferences): SortDirection {
  return view.direction === "ascending"
    ? SortDirection.ASCENDING
    : SortDirection.DESCENDING;
}

/** IssuePageQuery is the query-runtime state projected into route presentation. */
export interface IssuePageQuery {
  key: string;
  pages: readonly ListIssuesResponse[];
  fetching: boolean;
  fetchingNextPage: boolean;
  error?: Error;
}

/** projectIssuePages preserves display order while removing cross-stream duplicates. */
export function projectIssuePages(
  streams: readonly IssueStream[],
  queries: readonly IssuePageQuery[],
  hasCompletedInitialLoad: boolean,
): IssuePageState {
  const queryByKey = new Map(queries.map((query) => [query.key, query]));
  const seen = new Set<string>();
  let completed = hasCompletedInitialLoad;
  const projected = streams.map((stream): IssuePageStream => {
    const query = queryByKey.get(stream.key);
    const pages = query?.pages ?? [];
    if (pages.length > 0) {
      completed = true;
    }
    const issues = pages.flatMap((page) => page.issues).filter((issue) => {
      if (seen.has(issue.id)) {
        return false;
      }
      seen.add(issue.id);
      return true;
    });
    const lastPage = pages.at(-1);
    let status: IssuePageStatus = "idle";
    const loading = pages.length === 0
      ? query?.fetching
      : query?.fetchingNextPage;
    if (loading) {
      status = "loading";
    } else if (query?.error !== undefined) {
      status = "error";
    } else if (pages.length > 0) {
      status = "ready";
    }
    return {
      ...stream,
      issues,
      totalCount: lastPage?.totalCount ?? 0,
      status,
      pageCount: pages.length,
      nextPageToken: lastPage?.nextPageToken,
      aggregateStatus: lastPage?.aggregateStatus,
      error: query?.error,
    };
  });
  return { streams: projected, hasCompletedInitialLoad: completed };
}

/** issuePageIssues returns all loaded issues in stream display order. */
export function issuePageIssues(state: IssuePageState): IssueSummary[] {
  return state.streams.flatMap((stream) => stream.issues);
}

export type IssueLoadControl =
  | { kind: "load"; label: string }
  | { kind: "loading"; label: string }
  | { kind: "retry"; label: string; message: string }
  | { kind: "exhausted"; label: string };

/** issueLoadControl defines the accessible state rendered at a stream's end. */
export function issueLoadControl(stream: IssuePageStream): IssueLoadControl {
  const subject = stream.label === "" ? "issues" : `${stream.label} issues`;
  if (stream.status === "loading") {
    return {
      kind: "loading",
      label: stream.pageCount === 0
        ? `Loading ${subject}`
        : `Loading more ${subject}`,
    };
  }
  if (stream.status === "error") {
    return {
      kind: "retry",
      label: `Retry loading ${subject}`,
      message: stream.error?.message ?? "Issues could not be loaded",
    };
  }
  if (stream.pageCount > 0 && stream.nextPageToken === undefined) {
    return { kind: "exhausted", label: `All ${subject} loaded` };
  }
  return { kind: "load", label: `Load more ${subject}` };
}

/** isInvalidContinuation identifies caller-repairable page-token failures. */
export function isInvalidContinuation(error: unknown): boolean {
  const code = ConnectError.from(error).code;
  return code === Code.InvalidArgument || code === Code.FailedPrecondition;
}

/** recoverInvalidIssueContinuation discards only the stream with a stale cursor. */
export async function recoverInvalidIssueContinuation(
  queryClient: QueryClient,
  queryKey: QueryKey,
  pageCount: number,
  error: unknown,
): Promise<boolean> {
  if (pageCount === 0 || !isInvalidContinuation(error)) {
    return false;
  }
  await queryClient.resetQueries({ queryKey, exact: true });
  return true;
}

export interface IssuePages {
  state: IssuePageState;
  loadMore: (key: string) => void;
}

interface IssueQuerySlot extends IssuePageQuery {
  loadMore: () => void;
}

function useIssueQuerySlot(stream: IssueStream | undefined): IssueQuerySlot | undefined {
  const transport = useTransport();
  const queryClient = useQueryClient();
  const input = stream?.request ?? skipToken;
  const options = useMemo(
    () => issueCollectionQueryOptions(transport, input),
    [input, transport],
  );
  const query = useInfiniteQuery(options);
  const pages = query.data?.pages ?? [];

  useEffect(() => {
    if (stream === undefined || query.error === null) {
      return;
    }
    void recoverInvalidIssueContinuation(
      queryClient,
      options.queryKey,
      pages.length,
      query.error,
    );
  }, [options.queryKey, pages.length, query.error, queryClient, stream]);

  const loadMore = useCallback(() => {
    if (query.isFetching) {
      return;
    }
    if (pages.length === 0 && query.isError) {
      void query.refetch();
      return;
    }
    if (query.hasNextPage || query.isFetchNextPageError) {
      void query.fetchNextPage();
    }
  }, [pages.length, query]);

  if (stream === undefined) {
    return undefined;
  }
  return {
    key: stream.key,
    pages,
    fetching: query.isFetching,
    fetchingNextPage: query.isFetchingNextPage,
    ...(query.isError && query.error !== null ? { error: query.error } : {}),
    loadMore,
  };
}

/** useIssueQueries projects fixed query slots into the visible collection. */
export function useIssueQueries(streams: readonly IssueStream[]): IssuePages {
  const byKey = new Map(streams.map((stream) => [stream.key, stream]));
  // Visible columns vary, but React requires hook calls to retain a fixed
  // count and order. Inactive domain slots use skipToken inside the hook.
  const list = useIssueQuerySlot(byKey.get("list"));
  const ready = useIssueQuerySlot(byKey.get(`status:${IssueStatus.READY}`));
  const blocked = useIssueQuerySlot(byKey.get(`status:${IssueStatus.BLOCKED}`));
  const active = useIssueQuerySlot(
    byKey.get(`status:${IssueStatus.IN_PROGRESS}`),
  );
  const waiting = useIssueQuerySlot(
    byKey.get(`status:${IssueStatus.WAITING}`),
  );
  const closed = useIssueQuerySlot(byKey.get(`status:${IssueStatus.CLOSED}`));
  const cancelled = useIssueQuerySlot(
    byKey.get(`status:${IssueStatus.CANCELLED}`),
  );
  const workstreams = useIssueQuerySlot(
    byKey.get(`type:${IssueType.WORKSTREAM}`),
  );
  const tasks = useIssueQuerySlot(byKey.get(`type:${IssueType.TASK}`));
  const checkpoints = useIssueQuerySlot(
    byKey.get(`type:${IssueType.CHECKPOINT}`),
  );
  const routines = useIssueQuerySlot(byKey.get(`type:${IssueType.ROUTINE}`));
  const querySlots = [
    list,
    ready,
    blocked,
    active,
    waiting,
    closed,
    cancelled,
    workstreams,
    tasks,
    checkpoints,
    routines,
  ].filter((slot): slot is IssueQuerySlot => slot !== undefined);
  const completedInitialLoad = useRef(false);
  const state = projectIssuePages(
    streams,
    querySlots,
    completedInitialLoad.current,
  );
  completedInitialLoad.current = state.hasCompletedInitialLoad;
  const loadMore = useCallback((key: string) => {
    querySlots.find((slot) => slot.key === key)?.loadMore();
  }, [querySlots]);
  return { state, loadMore };
}

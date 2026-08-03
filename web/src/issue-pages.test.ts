import { Code, ConnectError } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { InfiniteQueryObserver, QueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";

import { toBoardScopeMessage } from "./board-scope.ts";
import {
  IssueSort,
  IssueStatus,
  IssueType,
  SortDirection,
  type IssueSummary,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import { ListIssuesResponseSchema } from "./gen/cardamom/private/v1/issue_pb.ts";
import { SourceRefSchema } from "./gen/cardamom/private/v1/source_pb.ts";
import { defaultBoardView, defaultListView } from "./issue-collection.ts";
import { issueIdentity } from "./provenance.ts";
import {
  buildIssueStreams,
  issueLoadControl,
  issuePageIssues,
  isInvalidContinuation,
  projectIssuePages,
  recoverInvalidIssueContinuation,
} from "./issue-pages.ts";

describe("issue page streams", () => {
  it("uses one bounded continuation stream for List", () => {
    const scope = toBoardScopeMessage({ kind: "board", boardId: "board-1" });

    const streams = buildIssueStreams(scope!, "list", defaultListView);

    expect(streams).toHaveLength(1);
    expect(streams[0]?.key).toBe("list");
    expect(streams[0]?.request.limit).toBe(100);
    expect(streams[0]?.request.statuses).toEqual([]);
    expect(streams[0]?.request.types).toEqual([
      IssueType.WORKSTREAM,
      IssueType.TASK,
      IssueType.CHECKPOINT,
    ]);
  });

  it("gives each visible status its natural observer ordering", () => {
    const scope = toBoardScopeMessage({ kind: "board", boardId: "board-1" });

    const streams = buildIssueStreams(
      scope!,
      "board",
      defaultBoardView,
      "status",
    );

    expect(streams.map((stream) => ({
      key: stream.key,
      statuses: stream.request.statuses,
      sort: stream.request.sort,
      direction: stream.request.direction,
      limit: stream.request.limit,
    }))).toEqual([
      {
        key: `status:${IssueStatus.READY}`,
        statuses: [IssueStatus.READY],
        sort: IssueSort.PRIORITY,
        direction: SortDirection.ASCENDING,
        limit: 20,
      },
      {
        key: `status:${IssueStatus.BLOCKED}`,
        statuses: [IssueStatus.BLOCKED],
        sort: IssueSort.PRIORITY,
        direction: SortDirection.ASCENDING,
        limit: 20,
      },
      {
        key: `status:${IssueStatus.IN_PROGRESS}`,
        statuses: [IssueStatus.IN_PROGRESS],
        sort: IssueSort.UPDATED_AT,
        direction: SortDirection.DESCENDING,
        limit: 20,
      },
      {
        key: `status:${IssueStatus.WAITING}`,
        statuses: [IssueStatus.WAITING],
        sort: IssueSort.UPDATED_AT,
        direction: SortDirection.DESCENDING,
        limit: 20,
      },
      {
        key: `status:${IssueStatus.CLOSED}`,
        statuses: [IssueStatus.CLOSED],
        sort: IssueSort.UPDATED_AT,
        direction: SortDirection.DESCENDING,
        limit: 20,
      },
    ]);
  });

  it("uses one type stream and selected ordering for each visible type", () => {
    const scope = toBoardScopeMessage({ kind: "board", boardId: "board-1" });
    const view = {
      ...defaultBoardView,
      sort: "title" as const,
      direction: "ascending" as const,
    };

    const streams = buildIssueStreams(scope!, "board", view, "type");

    expect(streams.map((stream) => ({
      types: stream.request.types,
      sort: stream.request.sort,
      direction: stream.request.direction,
    }))).toEqual([
      {
        types: [IssueType.WORKSTREAM],
        sort: IssueSort.TITLE,
        direction: SortDirection.ASCENDING,
      },
      {
        types: [IssueType.TASK],
        sort: IssueSort.TITLE,
        direction: SortDirection.ASCENDING,
      },
      {
        types: [IssueType.CHECKPOINT],
        sort: IssueSort.TITLE,
        direction: SortDirection.ASCENDING,
      },
    ]);
  });

  it("uses recent activity for natural type streams", () => {
    const scope = toBoardScopeMessage({ kind: "board", boardId: "board-1" });

    const streams = buildIssueStreams(
      scope!,
      "board",
      defaultBoardView,
      "type",
    );

    expect(streams.every(
      (stream) =>
        stream.request.sort === IssueSort.UPDATED_AT &&
        stream.request.direction === SortDirection.DESCENDING,
    )).toBe(true);
  });
});

describe("issue page state", () => {
  it("appends pages, removes duplicate IDs, and records exhaustion", () => {
    const exhausted = projectIssuePages(
      [stream("list", "")],
      [{
        key: "list",
        pages: [
          response([issue("cm-1"), issue("cm-2")], "next", 3),
          response([issue("cm-2"), issue("cm-3")], undefined, 3),
        ],
        fetching: false,
        fetchingNextPage: false,
      }],
      false,
    );

    expect(issuePageIssues(exhausted).map((item) => item.id)).toEqual([
      "cm-1",
      "cm-2",
      "cm-3",
    ]);
    expect(exhausted.streams[0]).toMatchObject({
      status: "ready",
      pageCount: 2,
      nextPageToken: undefined,
      totalCount: 3,
    });
  });

  it("deduplicates stable IDs across independent Board streams", () => {
    const active = projectIssuePages(
      [stream("status:1", "Open"), stream("status:3", "In progress")],
      [
        {
          key: "status:1",
          pages: [response([issue("cm-moving"), issue("cm-open")])],
          fetching: false,
          fetchingNextPage: false,
        },
        {
          key: "status:3",
          pages: [response([issue("cm-moving"), issue("cm-active")])],
          fetching: false,
          fetchingNextPage: false,
        },
      ],
      false,
    );

    expect(issuePageIssues(active).map((item) => item.id)).toEqual([
      "cm-moving",
      "cm-open",
      "cm-active",
    ]);
  });

  it("keeps matching IDs from separate aggregate sources", () => {
    const aggregate = projectIssuePages(
      [stream("status:1", "Open"), stream("status:3", "In progress")],
      [
        {
          key: "status:1",
          pages: [response([issue("cm-shared", "builder", "board-1")])],
          fetching: false,
          fetchingNextPage: false,
        },
        {
          key: "status:3",
          pages: [response([issue("cm-shared", "laptop", "board-1")])],
          fetching: false,
          fetchingNextPage: false,
        },
      ],
      false,
    );

    expect(issuePageIssues(aggregate).map((item) => item.source?.sourceId)).toEqual([
      "builder",
      "laptop",
    ]);
    expect([...new Set(issuePageIssues(aggregate).map(issueIdentity))]).toHaveLength(2);
  });

  it("keeps the collection surface mounted while a new query loads", () => {
    const reset = projectIssuePages(
      [{ ...stream("list", ""), key: "list:new-query" }],
      [{
        key: "list:new-query",
        pages: [],
        fetching: true,
        fetchingNextPage: false,
      }],
      true,
    );

    expect(reset).toMatchObject({
      hasCompletedInitialLoad: true,
      streams: [{
        key: "list:new-query",
        issues: [],
        status: "loading",
        pageCount: 0,
      }],
    });
  });

  it("classifies continuation failures without hiding ordinary errors", () => {
    expect(isInvalidContinuation(
      new ConnectError("page token is stale", Code.InvalidArgument),
    )).toBe(true);
    expect(isInvalidContinuation(
      new ConnectError("board revision changed", Code.FailedPrecondition),
    )).toBe(true);
    expect(isInvalidContinuation(
      new ConnectError("service unavailable", Code.Unavailable),
    )).toBe(false);
  });

  it("retains loaded rows and continuation after an ordinary service error", () => {
    const failed = projectIssuePages(
      [stream("list", "")],
      [{
        key: "list",
        pages: [response([issue("cm-1")], "next")],
        fetching: false,
        fetchingNextPage: false,
        error: new ConnectError("service unavailable", Code.Unavailable),
      }],
      true,
    );

    expect(failed.streams[0]).toMatchObject({
      issues: [{ id: "cm-1" }],
      status: "error",
      pageCount: 1,
      nextPageToken: "next",
    });
    expect(issueLoadControl(failed.streams[0]!)).toMatchObject({
      kind: "retry",
      label: "Retry loading issues",
    });
  });

  it("keeps loaded rows stable during a background refresh", () => {
    const refreshing = projectIssuePages(
      [stream("list", "")],
      [{
        key: "list",
        pages: [response([issue("cm-1")], undefined, 1)],
        fetching: true,
        fetchingNextPage: false,
      }],
      true,
    );

    expect(refreshing.streams[0]).toMatchObject({
      issues: [{ id: "cm-1" }],
      status: "ready",
      pageCount: 1,
    });
    expect(issueLoadControl(refreshing.streams[0]!)).toEqual({
      kind: "exhausted",
      label: "All issues loaded",
    });
  });
});

describe("issue loading presentation", () => {
  it("keeps load, loading, retry, and exhausted states accessible", () => {
    const initial = projectIssuePages(
      [stream("status:1", "Open")],
      [{
        key: "status:1",
        pages: [],
        fetching: false,
        fetchingNextPage: false,
      }],
      false,
    );
    expect(issueLoadControl(initial.streams[0]!)).toEqual({
      kind: "load",
      label: "Load more Open issues",
    });

    const loading = projectIssuePages(
      [stream("status:1", "Open")],
      [{
        key: "status:1",
        pages: [],
        fetching: true,
        fetchingNextPage: false,
      }],
      false,
    );
    expect(issueLoadControl(loading.streams[0]!)).toEqual({
      kind: "loading",
      label: "Loading Open issues",
    });

    const failed = projectIssuePages(
      [stream("status:1", "Open")],
      [{
        key: "status:1",
        pages: [],
        fetching: false,
        fetchingNextPage: false,
        error: new Error("network unavailable"),
      }],
      false,
    );
    expect(issueLoadControl(failed.streams[0]!)).toEqual({
      kind: "retry",
      label: "Retry loading Open issues",
      message: "network unavailable",
    });

    const exhausted = projectIssuePages(
      [stream("status:1", "Open")],
      [{
        key: "status:1",
        pages: [response([issue("cm-open")])],
        fetching: false,
        fetchingNextPage: false,
      }],
      false,
    );
    expect(issueLoadControl(exhausted.streams[0]!)).toEqual({
      kind: "exhausted",
      label: "All Open issues loaded",
    });
  });

  it("resets only a loaded stream after its continuation becomes invalid", async () => {
    const queryClient = new QueryClient();
    const staleKey = ["issues", "open"] as const;
    const otherKey = ["issues", "closed"] as const;
    queryClient.setQueryData(otherKey, { pages: ["current"] });
    const firstPage = vi.fn()
      .mockResolvedValueOnce({ value: "stale", nextPageToken: "stale-token" })
      .mockResolvedValueOnce({ value: "fresh" });
    const observer = new InfiniteQueryObserver(queryClient, {
      queryKey: staleKey,
      queryFn: ({ pageParam }) =>
        pageParam === undefined
          ? firstPage()
          : Promise.reject(
            new ConnectError("page token is stale", Code.InvalidArgument),
          ),
      initialPageParam: undefined,
      getNextPageParam: (lastPage) => lastPage.nextPageToken,
      retry: false,
    });
    const unsubscribe = observer.subscribe(() => {});
    await observer.refetch();
    await observer.fetchNextPage();
    const failed = observer.getCurrentResult();

    await recoverInvalidIssueContinuation(
      queryClient,
      staleKey,
      failed.data?.pages.length ?? 0,
      failed.error,
    );

    expect(firstPage).toHaveBeenCalledTimes(2);
    expect(observer.getCurrentResult().data?.pages).toEqual([
      { value: "fresh" },
    ]);
    expect(queryClient.getQueryData(otherKey)).toEqual({ pages: ["current"] });
    unsubscribe();
  });
});

function stream(key: string, label: string) {
  const scope = toBoardScopeMessage({ kind: "board", boardId: "board-1" });
  return buildIssueStreams(scope!, "list", defaultListView).map((value) => ({
    ...value,
    key,
    label,
  }))[0]!;
}

function response(
  issues: IssueSummary[],
  nextPageToken?: string,
  totalCount = issues.length,
) {
  return create(ListIssuesResponseSchema, {
    issues,
    totalCount,
    ...(nextPageToken === undefined ? {} : { nextPageToken }),
  });
}

function issue(
  id: string,
  sourceId?: string,
  boardId = "board-1",
): IssueSummary {
  return {
    $typeName: "cardamom.private.v1.IssueSummary",
    id,
    boardId,
    title: id,
    type: IssueType.TASK,
    lifecycle: 1,
    status: IssueStatus.READY,
    priority: 0,
    labels: [],
    blocked: false,
    ...(sourceId === undefined
      ? {}
      : { source: create(SourceRefSchema, { sourceId }) }),
  };
}

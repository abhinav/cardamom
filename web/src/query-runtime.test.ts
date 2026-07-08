import { create } from "@bufbuild/protobuf";
import { createRouterTransport } from "@connectrpc/connect";
import {
  createConnectQueryKey,
  createInfiniteQueryOptions,
  createQueryOptions,
} from "@connectrpc/connect-query";
import {
  InfiniteQueryObserver,
  QueryClient,
  QueryObserver,
} from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";

import { WatchResource } from "./gen/cardamom/private/v1/change_pb.ts";
import {
  ConfigurationService,
  GetConfigurationResponseSchema,
} from "./gen/cardamom/private/v1/configuration_pb.ts";
import {
  AttachmentService,
  ListAttachmentsResponseSchema,
} from "./gen/cardamom/private/v1/attachment_pb.ts";
import {
  GetBoardResponseSchema,
  ProjectService,
} from "./gen/cardamom/private/v1/project_pb.ts";
import {
  IssueService,
  IssueStatus,
  ListIssuesRequestSchema,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import {
  bootstrapQueryOptions,
  invalidateActiveQueryResources,
  invalidateQueryResources,
  issueCollectionQueryOptions,
  runInvalidatingMutation,
  unaryRouteQueryOptions,
} from "./query-runtime.ts";

describe("bootstrap query", () => {
  it("loads the startup catalog through the generated Connect method", async () => {
    const getBootstrap = vi.fn(() => ({
      boards: [{ id: "board-1", name: "Primary" }],
      serverDefaultBoardId: "board-1",
    }));
    const transport = createRouterTransport(({ service }) => {
      service(ProjectService, { getBootstrap });
    });
    const queryClient = new QueryClient();

    const bootstrap = await queryClient.fetchQuery(
      bootstrapQueryOptions(transport),
    );

    expect(getBootstrap).toHaveBeenCalledOnce();
    expect(bootstrap.boards).toEqual([
      expect.objectContaining({ id: "board-1", name: "Primary" }),
    ]);
    expect(bootstrap.serverDefaultBoardId).toBe("board-1");
  });

  it("waits for an explicit retry after a startup failure", async () => {
    const getBootstrap = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({ boards: [] });
    const transport = createRouterTransport(({ service }) => {
      service(ProjectService, { getBootstrap });
    });
    const queryClient = new QueryClient();
    const options = bootstrapQueryOptions(transport);

    await expect(queryClient.fetchQuery(options)).rejects.toThrow();
    expect(getBootstrap).toHaveBeenCalledOnce();

    const bootstrap = await queryClient.fetchQuery(options);

    expect(getBootstrap).toHaveBeenCalledTimes(2);
    expect(bootstrap.boards).toEqual([]);
  });
});

describe("issue collection query", () => {
  it("continues one descriptor-keyed stream with the server page token", async () => {
    const listIssues = vi.fn((request: { pageToken?: string }) => ({
      issues: [],
      nextPageToken: request.pageToken === undefined ? "next" : undefined,
    }));
    const transport = createRouterTransport(({ service }) => {
      service(IssueService, { listIssues });
    });
    const queryClient = new QueryClient();
    const options = issueCollectionQueryOptions(
      transport,
      create(ListIssuesRequestSchema, {
        statuses: [IssueStatus.READY],
        limit: 100,
      }),
    );
    const observer = new InfiniteQueryObserver(queryClient, options);

    await observer.refetch();
    await observer.fetchNextPage();

    expect(listIssues.mock.calls.map(([request]) => request.pageToken)).toEqual([
      undefined,
      "next",
    ]);
    expect(observer.getCurrentResult().data?.pages).toHaveLength(2);
  });

  it("keys independent column streams by their complete stable request", () => {
    const transport = createRouterTransport(() => {});
    const open = issueCollectionQueryOptions(
      transport,
      create(ListIssuesRequestSchema, {
        statuses: [IssueStatus.READY],
        limit: 100,
      }),
    );
    const closed = issueCollectionQueryOptions(
      transport,
      create(ListIssuesRequestSchema, {
        statuses: [IssueStatus.CLOSED],
        limit: 100,
      }),
    );

    expect(open.queryKey).not.toEqual(closed.queryKey);
    expect(open.queryKey[1]).toMatchObject({ cardinality: "infinite" });
    expect(closed.queryKey[1]).toMatchObject({ cardinality: "infinite" });
  });

  it("aborts an in-flight page request when its observer is removed", async () => {
    let requestSignal: AbortSignal | undefined;
    const transport = createRouterTransport(({ service }) => {
      service(IssueService, {
        async listIssues(_request, context) {
          requestSignal = context.signal;
          await new Promise((_, reject) => {
            context.signal.addEventListener(
              "abort",
              () => reject(new Error("aborted")),
              { once: true },
            );
          });
          return { issues: [] };
        },
      });
    });
    const queryClient = new QueryClient();
    const observer = new InfiniteQueryObserver(
      queryClient,
      issueCollectionQueryOptions(
        transport,
        create(ListIssuesRequestSchema, { limit: 100 }),
      ),
    );
    const unsubscribe = observer.subscribe(() => {});
    await vi.waitFor(() => expect(requestSignal).toBeDefined());

    unsubscribe();

    expect(requestSignal?.aborted).toBe(true);
  });
});

describe("query invalidation", () => {
  it("invalidates infinite issue collections through the IssueService family", async () => {
    const transport = createRouterTransport(() => {});
    const queryClient = new QueryClient();
    const issues = issueCollectionQueryOptions(
      transport,
      create(ListIssuesRequestSchema, { limit: 100 }),
    );
    queryClient.setQueryData(issues.queryKey, { pages: [], pageParams: [] });

    await invalidateQueryResources(queryClient, [WatchResource.ISSUES]);

    expect(queryClient.getQueryState(issues.queryKey)?.isInvalidated).toBe(true);
  });

  it("invalidates only the query families named by a change resource", async () => {
    const transport = createRouterTransport(() => {});
    const queryClient = new QueryClient();
    const bootstrapKey = createQueryOptions(
      ProjectService.method.getBootstrap,
      {},
      { transport },
    ).queryKey;
    const boardKey = createConnectQueryKey({
      schema: ProjectService.method.getBoard,
      input: { boardId: "board-1" },
      transport,
      cardinality: "finite",
    });
    queryClient.setQueryData(bootstrapKey, {
      boards: [],
      projects: [],
      serverDefaultBoardId: undefined,
      version: "",
      schemaVersion: 0n,
    });
    queryClient.setQueryData(boardKey, create(GetBoardResponseSchema));

    await invalidateQueryResources(queryClient, [
      WatchResource.BOARD_CATALOG,
    ]);

    expect(queryClient.getQueryState(bootstrapKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(boardKey)?.isInvalidated).toBe(false);
  });

  it("invalidates attachment metadata with issue-owned reads", async () => {
    const transport = createRouterTransport(() => {});
    const queryClient = new QueryClient();
    const attachmentKey = createInfiniteQueryOptions(
      AttachmentService.method.listAttachments,
      { boardId: "board-1", pageSize: 100, pageToken: "" },
      {
        transport,
        pageParamKey: "pageToken",
        getNextPageParam: (page) => page.nextPageToken || undefined,
      },
    ).queryKey;
    queryClient.setQueryData(
      attachmentKey,
      {
        pages: [create(ListAttachmentsResponseSchema)],
        pageParams: [""],
      },
    );

    await invalidateQueryResources(queryClient, [WatchResource.ISSUES]);

    expect(queryClient.getQueryState(attachmentKey)?.isInvalidated).toBe(true);
  });

  it("invalidates resource families only after a successful mutation", async () => {
    const transport = createRouterTransport(() => {});
    const queryClient = new QueryClient();
    const boardKey = createConnectQueryKey({
      schema: ProjectService.method.getBoard,
      input: { boardId: "board-1" },
      transport,
      cardinality: "finite",
    });
    queryClient.setQueryData(boardKey, create(GetBoardResponseSchema));

    await runInvalidatingMutation(
      queryClient,
      [WatchResource.BOARD],
      async () => "updated",
    );

    expect(queryClient.getQueryState(boardKey)?.isInvalidated).toBe(true);

    const failedQueryClient = new QueryClient();
    failedQueryClient.setQueryData(boardKey, create(GetBoardResponseSchema));
    await expect(
      runInvalidatingMutation(
        failedQueryClient,
        [WatchResource.BOARD],
        async () => {
          throw new Error("update failed");
        },
      ),
    ).rejects.toThrow("update failed");
    expect(failedQueryClient.getQueryState(boardKey)?.isInvalidated).toBe(false);
  });

  it("invalidates configuration reads with board settings", async () => {
    const transport = createRouterTransport(() => {});
    const queryClient = new QueryClient();
    const configurationKey = createConnectQueryKey({
      schema: ConfigurationService.method.getConfiguration,
      input: { boardId: "board-1" },
      transport,
      cardinality: "finite",
    });
    queryClient.setQueryData(
      configurationKey,
      create(GetConfigurationResponseSchema),
    );

    await invalidateQueryResources(queryClient, [WatchResource.BOARD]);

    expect(queryClient.getQueryState(configurationKey)?.isInvalidated).toBe(true);
  });

  it("refreshes active queries after reconnect without touching inactive queries", async () => {
    const getBootstrap = vi.fn(() => ({ boards: [] }));
    const transport = createRouterTransport(({ service }) => {
      service(ProjectService, { getBootstrap });
    });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { staleTime: Infinity } },
    });
    const bootstrapOptions = bootstrapQueryOptions(transport);
    const boardKey = createConnectQueryKey({
      schema: ProjectService.method.getBoard,
      input: { boardId: "board-1" },
      transport,
      cardinality: "finite",
    });
    await queryClient.fetchQuery(bootstrapOptions);
    queryClient.setQueryData(boardKey, create(GetBoardResponseSchema));
    getBootstrap.mockClear();
    const observer = new QueryObserver(queryClient, bootstrapOptions);
    const unsubscribe = observer.subscribe(() => {});

    await invalidateActiveQueryResources(queryClient);

    expect(getBootstrap).toHaveBeenCalledOnce();
    expect(queryClient.getQueryState(boardKey)?.isInvalidated).toBe(false);
    unsubscribe();
  });
});

describe("route query policy", () => {
  it("preserves explicit retry and stale-data behavior", () => {
    const transport = createRouterTransport(() => {});
    const options = unaryRouteQueryOptions(
      ProjectService.method.getBoard,
      { boardId: "board-1" },
      transport,
    );
    const previous = create(GetBoardResponseSchema);

    expect(options.retry).toBe(false);
    expect(options.refetchOnReconnect).toBe(false);
    expect(options.refetchOnWindowFocus).toBe(false);
    expect(options.placeholderData?.(previous)).toBe(previous);
  });
});

import type {
  DescMessage,
  DescMethodUnary,
  MessageInitShape,
} from "@bufbuild/protobuf";
import type { Transport } from "@connectrpc/connect";
import {
  createInfiniteQueryOptions,
  createConnectQueryKey,
  createQueryOptions,
  skipToken,
} from "@connectrpc/connect-query";
import {
  keepPreviousData,
  type QueryClient,
  type QueryKey,
  type SkipToken,
} from "@tanstack/react-query";

import { AttachmentService } from "./gen/cardamom/private/v1/attachment_pb.ts";
import { CheckpointService } from "./gen/cardamom/private/v1/checkpoint_pb.ts";
import { WatchResource } from "./gen/cardamom/private/v1/change_pb.ts";
import { ConfigurationService } from "./gen/cardamom/private/v1/configuration_pb.ts";
import { ExecutionService } from "./gen/cardamom/private/v1/execution_pb.ts";
import {
  IssueService,
  type ListIssuesRequest,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import { ProjectService } from "./gen/cardamom/private/v1/project_pb.ts";
import { RecordService } from "./gen/cardamom/private/v1/record_pb.ts";

const watchResources = [
  WatchResource.BOARD_CATALOG,
  WatchResource.BOARD,
  WatchResource.ISSUES,
  WatchResource.LOG,
  WatchResource.APPROVALS,
] as const;

/** bootstrapQueryOptions preserves manual startup retry over the shared transport. */
export function bootstrapQueryOptions(transport: Transport) {
  return {
    ...createQueryOptions(ProjectService.method.getBootstrap, {}, { transport }),
    retry: false,
    refetchOnReconnect: false,
    refetchOnWindowFocus: false,
  };
}

/** issueCollectionQueryOptions owns continuation behavior for ListIssues. */
export function issueCollectionQueryOptions(
  transport: Transport,
  request: ListIssuesRequest | SkipToken,
) {
  const paging = {
    transport,
    pageParamKey: "pageToken" as const,
    getNextPageParam: (lastPage: { nextPageToken?: string }) =>
      lastPage.nextPageToken,
  };
  const options = request === skipToken
    ? createInfiniteQueryOptions(
      IssueService.method.listIssues,
      skipToken,
      paging,
    )
    : createInfiniteQueryOptions(
      IssueService.method.listIssues,
      // Connect Query requires the page field at the type level, while the
      // protocol uses an absent optional value to select the first page.
      request as ListIssuesRequest & { pageToken: string },
      paging,
    );
  return {
    ...options,
    retry: false,
    refetchOnReconnect: false,
    refetchOnWindowFocus: false,
  };
}

/** unaryRouteQueryOptions preserves route refresh and retry behavior. */
export function unaryRouteQueryOptions<
  I extends DescMessage,
  O extends DescMessage,
>(
  schema: DescMethodUnary<I, O>,
  input: MessageInitShape<I> | undefined,
  transport: Transport,
) {
  return {
    ...createQueryOptions(schema, input, { transport }),
    retry: false,
    refetchOnReconnect: false,
    refetchOnWindowFocus: false,
    placeholderData: keepPreviousData,
  };
}

/** unaryScopeQueryOptions never presents data from a previous scope identity. */
export function unaryScopeQueryOptions<
  I extends DescMessage,
  O extends DescMessage,
>(
  schema: DescMethodUnary<I, O>,
  input: MessageInitShape<I> | undefined,
  transport: Transport,
) {
  return {
    ...createQueryOptions(schema, input, { transport }),
    retry: false,
    refetchOnReconnect: false,
    refetchOnWindowFocus: false,
  };
}

/** invalidateQueryResources marks every Connect query family named by a change. */
export async function invalidateQueryResources(
  queryClient: QueryClient,
  resources: readonly WatchResource[],
): Promise<void> {
  await invalidateQueries(queryClient, resources, "all");
}

/** invalidateActiveQueryResources refreshes reads that may be stale after reconnect. */
export async function invalidateActiveQueryResources(
  queryClient: QueryClient,
): Promise<void> {
  await invalidateQueries(queryClient, watchResources, "active");
}

/** runInvalidatingMutation starts affected query refreshes after mutation success. */
export async function runInvalidatingMutation<T>(
  queryClient: QueryClient,
  resources: readonly WatchResource[],
  mutation: () => Promise<T>,
): Promise<T> {
  const result = await mutation();
  void invalidateQueryResources(queryClient, resources);
  return result;
}

async function invalidateQueries(
  queryClient: QueryClient,
  resources: readonly WatchResource[],
  type: "active" | "all",
): Promise<void> {
  const keys = [...new Set(resources)].flatMap(queryKeysForResource);
  await Promise.all(
    keys.map((queryKey) => queryClient.invalidateQueries({ queryKey, type })),
  );
}

function queryKeysForResource(resource: WatchResource): QueryKey[] {
  switch (resource) {
    case WatchResource.BOARD_CATALOG:
      return [
        createConnectQueryKey({
          schema: ProjectService.method.listProjects,
          cardinality: "finite",
        }),
        createConnectQueryKey({
          schema: ProjectService.method.listBoards,
          cardinality: "finite",
        }),
        createConnectQueryKey({
          schema: ProjectService.method.getBootstrap,
          cardinality: "finite",
        }),
      ];
    case WatchResource.BOARD:
      return [
        createConnectQueryKey({
          schema: ProjectService.method.getBoard,
          cardinality: "finite",
        }),
        createConnectQueryKey({
          schema: ConfigurationService.method.getConfiguration,
          cardinality: "finite",
        }),
      ];
    case WatchResource.ISSUES:
      return [
        createConnectQueryKey({ schema: IssueService, cardinality: undefined }),
        createConnectQueryKey({
          schema: AttachmentService,
          cardinality: undefined,
        }),
        createConnectQueryKey({
          schema: ExecutionService.method.listReadyIssues,
          cardinality: "finite",
        }),
        createConnectQueryKey({
          schema: ExecutionService.method.listBlockedIssues,
          cardinality: "finite",
        }),
      ];
    case WatchResource.LOG:
      return [
        createConnectQueryKey({
          schema: RecordService.method.listLogEntries,
          cardinality: "finite",
        }),
      ];
    case WatchResource.APPROVALS:
      return [
        createConnectQueryKey({
          schema: CheckpointService.method.listActionableCheckpoints,
          cardinality: "finite",
        }),
      ];
    case WatchResource.UNSPECIFIED:
      return [];
  }
}

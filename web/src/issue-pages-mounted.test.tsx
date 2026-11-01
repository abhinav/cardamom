// @vitest-environment jsdom

import { createRouterTransport } from "@connectrpc/connect";
import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { toBoardScopeMessage } from "./board-scope.ts";
import { IssueService, IssueStatus, IssueType } from "./gen/cardamom/private/v1/issue_pb.ts";
import { defaultBoardView, type IssueGrouping } from "./issue-collection.ts";
import { buildIssueStreams, useIssueQueries } from "./issue-pages.ts";

afterEach(cleanup);

describe("mounted collection queries", () => {
  it.each([
    { grouping: "status" as IssueGrouping, key: `status:${IssueStatus.UNRESOLVED}` },
    { grouping: "type" as IssueGrouping, key: `type:${IssueType.UNSPECIFIED}` },
  ])("loads unresolved issues when grouped by $grouping", async ({ grouping, key }) => {
    const transport = createRouterTransport(({ service }) => {
      service(IssueService, {
        listIssues(request) {
          const selected = grouping === "status"
            ? request.statuses.includes(IssueStatus.UNRESOLVED)
            : request.types.length === 1 && request.types[0] === IssueType.UNSPECIFIED;
          return {
            issues: selected ? [{ id: "cm-uncertain", status: IssueStatus.UNRESOLVED }] : [],
            totalCount: selected ? 1 : 0,
          };
        },
      });
    });
    const client = new QueryClient();
    const scope = toBoardScopeMessage({ kind: "board", boardId: "board-1" })!;
    const streams = buildIssueStreams(scope, "board", defaultBoardView, grouping);
    const { result, unmount } = renderHook(() => useIssueQueries(streams), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>
          <TransportProvider transport={transport}>{children}</TransportProvider>
        </QueryClientProvider>
      ),
    });
    await waitFor(() => {
      const unresolved = result.current.state.streams.find((stream) => stream.key === key);
      expect(unresolved?.status).toBe("ready");
      expect(unresolved?.totalCount).toBe(1);
      expect(unresolved?.issues.map((issue) => issue.id)).toEqual(["cm-uncertain"]);
    });
    unmount();
    client.clear();
  });
});

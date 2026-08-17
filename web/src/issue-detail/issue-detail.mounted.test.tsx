// @vitest-environment jsdom

import { create } from "@bufbuild/protobuf";
import { createRouterTransport } from "@connectrpc/connect";
import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import type { AttachmentClient } from "../api.ts";
import { AttachmentService } from "../gen/cardamom/private/v1/attachment_pb.ts";
import {
  IssueDetailSchema,
  IssueLifecycle,
  IssueService,
  IssueStatus,
  IssueSummarySchema,
  IssueType,
  RelatedIssueSchema,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import { RecordService } from "../gen/cardamom/private/v1/record_pb.ts";
import { AccessMode } from "../gen/cardamom/private/v1/project_pb.ts";
import { unaryRouteQueryOptions } from "../query-runtime.ts";
import { ServerAccessProvider } from "../server-access.tsx";
import { IssueDetailPage, RelationshipBand } from "./issue-detail.tsx";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

describe("issue detail interactions", () => {
  it("shows a completed pin while the issue query remains stale", async () => {
    const issueId = "cm-task";
    const detail = create(IssueDetailSchema, {
      issue: create(IssueSummarySchema, {
        id: issueId,
        boardId: "board-1",
        title: "Pin this issue",
        lifecycle: IssueLifecycle.OPEN,
        status: IssueStatus.READY,
        type: IssueType.TASK,
      }),
    });
    const getIssue = vi.fn(() => ({ issue: detail }));
    const pinBoardIssue = vi.fn(() => ({
      issue: create(RelatedIssueSchema, {
        id: issueId,
        boardId: "board-1",
        title: detail.issue?.title,
      }),
      changed: true,
    }));
    const transport = createRouterTransport(({ service }) => {
      service(IssueService, { getIssue, pinBoardIssue });
      service(RecordService, {
        listLogEntries: () => ({ logEntries: [] }),
        getState: () => ({}),
      });
      service(AttachmentService, {
        listAttachments: () => ({ attachments: [] }),
      });
    });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    await queryClient.fetchQuery(
      unaryRouteQueryOptions(
        IssueService.method.getIssue,
        { issueId },
        transport,
      ),
    );
    const container = document.createElement("div");
    const root = createRoot(container);

    await act(async () => {
      root.render(
        <ServerAccessProvider accessMode={AccessMode.READ_WRITE}>
          <TransportProvider transport={transport}>
            <QueryClientProvider client={queryClient}>
              <MemoryRouter>
                <IssueDetailPage
                  actor="Rawls"
                  attachmentClient={{} as AttachmentClient}
                  collapsedDetailsBoardIds={[]}
                  expectedBoardId="board-1"
                  issueId={issueId}
                  relationFocus="hierarchy"
                  relationsOpen
                  selectLabel={vi.fn()}
                  setDetailsCollapsed={vi.fn()}
                  setRelationFocus={vi.fn()}
                  setRelationsOpen={vi.fn()}
                />
              </MemoryRouter>
            </QueryClientProvider>
          </TransportProvider>
        </ServerAccessProvider>,
      );
    });
    const pinButton = findButton(container, "Pin to board");

    await act(async () => {
      pinButton.click();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(pinBoardIssue).toHaveBeenCalledOnce();
    expect(getIssue.mock.calls.length).toBeGreaterThan(1);
    expect(findButton(container, "Unpin from board")).toBeDefined();

    await act(async () => root.unmount());
  });

  it("requests the selected relation focus from the shared preferences owner", async () => {
    const issueId = "cm-relations";
    const detail = create(IssueDetailSchema, {
      issue: create(IssueSummarySchema, {
        id: issueId,
        boardId: "board-1",
        title: "Current issue",
      }),
      dependents: [
        create(RelatedIssueSchema, {
          id: "cm-dependent",
          title: "Dependent issue",
        }),
      ],
      containment: {
        nodes: [
          {
            issue: { id: issueId, title: "Current issue" },
            selectedPath: true,
          },
        ],
      },
    });
    const setRelationFocus = vi.fn();
    const transport = createRouterTransport(() => {});
    const queryClient = new QueryClient();
    const container = document.createElement("div");
    const root = createRoot(container);

    await act(async () => {
      root.render(
        <TransportProvider transport={transport}>
          <QueryClientProvider client={queryClient}>
            <MemoryRouter>
              <RelationshipBand
                addDependency={vi.fn()}
                dependencyQuery=""
                detail={detail}
                pending={false}
                relationFocus="hierarchy"
                relationsOpen
                removeDependency={vi.fn()}
                setDependencyQuery={vi.fn()}
                setRelationFocus={setRelationFocus}
                setRelationsOpen={vi.fn()}
              />
            </MemoryRouter>
          </QueryClientProvider>
        </TransportProvider>,
      );
    });

    const relationTabs = [...container.querySelectorAll<HTMLElement>(
      '[role="tab"]',
    )];
    expect(relationTabs.map((tab) => tab.getAttribute("aria-label"))).toEqual([
      "Dependencies 0",
      "Hierarchy 1",
      "Dependents 1",
    ]);
    expect(relationTabs[1]?.getAttribute("aria-selected")).toBe("true");

    await act(async () => relationTabs[2]?.click());

    expect(setRelationFocus).toHaveBeenCalledWith("dependents");

    await act(async () => root.unmount());
  });
});

function findButton(container: HTMLElement, label: string): HTMLButtonElement {
  const button = [...container.querySelectorAll("button")].find(
    (candidate) => candidate.textContent?.includes(label),
  );
  if (button === undefined) {
    throw new Error(`button not found: ${label}`);
  }
  return button;
}

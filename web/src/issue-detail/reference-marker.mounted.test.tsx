// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { create } from "@bufbuild/protobuf";
import { createRouterTransport } from "@connectrpc/connect";
import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";

import type { AttachmentClient } from "../api.ts";
import { AttachmentService } from "../gen/cardamom/private/v1/attachment_pb.ts";
import { type GetIssueRequest, IssueDetailSchema, IssueReferenceMarkerSchema, IssueService, IssueStatus, IssueType } from "../gen/cardamom/private/v1/issue_pb.ts";
import { RecordService } from "../gen/cardamom/private/v1/record_pb.ts";
import { IssueDetailPage } from "./issue-detail.tsx";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it("opens an old board link as a copyable marker without reading issue records", async () => {
  vi.stubGlobal("matchMedia", vi.fn(() => ({ matches: true })));
  const replacement = "cm-old-00112233445566778899aabbccddeeff";
  const marker = create(IssueReferenceMarkerSchema, {
    id: "cm-old",
    candidates: [
      { id: replacement, boardId: "board-other", available: true, title: "First candidate", source: { sourceId: "other", storeLineageId: "lineage-other" } },
      { id: "cm-second", boardId: "board-other", available: true },
      { id: "cm-absent", boardId: "board-absent" },
    ],
  });
  const listLogEntries = vi.fn(() => ({ logEntries: [] }));
  const getState = vi.fn(() => ({}));
  const getIssue = vi.fn((request: GetIssueRequest) => request.issueId === "cm-old"
    ? { marker }
    : { issue: create(IssueDetailSchema, { issue: {
      id: replacement, boardId: "board-other", title: "Candidate hover details",
      status: IssueStatus.READY, type: IssueType.TASK, priority: 1,
    } }) });
  const transport = createRouterTransport(({ service }) => {
    service(IssueService, { getIssue });
    service(RecordService, { listLogEntries, getState });
  });
  const writeText = vi.fn(async () => undefined);
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
  render(
    <TransportProvider transport={transport}>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter initialEntries={["/board/board-original/issue/cm-old"]}>
          <IssueDetailPage
            actor="reader" attachmentClient={{} as AttachmentClient}
            collapsedDetailsBoardIds={[]} expectedBoardId="board-original" issueId="cm-old"
            relationFocus="hierarchy" relationsOpen selectLabel={vi.fn()}
            setDetailsCollapsed={vi.fn()} setRelationFocus={vi.fn()} setRelationsOpen={vi.fn()}
          />
        </MemoryRouter>
      </QueryClientProvider>
    </TransportProvider>,
  );
  expect(await screen.findByRole("heading", { name: "Ambiguous issue reference" })).toBeVisible();
  const candidateLink = screen.getByRole("link", { name: replacement });
  expect(candidateLink).toHaveAttribute("href", `/source/other/board/board-other/issue/${replacement}`);
  expect(screen.getByText("Title unresolved")).toBeVisible();
  expect(screen.getByText("Issue unavailable locally")).toBeVisible();
  expect(screen.queryByRole("link", { name: "cm-absent" })).toBeNull();
  fireEvent.pointerEnter(candidateLink.closest(".issue-reference-pill")!, { pointerType: "mouse" });
  expect(await screen.findByRole("tooltip")).toHaveTextContent("Candidate hover details");
  expect(getIssue).toHaveBeenCalledWith(expect.objectContaining({
    issueId: replacement, boardId: "board-other",
    source: expect.objectContaining({ sourceId: "other", storeLineageId: "lineage-other" }),
  }), expect.anything());
  fireEvent.click(screen.getByRole("button", { name: `Copy issue ID %${replacement}` }));
  await waitFor(() => expect(writeText).toHaveBeenCalledWith(`%${replacement}`));
  expect(listLogEntries).not.toHaveBeenCalled();
  expect(getState).not.toHaveBeenCalled();
});

it("does not request marker records while ordinary issue data is a placeholder", async () => {
  const markerReady = Promise.withResolvers<void>();
  const marker = create(IssueReferenceMarkerSchema, { id: "cm-old" });
  const getIssue = vi.fn(async (request: GetIssueRequest) => {
    if (request.issueId === "cm-old") {
      await markerReady.promise;
      return { marker };
    }
    return { issue: create(IssueDetailSchema, { issue: {
      id: "cm-current", boardId: "board-1", title: "Current issue",
      status: IssueStatus.READY, type: IssueType.TASK,
    } }) };
  });
  const logIDs: string[] = [];
  const stateIDs: string[] = [];
  const transport = createRouterTransport(({ service }) => {
    service(IssueService, { getIssue });
    service(RecordService, {
      listLogEntries: (request) => { logIDs.push(request.issueId); return { logEntries: [] }; },
      getState: (request) => { stateIDs.push(request.issueId); return {}; },
    });
    service(AttachmentService, { listAttachments: () => ({ attachments: [] }) });
  });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = (issueId: string) => (
    <TransportProvider transport={transport}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <IssueDetailPage
            actor="reader" attachmentClient={{} as AttachmentClient}
            collapsedDetailsBoardIds={[]} expectedBoardId="board-1" issueId={issueId}
            relationFocus="hierarchy" relationsOpen selectLabel={vi.fn()}
            setDetailsCollapsed={vi.fn()} setRelationFocus={vi.fn()} setRelationsOpen={vi.fn()}
          />
        </MemoryRouter>
      </QueryClientProvider>
    </TransportProvider>
  );
  const rendered = render(view("cm-current"));
  await screen.findByRole("heading", { name: "Current issue" });
  await waitFor(() => expect(stateIDs).toEqual(["cm-current"]));
  rendered.rerender(view("cm-old"));
  await waitFor(() => expect(getIssue).toHaveBeenCalledWith(
    expect.objectContaining({ issueId: "cm-old" }), expect.anything(),
  ));
  try {
    expect(logIDs).toEqual(["cm-current"]);
    expect(stateIDs).toEqual(["cm-current"]);
  } finally {
    markerReady.resolve();
  }
  await screen.findByRole("heading", { name: "Ambiguous issue reference" });
  expect(logIDs).toEqual(["cm-current"]);
  expect(stateIDs).toEqual(["cm-current"]);
});

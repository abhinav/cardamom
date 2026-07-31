import { create } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";
import { createRouterTransport } from "@connectrpc/connect";
import {
  createInfiniteQueryOptions,
  TransportProvider,
} from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import type { AttachmentClient } from "../api.ts";
import {
  AttachmentSchema,
  AttachmentService,
  BlobAvailability,
} from "../gen/cardamom/private/v1/attachment_pb.ts";
import { MarkdownContentSchema } from "../gen/cardamom/private/v1/content_pb.ts";
import {
  AncestorContextSchema,
  CheckpointOutcome,
  IssueDetailSchema,
  IssueLifecycle,
  IssueService,
  RelatedIssueSchema,
  IssueSort,
  IssueStatus,
  IssueSummarySchema,
  IssueType,
  SortDirection,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import {
  LogEntrySchema,
  RecordService,
  StateRecordSchema,
} from "../gen/cardamom/private/v1/record_pb.ts";
import { AccessMode } from "../gen/cardamom/private/v1/project_pb.ts";
import { LifecycleAction } from "../issue-lifecycle.ts";
import {
  attachmentListInput,
  nextAttachmentPageToken,
} from "../attachments.tsx";
import { unaryRouteQueryOptions } from "../query-runtime.ts";
import { ServerAccessProvider } from "../server-access.tsx";
import {
  addIssueLogEntryInput,
  changeIssueLifecycle,
  dependencyCandidates,
  dependencySearchInput,
  CurrentIssueState,
  editIssueMetadataInput,
  editIssueDependencyInput,
  IssueActions,
  IssueDetailPage,
  IssueHeader,
  LogEntryList,
  issueMetadataDraft,
  PrimaryRecord,
  RelationshipBand,
  RelationshipList,
  resolveIssueCheckpointInput,
} from "./issue-detail.tsx";

describe("issue detail presentation", () => {
  it("presents the current issue ID as a percent-prefixed copy pill", () => {
    const markup = renderToStaticMarkup(
      IssueHeader({
        summary: create(IssueSummarySchema, {
          id: "cm-43px",
          title: "Summary backend",
        }),
        externalKeys: [],
      }),
    );

    expect(markup).toContain("%cm-43px");
    expect(markup).toContain('aria-label="Copy issue ID %cm-43px"');
    expect(markup).not.toContain(">cm-43px<");
  });

  it("orders containment breadcrumbs and links only ancestors", () => {
    const markup = renderToStaticMarkup(
      createElement(
        MemoryRouter,
        null,
        IssueHeader({
          ancestors: [
            create(AncestorContextSchema, {
              issue: create(RelatedIssueSchema, {
                id: "cm-root",
                title: "Root workstream",
              }),
            }),
            create(AncestorContextSchema, {
              issue: create(RelatedIssueSchema, {
                id: "cm-parent",
                title: "Parent workstream",
              }),
            }),
          ],
          summary: create(IssueSummarySchema, {
            id: "cm-current",
            title: "Current task",
          }),
          externalKeys: [],
        }),
      ),
    );

    const positions = ["%cm-root", "%cm-parent", "%cm-current"].map(
      (reference) => markup.indexOf(reference),
    );
    expect(positions).toEqual([...positions].sort((left, right) => left - right));
    expect(positions).not.toContain(-1);
    expect(markup).toContain('aria-label="Issue containment"');
    expect(markup).toContain('href="/issues/cm-root"');
    expect(markup).toContain('href="/issues/cm-parent"');
    expect(markup).not.toContain('href="/issues/cm-current"');
    expect(markup).toContain('aria-current="page"');
    expect(markup).toContain('title="Root workstream"');
    expect(markup).toContain('title="Parent workstream"');
    expect(markup).toContain('title="Current task"');
  });

  it("renders distinct producer keys in supplied order as noninteractive metadata", () => {
    const longKey = `producer:${"a".repeat(96)}`;
    const markup = renderToStaticMarkup(
      IssueHeader({
        summary: create(IssueSummarySchema, {
          id: "cm-43px",
          title: "Summary backend",
        }),
        externalKeys: [" producer:z ", longKey],
      }),
    );

    expect(markup).toContain(
      '<span class="metadata-chip issue-external-key"> producer:z </span>' +
        `<span class="metadata-chip issue-external-key">${longKey}</span>`,
    );
    expect(markup).not.toContain("Copy producer key");
  });

  it("renders summary and details from the renamed protocol shape", () => {
    const detail = create(IssueDetailSchema, {
      issue: create(IssueSummarySchema, {
        id: "cm-43px",
        boardId: "board-1",
        title: "Summary backend",
        labels: ["backend"],
      }),
      summary: create(MarkdownContentSchema, {
        source: "Concise contract.",
        renderedHtml: "<p>Concise contract.</p>",
      }),
      details: create(MarkdownContentSchema, {
        source: "Expanded material.",
        renderedHtml: "<p>Expanded material.</p>",
      }),
      result: create(MarkdownContentSchema, {
        source: "Completed outcome.",
        renderedHtml: "<p>Completed outcome.</p>",
      }),
    });

    const content = PrimaryRecord({ detail, selectLabel: vi.fn() });
    expect(content).not.toBeNull();
    if (content === null) {
      throw new Error("primary issue record was not rendered");
    }
    const markup = renderToStaticMarkup(content);

    expect(markup).toContain("<h2 id=\"summary-title\">Summary</h2>");
    expect(markup).toContain("Concise contract.");
    expect(markup).toContain("<h2 id=\"details-title\">Details</h2>");
    expect(markup).toContain("Expanded material.");
    expect(markup).not.toContain("state-title");
    expect(markup).toContain("<h2 id=\"result-title\">Result</h2>");
    expect(markup).toContain("Completed outcome.");
    expect(markup).toContain('class="issue-detail-section issue-details" open=""');
    expect(markup).not.toContain("Description");
  });

  it("orders stable records for reading and omits empty optional sections", () => {
    const detail = create(IssueDetailSchema, {
      issue: create(IssueSummarySchema, {
        id: "cm-reading-order",
        boardId: "board-1",
        title: "Reading order",
      }),
      summary: create(MarkdownContentSchema, {
        source: "Visible summary.",
        renderedHtml: "<p>Visible summary.</p>",
      }),
    });

    const content = PrimaryRecord({ detail, selectLabel: vi.fn() });
    expect(content).not.toBeNull();
    if (content === null) {
      throw new Error("primary issue record was not rendered");
    }
    const markup = renderToStaticMarkup(content);

    expect(markup.indexOf('id="record-title"')).toBeLessThan(
      markup.indexOf('id="summary-title"'),
    );
    expect(markup).not.toContain('id="details-title"');
    expect(markup).not.toContain('id="result-title"');
    expect(markup).not.toContain("No details.");
    expect(markup).not.toContain("No result recorded.");
  });

  it("keeps the complete reading sequence ahead of secondary controls", async () => {
    const issueId = "cm-reading-flow";
    const detail = create(IssueDetailSchema, {
      issue: create(IssueSummarySchema, {
        id: issueId,
        boardId: "board-1",
        title: "Reading flow",
        lifecycle: IssueLifecycle.OPEN,
        status: IssueStatus.READY,
        type: IssueType.TASK,
      }),
      summary: create(MarkdownContentSchema, {
        source: "Summary record.",
        renderedHtml: "<p>Summary record.</p>",
      }),
      details: create(MarkdownContentSchema, {
        source: "Details record.",
        renderedHtml: "<p>Details record.</p>",
      }),
      result: create(MarkdownContentSchema, {
        source: "Result record.",
        renderedHtml: "<p>Result record.</p>",
      }),
      prerequisites: [
        create(RelatedIssueSchema, {
          id: "cm-prerequisite",
          title: "Prerequisite",
        }),
      ],
    });
    const serviceFirst = create(LogEntrySchema, {
      id: "log-service-first",
      payload: {
        case: "post",
        value: {
          actor: "observer",
          body: {
            source: "Service first.",
            renderedHtml: "<p>Service first.</p>",
          },
          createdAt: create(TimestampSchema, { seconds: 300n }),
        },
      },
    });
    const serviceSecond = create(LogEntrySchema, {
      id: "log-service-second",
      payload: {
        case: "post",
        value: {
          actor: "observer",
          body: {
            source: "Service second.",
            renderedHtml: "<p>Service second.</p>",
          },
          createdAt: create(TimestampSchema, { seconds: 100n }),
        },
      },
    });
    const state = create(StateRecordSchema, {
      body: {
        source: "State record.",
        renderedHtml: "<p>State record.</p>",
      },
    });
    const transport = createRouterTransport(({ service }) => {
      service(IssueService, {
        getIssue: () => ({ issue: detail }),
      });
      service(RecordService, {
        listLogEntries: (request) => {
          expect(request.direction).toBe(SortDirection.ASCENDING);
          return { logEntries: [serviceFirst, serviceSecond] };
        },
        getState: () => ({ state }),
      });
      service(AttachmentService, {
        listAttachments: () => ({
          attachments: [
            create(AttachmentSchema, {
              id: "att-reading-flow",
              boardId: "board-1",
              issueId,
              filename: "reading-flow.txt",
              availability: BlobAvailability.VERIFIED,
            }),
          ],
        }),
      });
    });
    const queryClient = new QueryClient();
    await queryClient.fetchQuery(
      unaryRouteQueryOptions(
        IssueService.method.getIssue,
        { issueId },
        transport,
      ),
    );
    await queryClient.fetchQuery(
      unaryRouteQueryOptions(
        RecordService.method.listLogEntries,
        { issueId, direction: SortDirection.ASCENDING },
        transport,
      ),
    );
    await queryClient.fetchQuery(
      unaryRouteQueryOptions(
        RecordService.method.getState,
        { issueId },
        transport,
      ),
    );
    await queryClient.fetchInfiniteQuery(
      createInfiniteQueryOptions(
        AttachmentService.method.listAttachments,
        attachmentListInput("board-1", issueId),
        {
          transport,
          pageParamKey: "pageToken",
          getNextPageParam: nextAttachmentPageToken,
        },
      ),
    );

    const renderPage = (accessMode: AccessMode) => renderToStaticMarkup(
      createElement(
        ServerAccessProvider,
        { accessMode },
        createElement(
          TransportProvider,
          { transport },
          createElement(
            QueryClientProvider,
            { client: queryClient },
            createElement(
              MemoryRouter,
              null,
              createElement(IssueDetailPage, {
                actor: "observer",
                attachmentClient: {} as AttachmentClient,
                collapsedDetailsBoardIds: [],
                issueId,
                relationsOpen: true,
                selectLabel: vi.fn(),
                setDetailsCollapsed: vi.fn(),
                setRelationsOpen: vi.fn(),
              }),
            ),
          ),
        ),
      ),
    );
    const markup = renderPage(AccessMode.READ_WRITE);
    const positions = [
      'id="record-title"',
      'id="summary-title"',
      'id="details-title"',
      'id="result-title"',
      `id="attachments-${issueId}"`,
      'id="relations-title"',
      'id="log-title"',
      'id="state-title"',
      'id="log-body"',
      "Add a file",
    ].map((marker) => markup.indexOf(marker));

    expect(positions).toEqual([...positions].sort((left, right) => left - right));
    expect(positions).not.toContain(-1);
    expect(markup).toContain("<h2 id=\"dependencies-title\">Dependencies</h2>");
    expect(markup).toContain("<h2 id=\"hierarchy-title\">Hierarchy</h2>");
    expect(markup).toContain("<h2 id=\"dependents-title\">Dependents</h2>");
    expect(markup.indexOf("Service first.")).toBeLessThan(
      markup.indexOf("Service second."),
    );
    expect(markup).toContain(
      '<details class="issue-actions"><summary>Issue actions</summary>',
    );
    expect(markup).not.toContain('id="lifecycle-title"');
    expect(markup).toContain(
      '<details class="issue-detail-section issue-relations" open=""><summary>',
    );

    const readOnlyMarkup = renderPage(AccessMode.READ_ONLY);
    expect(readOnlyMarkup).toContain("Summary record.");
    expect(readOnlyMarkup).toContain("reading-flow.txt");
    expect(readOnlyMarkup).toContain("Service first.");
    expect(readOnlyMarkup).toContain("Dependencies");
    expect(readOnlyMarkup).not.toContain("Issue actions");
    expect(readOnlyMarkup).not.toContain("Add dependency");
    expect(readOnlyMarkup).not.toContain(">Remove</button>");
    expect(readOnlyMarkup).not.toContain("Log composer");
    expect(readOnlyMarkup).not.toContain("Add log entry");
    expect(readOnlyMarkup).not.toContain("Attachment controls");
    expect(readOnlyMarkup).not.toContain("Add a file");
  });

  it("omits Relations when hierarchy contains only the current issue", () => {
    const issueId = "cm-unrelated";
    const detail = create(IssueDetailSchema, {
      issue: create(IssueSummarySchema, {
        id: issueId,
        boardId: "board-1",
        title: "Unrelated issue",
      }),
      containment: {
        nodes: [
          {
            issue: {
              id: issueId,
              title: "Unrelated issue",
            },
            selectedPath: true,
          },
        ],
      },
    });
    const transport = createRouterTransport(() => {});
    const markup = renderToStaticMarkup(
      createElement(
        TransportProvider,
        { transport },
        createElement(
          QueryClientProvider,
          { client: new QueryClient() },
          createElement(
            MemoryRouter,
            null,
            createElement(RelationshipBand, {
              addDependency: vi.fn(),
              dependencyQuery: "",
              detail,
              pending: false,
              relationsOpen: true,
              removeDependency: vi.fn(),
              setDependencyQuery: vi.fn(),
              setRelationsOpen: vi.fn(),
            }),
          ),
        ),
      ),
    );

    expect(markup).not.toContain('id="relations-title"');
    expect(markup).not.toContain("Dependencies");
    expect(markup).not.toContain("Hierarchy");
    expect(markup).not.toContain("Dependents");
  });

  it("keeps Relations collapsed when the remembered preference is closed", () => {
    const issueId = "cm-collapsed-relations";
    const detail = create(IssueDetailSchema, {
      issue: create(IssueSummarySchema, {
        id: issueId,
        boardId: "board-1",
        title: "Collapsed relations",
      }),
      prerequisites: [
        create(RelatedIssueSchema, {
          id: "cm-prerequisite",
          title: "Prerequisite",
        }),
      ],
    });
    const transport = createRouterTransport(() => {});
    const markup = renderToStaticMarkup(
      createElement(
        TransportProvider,
        { transport },
        createElement(
          QueryClientProvider,
          { client: new QueryClient() },
          createElement(
            MemoryRouter,
            null,
            createElement(RelationshipBand, {
              addDependency: vi.fn(),
              dependencyQuery: "",
              detail,
              pending: false,
              relationsOpen: false,
              removeDependency: vi.fn(),
              setDependencyQuery: vi.fn(),
              setRelationsOpen: vi.fn(),
            }),
          ),
        ),
      ),
    );

    expect(markup).toContain(
      '<details class="issue-detail-section issue-relations"><summary>',
    );
    expect(markup).not.toContain(
      '<details class="issue-detail-section issue-relations" open="">',
    );
  });

  it("keeps retained issue content stable during a background refresh", () => {
    const issueId = "cm-refreshing";
    const transport = createRouterTransport(({ service }) => {
      service(IssueService, {
        getIssue: () => new Promise(() => {}),
      });
      service(RecordService, {
        listLogEntries: () => new Promise(() => {}),
      });
      service(AttachmentService, {
        listAttachments: () => ({ attachments: [] }),
      });
    });
    const queryClient = new QueryClient();
    queryClient.setQueryData(
      unaryRouteQueryOptions(
        IssueService.method.getIssue,
        { issueId },
        transport,
      ).queryKey,
      {
        issue: create(IssueDetailSchema, {
          issue: create(IssueSummarySchema, {
            id: issueId,
            boardId: "board-1",
            title: "Retained issue",
          }),
        }),
      },
    );
    queryClient.setQueryData(
      unaryRouteQueryOptions(
        RecordService.method.listLogEntries,
        { issueId },
        transport,
      ).queryKey,
      { logEntries: [] },
    );

    const markup = renderToStaticMarkup(
      createElement(
        TransportProvider,
        { transport },
        createElement(
          QueryClientProvider,
          { client: queryClient },
          createElement(
            MemoryRouter,
            null,
            createElement(IssueDetailPage, {
              actor: "observer",
              attachmentClient: {} as AttachmentClient,
              collapsedDetailsBoardIds: [],
              issueId,
              relationsOpen: true,
              selectLabel: vi.fn(),
              setDetailsCollapsed: vi.fn(),
              setRelationsOpen: vi.fn(),
            }),
          ),
        ),
      ),
    );

    expect(markup).toContain("Retained issue");
    expect(markup).not.toContain("Refreshing issue");
    expect(markup).not.toContain("Refreshing log");
  });

  it("labels pinned recovery content as State", () => {
    const markup = renderToStaticMarkup(
      CurrentIssueState({
        state: create(StateRecordSchema, {
          body: {
            source: "## Current state\n\nReady.",
            renderedHtml: "<h2>Current state</h2><p>Ready.</p>",
          },
          nextAction: {
            source: "Run the acceptance probe.",
            renderedHtml: "<p>Run the acceptance probe.</p>",
          },
        }),
      }),
    );

    expect(markup).toContain('<h2 id="state-title">State</h2>');
    expect(markup).toContain("<h2>Current state</h2>");
    expect(markup).toContain("<h3>Next action</h3>");
    expect(markup).toContain("Run the acceptance probe.");
    expect(markup.indexOf("<h2>Current state</h2>")).toBeLessThan(
      markup.indexOf("<h3>Next action</h3>"),
    );
    expect(markup).not.toContain(
      '<h2 id="state-title">Current State</h2>',
    );
  });

  it("leaves legacy State Markdown unchanged without inventing an action block", () => {
    const markup = renderToStaticMarkup(
      CurrentIssueState({
        state: create(StateRecordSchema, {
          body: {
            source: "## Next action\n\nLegacy recovery prose.",
            renderedHtml: "<h2>Next action</h2><p>Legacy recovery prose.</p>",
          },
        }),
      }),
    );

    expect(markup).toContain(
      "<h2>Next action</h2><p>Legacy recovery prose.</p>",
    );
    expect(markup).not.toContain("issue-next-action");
    expect(markup).not.toContain("<h3>Next action</h3>");
  });

  it("omits State when neither recovery field has content", () => {
    const markup = renderToStaticMarkup(
      CurrentIssueState({
        state: create(StateRecordSchema, {
          body: { source: "  " },
          nextAction: { source: "\n" },
        }),
      }),
    );

    expect(markup).toBe("");
  });

  it("presents effective status on direct relationship rows", () => {
    const content = RelationshipList({
      empty: "No dependencies.",
      issues: [
        create(RelatedIssueSchema, {
          id: "cm-prerequisite",
          title: "Completed prerequisite",
          status: IssueStatus.CLOSED,
        }),
      ],
      pending: false,
    });
    const markup = renderToStaticMarkup(
      createElement(MemoryRouter, null, content),
    );

    expect(markup).toContain('data-issue-state="closed"');
    expect(markup).toContain('aria-label="Closed"');
    expect(markup).toContain("Completed prerequisite");
    expect(markup).toContain("cm-prerequisite");
  });

  it("marks State snapshots without adding a label to ordinary posts", () => {
    const markup = renderToStaticMarkup(
      LogEntryList({
        entries: [
          create(LogEntrySchema, {
            id: "log-post",
            payload: {
              case: "post",
              value: {
                actor: "observer",
                body: {
                  source: "Observed behavior",
                  renderedHtml: "<p>Observed behavior</p>",
                },
              },
            },
          }),
          create(LogEntrySchema, {
            id: "log-state",
            payload: {
              case: "stateSnapshot",
              value: {
                author: "engineer",
                body: {
                  source: "Recovery state",
                  renderedHtml: "<p>Recovery state</p>",
                },
                nextAction: {
                  source: "Run the next diagnostic.",
                  renderedHtml: "<p>Run the next diagnostic.</p>",
                },
              },
            },
          }),
        ],
      }),
    );

    expect(markup).toContain("Observed behavior");
    expect(markup).toContain("Recovery state");
    expect(markup).toContain("State snapshot");
    expect(markup.match(/State snapshot/g)).toHaveLength(1);
    expect(markup).toContain("<h3>Planned next action</h3>");
    expect(markup).toContain("Run the next diagnostic.");
    expect(markup.indexOf("Recovery state")).toBeLessThan(
      markup.indexOf("<h3>Planned next action</h3>"),
    );
    expect(markup.match(/Planned next action/g)).toHaveLength(1);
  });
});

describe("issue detail RPC boundaries", () => {
  it("prefills metadata from the authoritative issue detail", () => {
    const detail = create(IssueDetailSchema, {
      issue: create(IssueSummarySchema, {
        id: "cm-task",
        title: "Existing task",
        type: IssueType.TASK,
        priority: 3,
        labels: ["area:web", "priority:high"],
      }),
      summary: create(MarkdownContentSchema, { source: "Stable summary" }),
      details: create(MarkdownContentSchema, { source: "Expanded details" }),
      containment: {
        nodes: [
          {
            issue: { id: "cm-task" },
            parentId: "cm-parent",
          },
        ],
      },
    });

    expect(issueMetadataDraft(detail)).toEqual({
      title: "Existing task",
      type: IssueType.TASK,
      priority: 3,
      summary: "Stable summary",
      details: "Expanded details",
      labels: "area:web, priority:high",
      parent: "cm-parent",
    });
  });

  it("replaces every metadata field and preserves clearing presence", () => {
    const input = editIssueMetadataInput(
      "cm-task",
      "  issue-detail-worker  ",
      {
        title: "  Revised task  ",
        type: IssueType.WORKSTREAM,
        priority: 2,
        summary: "",
        details: "",
        labels: " area:web, priority:high, area:web ",
        parent: "",
      },
    );

    expect(input).toEqual({
      issueId: "cm-task",
      title: "Revised task",
      type: IssueType.WORKSTREAM,
      priority: 2,
      summarySource: "",
      detailsSource: "",
      labels: { values: ["area:web", "priority:high"] },
      parentId: "",
      context: expect.objectContaining({ actor: "issue-detail-worker" }),
    });
    expect(input).toHaveProperty("summarySource");
    expect(input).toHaveProperty("detailsSource");
    expect(input).toHaveProperty("parentId");
  });

  it("keeps edit and lifecycle mutations in collapsed Issue actions", () => {
    const markup = renderToStaticMarkup(
      IssueActions({
        actor: "issue-detail-worker",
        changeLifecycle: vi.fn(),
        edit: vi.fn(),
        pending: false,
        summary: create(IssueSummarySchema, {
          id: "cm-task",
          lifecycle: IssueLifecycle.OPEN,
          status: IssueStatus.READY,
          type: IssueType.TASK,
        }),
      }),
    );

    expect(markup).toContain(
      '<details class="issue-actions"><summary>Issue actions</summary>',
    );
    expect(markup).toContain("Edit issue");
    expect(markup).toContain("Claim");
    expect(markup).toContain("Mark done");
    expect(markup).toContain("Cancel");
    expect(markup).not.toContain('open=""');
  });

  it("searches the current board and excludes existing relationships", () => {
    const input = dependencySearchInput(
      "board-1",
      "  dependency  ",
    );
    const candidates = dependencyCandidates(
      [
        create(IssueSummarySchema, {
          id: "cm-current",
          title: "Current issue",
        }),
        create(IssueSummarySchema, {
          id: "cm-existing",
          title: "Existing dependency",
        }),
        create(IssueSummarySchema, {
          id: "cm-result",
          title: "Matching dependency",
        }),
      ],
      new Set(["cm-current", "cm-existing"]),
    );

    expect(input).toEqual(
      expect.objectContaining({
        scope: expect.objectContaining({
          selection: { case: "boardId", value: "board-1" },
        }),
        titleQuery: "dependency",
        sort: IssueSort.TITLE,
        direction: SortDirection.ASCENDING,
        limit: 8,
      }),
    );
    expect(candidates.map(({ id }) => id)).toEqual(["cm-result"]);
  });

  it("attributes checkpoint decisions to the selected actor", () => {
    expect(resolveIssueCheckpointInput(
      "cm-checkpoint",
      "  issue-detail-worker  ",
      CheckpointOutcome.APPROVED,
    )).toEqual({
      issueId: "cm-checkpoint",
      context: { actor: "issue-detail-worker" },
      outcome: CheckpointOutcome.APPROVED,
    });
  });

  it("cancels issue roots through ExecutionService and reports the cascade", async () => {
    const cancelIssuesRPC = vi.fn(async () => ({
      issues: [{ id: "cm-parent" }, { id: "cm-dependent" }],
      dependents: 1,
    }));
    const mutations = {
      cancel: cancelIssuesRPC,
    } as unknown as Parameters<typeof changeIssueLifecycle>[0];

    const result = await changeIssueLifecycle(
      mutations,
      "cm-parent",
      "  issue-detail-worker  ",
      LifecycleAction.CANCEL,
    );

    expect(result).toBe("Cancelled cm-parent and 1 descendant.");
    expect(cancelIssuesRPC).toHaveBeenCalledWith({
      rootIssueIds: ["cm-parent"],
      context: expect.objectContaining({ actor: "issue-detail-worker" }),
    });
  });

  it("attributes dependency edits and selects the requested relationship", () => {
    expect(editIssueDependencyInput(
      "cm-child",
      "cm-two",
      "  issue-detail-worker  ",
      "add",
    )).toEqual({
      issueId: "cm-child",
      addPrerequisiteIds: ["cm-two"],
      context: expect.objectContaining({ actor: "issue-detail-worker" }),
    });
  });

  it("attributes appended log entries through RecordService", () => {
    expect(addIssueLogEntryInput(
      "cm-task",
      "  issue-detail-worker  ",
      "Engineering update",
    )).toEqual({
      issueId: "cm-task",
      bodySource: "Engineering update",
      context: expect.objectContaining({ actor: "issue-detail-worker" }),
    });
  });
});

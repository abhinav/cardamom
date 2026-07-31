import { create } from "@bufbuild/protobuf";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import {
  IssueStatus,
  IssueType,
  ListIssuesRequestSchema,
  type IssueSummary,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import type { IssuePageStream } from "./issue-pages.ts";
import { formatIssueTime, KanbanBoard } from "./issue-views.tsx";

describe("issue time presentation", () => {
  it("includes time of day from the most recent issue timestamp", () => {
    const formatter = new Intl.DateTimeFormat("en-US", {
      dateStyle: "medium",
      timeStyle: "short",
      timeZone: "UTC",
    });

    expect(formatIssueTime(issueWithUpdatedTime(1_704_165_840), formatter)).toBe(
      "Jan 2, 2024, 3:24 AM",
    );
  });
});

describe("kanban board", () => {
  it("shows loaded and total issues in each column heading", () => {
    const stream = {
      key: `status:${IssueStatus.READY}`,
      label: "Open",
      request: create(ListIssuesRequestSchema, {
        statuses: [IssueStatus.READY],
        limit: 20,
      }),
      issues: [],
      status: "ready",
      pageCount: 1,
      nextPageToken: "next",
      totalCount: 7,
    } satisfies IssuePageStream;

    const markup = renderToStaticMarkup(KanbanBoard({
      boards: [],
      grouping: "status",
      streams: [stream],
      loadMore: vi.fn(),
      selectLabel: vi.fn(),
      showBoard: false,
    }));

    expect(markup).toContain("<h2 id=\"status:1-title\">Open</h2>");
    expect(markup).toContain("<span>0/7</span>");
    expect(markup).toContain('aria-label="Load more Open issues"');
  });

  it("omits empty columns while preserving the remaining order", () => {
    const markup = renderKanban({
      boards: [],
      grouping: "status",
      streams: [
        pageStream(
          `status:${IssueStatus.READY}`,
          "Open",
          [issueWithStatus("cm-open", IssueStatus.READY)],
        ),
        pageStream(`status:${IssueStatus.BLOCKED}`, "Blocked", []),
        pageStream(
          `status:${IssueStatus.IN_PROGRESS}`,
          "In progress",
          [issueWithStatus("cm-active", IssueStatus.IN_PROGRESS)],
        ),
        {
          ...pageStream(`status:${IssueStatus.WAITING}`, "Waiting", []),
          status: "loading",
        },
      ],
      loadMore: vi.fn(),
      selectLabel: vi.fn(),
      showBoard: false,
    });

    expect(markup).not.toContain(">Blocked</h2>");
    expect(markup).toContain(">Waiting</h2>");
    expect(markup.indexOf(">Open</h2>")).toBeLessThan(
      markup.indexOf(">In progress</h2>"),
    );
    expect(markup.indexOf(">In progress</h2>")).toBeLessThan(
      markup.indexOf(">Waiting</h2>"),
    );
  });

  it("shows one create prompt when every column is empty", () => {
    const markup = renderKanban({
      boards: [],
      grouping: "status",
      streams: [
        pageStream(`status:${IssueStatus.READY}`, "Open", []),
        pageStream(`status:${IssueStatus.BLOCKED}`, "Blocked", []),
      ],
      loadMore: vi.fn(),
      selectLabel: vi.fn(),
      showBoard: false,
    });

    expect(markup).toContain(
      "No issues here. Create a new issue to get started.",
    );
    expect(markup).not.toContain("kanban-column");
  });

  it("omits creation guidance when issue creation is unavailable", () => {
    const markup = renderKanban({
      boards: [],
      canCreateIssue: false,
      grouping: "status",
      streams: [pageStream(`status:${IssueStatus.READY}`, "Open", [])],
      loadMore: vi.fn(),
      selectLabel: vi.fn(),
      showBoard: false,
    });

    expect(markup).toContain("No issues here.");
    expect(markup).not.toContain("Create");
  });
});

function renderKanban(
  props: Parameters<typeof KanbanBoard>[0],
): string {
  return renderToStaticMarkup(
    createElement(
      MemoryRouter,
      null,
      createElement(KanbanBoard, props),
    ),
  );
}

function pageStream(
  key: string,
  label: string,
  issues: readonly IssueSummary[],
): IssuePageStream {
  return {
    key,
    label,
    request: create(ListIssuesRequestSchema, {}),
    issues,
    status: "ready",
    pageCount: 1,
    totalCount: issues.length,
  };
}

function issueWithStatus(id: string, status: IssueStatus): IssueSummary {
  return {
    ...issueWithUpdatedTime(1_704_165_840),
    id,
    status,
  };
}

function issueWithUpdatedTime(seconds: number): IssueSummary {
  return {
    $typeName: "cardamom.private.v1.IssueSummary",
    id: "cm-recent",
    boardId: "board-1",
    title: "Recent work",
    type: IssueType.TASK,
    lifecycle: 1,
    status: IssueStatus.IN_PROGRESS,
    priority: 0,
    labels: [],
    blocked: false,
    createdAt: {
      $typeName: "google.protobuf.Timestamp",
      seconds: 1_700_000_000n,
      nanos: 0,
    },
    updatedAt: {
      $typeName: "google.protobuf.Timestamp",
      seconds: BigInt(seconds),
      nanos: 0,
    },
  };
}

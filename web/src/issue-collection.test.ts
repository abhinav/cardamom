import { describe, expect, it } from "vitest";

import { toBoardScopeMessage } from "./board-scope.ts";
import {
  buildIssueQuery,
  defaultBoardView,
  defaultListView,
  groupIssues,
  parseBoardView,
  type IssueViewPreferences,
} from "./issue-collection.ts";
import {
  IssueSort,
  IssueStatus,
  IssueType,
  SortDirection,
  type IssueSummary,
} from "./gen/cardamom/private/v1/issue_pb.ts";

describe("issue collection queries", () => {
  it("translates persisted filters and ordering into the generated request", () => {
    const scope = toBoardScopeMessage({ kind: "board", boardId: "board-1" });
    const view: IssueViewPreferences = {
      ...defaultBoardView,
      filters: {
        lifecycle: "open",
        status: IssueStatus.BLOCKED,
        type: IssueType.TASK,
        actor: " captain ",
        label: " urgent ",
        query: " storage ",
      },
      sort: "updated",
      direction: "descending",
    };

    const request = buildIssueQuery(scope!, view);

    expect(request.scope).toBe(scope);
    expect(request.lifecycles).toEqual([1]);
    expect(request.statuses).toEqual([IssueStatus.BLOCKED]);
    expect(request.types).toEqual([IssueType.TASK]);
    expect(request.actor).toBe("captain");
    expect(request.labelsAll).toEqual(["urgent"]);
    expect(request.titleQuery).toBe("storage");
    expect(request.sort).toBe(IssueSort.UPDATED_AT);
    expect(request.direction).toBe(SortDirection.DESCENDING);
    expect(request.limit).toBe(100);
  });

  it("limits the default type filter to ordinary work", () => {
    const scope = toBoardScopeMessage({ kind: "board", boardId: "board-1" });

    const request = buildIssueQuery(scope!, defaultBoardView);

    expect(request.types).toEqual([
      IssueType.WORKSTREAM,
      IssueType.TASK,
      IssueType.CHECKPOINT,
    ]);
    expect(request.lifecycles).toEqual([1, 2]);
    expect(request.sort).toBe(IssueSort.UPDATED_AT);
    expect(request.direction).toBe(SortDirection.DESCENDING);
  });

  it("retains an intentional routine-only query", () => {
    const scope = toBoardScopeMessage({ kind: "board", boardId: "board-1" });

    const request = buildIssueQuery(scope!, {
      ...defaultBoardView,
      filters: { ...defaultBoardView.filters, type: IssueType.ROUTINE },
    });

    expect(request.types).toEqual([IssueType.ROUTINE]);
  });
});

describe("kanban grouping", () => {
  it("keeps current status columns stable and omits cancelled work by default", () => {
    const issues = [
      issue("cm-ready", IssueStatus.READY, IssueType.TASK, 1, 10),
      issue("cm-active", IssueStatus.IN_PROGRESS, IssueType.WORKSTREAM, 0, 20),
      issue("cm-waiting", IssueStatus.WAITING, IssueType.TASK, 0, 25),
      issue("cm-closed", IssueStatus.CLOSED, IssueType.TASK, 0, 30),
      issue("cm-cancelled", IssueStatus.CANCELLED, IssueType.TASK, 0, 40),
    ];

    const groups = groupIssues(issues, "status", "current", "all", "natural");

    expect(groups.map((group) => group.label)).toEqual([
      "Ready",
      "Blocked",
      "In progress",
      "Waiting",
      "Closed",
    ]);
    expect(groups.map((group) => group.issues.map((item) => item.id))).toEqual([
      ["cm-ready"],
      [],
      ["cm-active"],
      ["cm-waiting"],
      ["cm-closed"],
    ]);
  });

  it("uses priority for ready work and recent activity for active, waiting, and closed work", () => {
    const groups = groupIssues(
      [
        issue("cm-open-later", IssueStatus.READY, IssueType.TASK, 2, 50),
        issue("cm-open-priority", IssueStatus.READY, IssueType.TASK, 0, 10),
        issue("cm-active-older", IssueStatus.IN_PROGRESS, IssueType.TASK, 0, 20),
        issue("cm-active-newer", IssueStatus.IN_PROGRESS, IssueType.TASK, 3, 40),
        issue("cm-waiting", IssueStatus.WAITING, IssueType.TASK, 0, 45),
        issue("cm-closed-older", IssueStatus.CLOSED, IssueType.TASK, 0, 30),
        issue("cm-closed-newer", IssueStatus.CLOSED, IssueType.TASK, 3, 60),
      ],
      "status",
      "current",
      "all",
      "natural",
    );

    expect(groups.map((group) => group.issues.map((item) => item.id))).toEqual([
      ["cm-open-priority", "cm-open-later"],
      [],
      ["cm-active-newer", "cm-active-older"],
      ["cm-waiting"],
      ["cm-closed-newer", "cm-closed-older"],
    ]);
  });

  it("applies an explicit sort and direction within each group", () => {
    const groups = groupIssues(
      [
        issue("cm-low", IssueStatus.READY, IssueType.TASK, 3),
        issue("cm-high", IssueStatus.READY, IssueType.TASK, 0),
      ],
      "status",
      "open",
      "all",
      "priority",
      "descending",
    );

    expect(groups[0]?.issues.map((item) => item.id)).toEqual([
      "cm-low",
      "cm-high",
    ]);
  });

  it("omits the routine column unless routines are selected explicitly", () => {
    const task = issue("cm-task", IssueStatus.READY, IssueType.TASK, 1);
    const routine = issue("cm-routine", IssueStatus.READY, IssueType.ROUTINE, 1);

    const ordinaryGroups = groupIssues([task], "type", "open", "all");
    const routineGroups = groupIssues(
      [routine],
      "type",
      "open",
      IssueType.ROUTINE,
    );

    expect(ordinaryGroups.map((group) => group.label)).toEqual([
      "Workstream",
      "Task",
      "Checkpoint",
    ]);
    expect(routineGroups.map((group) => group.label)).toEqual(["Routine"]);
    expect(routineGroups[0]?.issues).toEqual([routine]);
  });
});

describe("board view preferences", () => {
  it("defaults to current work with natural ordering", () => {
    expect(defaultBoardView.filters.lifecycle).toBe("current");
    expect(defaultBoardView.sort).toBe("natural");
    expect(defaultBoardView.showEmptyColumns).toBe(false);
  });

  it("loads the empty-column choice while defaulting missing values off", () => {
    expect(parseBoardView({
      ...defaultBoardView,
      showEmptyColumns: true,
    }).showEmptyColumns).toBe(true);
    expect(parseBoardView({
      ...defaultBoardView,
      showEmptyColumns: undefined,
    }).showEmptyColumns).toBe(false);
  });

  it("rejects the removed priority grouping from persisted preferences", () => {
    expect(parseBoardView({
      ...defaultBoardView,
      grouping: "priority",
      filters: { ...defaultBoardView.filters, label: "ux" },
      sort: "updated",
      direction: "ascending",
    })).toEqual({
      ...defaultBoardView,
      filters: { ...defaultBoardView.filters, label: "ux" },
      sort: "updated",
      direction: "ascending",
    });
  });
});

function issue(
  id: string,
  status: IssueStatus,
  type: IssueType,
  priority: number,
  updatedSeconds = 0,
): IssueSummary {
  return {
    $typeName: "cardamom.private.v1.IssueSummary",
    id,
    boardId: "board-1",
    title: id,
    type,
    lifecycle: 1,
    status,
    priority,
    labels: [],
    blocked: status === IssueStatus.BLOCKED,
    updatedAt: {
      $typeName: "google.protobuf.Timestamp",
      seconds: BigInt(updatedSeconds),
      nanos: 0,
    },
  };
}

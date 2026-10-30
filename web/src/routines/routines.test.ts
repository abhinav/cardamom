import { create } from "@bufbuild/protobuf";
import { describe, expect, it, vi } from "vitest";

import { toBoardScopeMessage } from "../board-scope.ts";
import {
  IssueSummarySchema,
  IssueLifecycle,
  IssueStatus,
  IssueType,
  SortDirection,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import { LifecycleAction } from "../issue-lifecycle.ts";
import {
  changeRoutineLifecycle,
  formatRecency,
  routineListInput,
  routinePresentation,
} from "./routines.tsx";

describe("Routines RPC boundary", () => {
  it("distinguishes unknown execution inputs from unrelated disputed metadata", () => {
    const routine = create(IssueSummarySchema, {
      type: IssueType.ROUTINE,
      status: IssueStatus.UNRESOLVED,
      conflicts: [{ id: "lifecycle-conflict", field: "lifecycle" }],
    });
    expect(routinePresentation(routine, "worker", true)).toEqual({ state: "Unresolved", actions: [] });
    routine.lifecycle = IssueLifecycle.OPEN;
    routine.conflicts[0]!.field = "title";
    expect(routinePresentation(routine, "worker", true).actions).toContain(LifecycleAction.CLAIM);
  });
  it("lists open and unresolved routines when retired routines are hidden", () => {
    const scope = toBoardScopeMessage({ kind: "board", boardId: "board-1" })!;

    expect(routineListInput(scope, false)).toEqual(
      expect.objectContaining({
        scope,
        types: [IssueType.ROUTINE],
        lifecycles: [IssueLifecycle.OPEN, IssueLifecycle.UNSPECIFIED],
        direction: SortDirection.DESCENDING,
      }),
    );
  });

  it("keeps the explicit routine type when retired routines are shown", () => {
    const scope = toBoardScopeMessage({ kind: "all" })!;

    expect(routineListInput(scope, true)).toEqual(
      expect.objectContaining({
        types: [IssueType.ROUTINE],
        lifecycles: [
          IssueLifecycle.OPEN,
          IssueLifecycle.CLOSED,
          IssueLifecycle.CANCELLED,
          IssueLifecycle.UNSPECIFIED,
        ],
      }),
    );
  });

  it("derives operational state and domain-valid actions", () => {
    expect(
      routinePresentation(
        create(IssueSummarySchema, {
          type: IssueType.ROUTINE,
          lifecycle: IssueLifecycle.OPEN,
          status: IssueStatus.READY,
        }),
        "browser-actor",
        true,
      ),
    ).toEqual({
      state: "Available",
      actions: [
        LifecycleAction.CLAIM,
        LifecycleAction.CLOSE,
        LifecycleAction.CANCEL,
      ],
    });
    expect(
      routinePresentation(
        create(IssueSummarySchema, {
          type: IssueType.ROUTINE,
          lifecycle: IssueLifecycle.OPEN,
          status: IssueStatus.IN_PROGRESS,
          activeClaim: { actor: "browser-actor" },
        }),
        "browser-actor",
        true,
      ),
    ).toEqual({
      state: "Claimed by browser-actor",
      actions: [
        LifecycleAction.RELEASE,
        LifecycleAction.CLOSE,
        LifecycleAction.CANCEL,
      ],
    });
    expect(
      routinePresentation(
        create(IssueSummarySchema, {
          type: IssueType.ROUTINE,
          lifecycle: IssueLifecycle.OPEN,
          status: IssueStatus.BLOCKED,
          blocked: true,
        }),
        "browser-actor",
        true,
      ),
    ).toEqual({
      state: "Blocked",
      actions: [LifecycleAction.CLOSE, LifecycleAction.CANCEL],
    });
    expect(
      routinePresentation(
        create(IssueSummarySchema, {
          type: IssueType.ROUTINE,
          lifecycle: IssueLifecycle.CLOSED,
          status: IssueStatus.CLOSED,
        }),
        "browser-actor",
        true,
      ),
    ).toEqual({
      state: "Closed",
      actions: [LifecycleAction.REOPEN],
    });
  });

  it("removes mutation actions without effective capability or an actor", () => {
    const routine = create(IssueSummarySchema, {
      type: IssueType.ROUTINE,
      lifecycle: IssueLifecycle.OPEN,
      status: IssueStatus.READY,
    });

    expect(routinePresentation(routine, "browser-actor", false).actions).toEqual(
      [],
    );
    expect(routinePresentation(routine, "", true).actions).toEqual([]);
  });

  it("claims routines through ExecutionService with the browser actor", async () => {
    const claimIssueRPC = vi.fn(async () => ({
      issue: { issue: { id: "cm-routine" } },
    }));
    const mutations = {
      claim: claimIssueRPC,
    } as unknown as Parameters<typeof changeRoutineLifecycle>[0];

    const result = await changeRoutineLifecycle(
      mutations,
      "cm-routine",
      "browser-actor",
      LifecycleAction.CLAIM,
    );

    expect(result?.id).toBe("cm-routine");
    expect(claimIssueRPC).toHaveBeenCalledWith({
      issueId: "cm-routine",
      context: { actor: "browser-actor" },
    });
  });

  it("cancels routines through the execution root operation", async () => {
    const cancelIssuesRPC = vi.fn(async () => ({
      issues: [{ id: "cm-routine" }, { id: "cm-dependent" }],
    }));
    const mutations = {
      cancel: cancelIssuesRPC,
    } as unknown as Parameters<typeof changeRoutineLifecycle>[0];

    const result = await changeRoutineLifecycle(
      mutations,
      "cm-routine",
      "browser-actor",
      LifecycleAction.CANCEL,
    );

    expect(result?.id).toBe("cm-routine");
    expect(cancelIssuesRPC).toHaveBeenCalledWith({
      rootIssueIds: ["cm-routine"],
      context: { actor: "browser-actor" },
    });
  });
});

describe("Routine recency presentation", () => {
  it("formats recency against the provided time", () => {
    const updatedAt = {
      $typeName: "google.protobuf.Timestamp" as const,
      seconds: BigInt(Date.UTC(2026, 6, 18, 17) / 1_000),
      nanos: 0,
    };

    expect(formatRecency(updatedAt, Date.UTC(2026, 6, 18, 19))).toBe(
      "2 hours ago",
    );
  });
});

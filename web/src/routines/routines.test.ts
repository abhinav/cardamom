import { describe, expect, it, vi } from "vitest";

import { toBoardScopeMessage } from "../board-scope.ts";
import {
  IssueLifecycle,
  IssueStatus,
  IssueType,
  SortDirection,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import { LifecycleAction } from "../issue-lifecycle.ts";
import type { PreferencesStorage } from "../preferences.ts";
import {
  changeRoutineLifecycle,
  formatRecency,
  routineListInput,
  loadShowRetired,
  routinePresentation,
  saveShowRetired,
} from "./routines.tsx";

describe("Routines RPC boundary", () => {
  it("lists only open routines when retired routines are hidden", () => {
    const scope = toBoardScopeMessage({ kind: "board", boardId: "board-1" })!;

    expect(routineListInput(scope, false)).toEqual(
      expect.objectContaining({
        scope,
        types: [IssueType.ROUTINE],
        lifecycles: [IssueLifecycle.OPEN],
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
        ],
      }),
    );
  });

  it("derives operational state and domain-valid actions", () => {
    expect(
      routinePresentation(
        {
          lifecycle: IssueLifecycle.OPEN,
          status: IssueStatus.READY,
        },
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
        {
          lifecycle: IssueLifecycle.OPEN,
          status: IssueStatus.IN_PROGRESS,
          activeClaim: { actor: "browser-actor" },
        },
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
        {
          lifecycle: IssueLifecycle.OPEN,
          status: IssueStatus.BLOCKED,
        },
        "browser-actor",
        true,
      ),
    ).toEqual({
      state: "Blocked",
      actions: [LifecycleAction.CLOSE, LifecycleAction.CANCEL],
    });
    expect(
      routinePresentation(
        {
          lifecycle: IssueLifecycle.CLOSED,
          status: IssueStatus.CLOSED,
        },
        "browser-actor",
        true,
      ),
    ).toEqual({
      state: "Closed",
      actions: [LifecycleAction.REOPEN],
    });
  });

  it("removes mutation actions without effective capability or an actor", () => {
    const routine = {
      lifecycle: IssueLifecycle.OPEN,
      status: IssueStatus.READY,
    };

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

describe("Show retired preference", () => {
  it("persists route-local visibility under the Cardamom product key", () => {
    const values = new Map<string, string>();
    const storage: PreferencesStorage = {
      getItem: (key) => values.get(key) ?? null,
      setItem: (key, value) => values.set(key, value),
    };

    expect(loadShowRetired(storage)).toBe(false);
    saveShowRetired(storage, true);
    expect(loadShowRetired(storage)).toBe(true);
    expect(values.get("cardamom.routines.showRetired")).toBe("true");
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

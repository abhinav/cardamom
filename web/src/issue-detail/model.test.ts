import { create } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";
import { describe, expect, it } from "vitest";

import {
  ActiveClaimSchema,
  IssueDetailSchema,
  IssueLifecycle,
  RelatedIssueSchema,
  IssueStatus,
  IssueSummarySchema,
  IssueType,
  WaitingStateSchema,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import {
  LogEntrySchema,
  StateRecordSchema,
} from "../gen/cardamom/private/v1/record_pb.ts";
import {
  availableLifecycleActions,
  describeLifecycleMutation,
  LifecycleAction,
  lifecycleActionLabel,
} from "../issue-lifecycle.ts";
import {
  checkpointDecisionPresentation,
  currentIssueState,
  describeDependencyMutation,
  logEntryPresentation,
  visibleIssueLogEntries,
} from "./model.ts";

describe("availableLifecycleActions", () => {
  it("offers claim or release according to current custody", () => {
    const open = create(IssueSummarySchema, {
      id: "cm-task",
      lifecycle: IssueLifecycle.OPEN,
    });
    const claimed = create(IssueSummarySchema, {
      id: "cm-task",
      lifecycle: IssueLifecycle.OPEN,
      activeClaim: create(ActiveClaimSchema, { actor: "scotty" }),
    });

    expect(availableLifecycleActions(open, "scotty")).toEqual([
      LifecycleAction.CLAIM,
      LifecycleAction.CLOSE,
      LifecycleAction.CANCEL,
    ]);
    expect(availableLifecycleActions(claimed, "scotty")).toEqual([
      LifecycleAction.RELEASE,
      LifecycleAction.CLOSE,
      LifecycleAction.CANCEL,
    ]);
    expect(availableLifecycleActions(claimed, "spock")).toEqual([
      LifecycleAction.CLOSE,
      LifecycleAction.CANCEL,
    ]);
  });

  it("offers reopen for either terminal lifecycle", () => {
    for (const lifecycle of [
      IssueLifecycle.CLOSED,
      IssueLifecycle.CANCELLED,
    ]) {
      const issue = create(IssueSummarySchema, { lifecycle });
      expect(availableLifecycleActions(issue, "scotty")).toEqual([
        LifecycleAction.REOPEN,
      ]);
    }
  });

  it("does not invent controls for an unspecified lifecycle", () => {
    const issue = create(IssueSummarySchema, {
      lifecycle: IssueLifecycle.UNSPECIFIED,
    });

    expect(availableLifecycleActions(issue, "scotty")).toEqual([]);
  });

  it("reserves checkpoint completion for the approval workflow", () => {
    const checkpoint = create(IssueSummarySchema, {
      lifecycle: IssueLifecycle.OPEN,
      type: IssueType.CHECKPOINT,
    });

    expect(availableLifecycleActions(checkpoint, "scotty")).toEqual([
      LifecycleAction.CANCEL,
    ]);
  });

  it("does not offer custody for a blocked task", () => {
    const blockedTask = create(IssueSummarySchema, {
      blocked: true,
      lifecycle: IssueLifecycle.OPEN,
      type: IssueType.TASK,
    });

    expect(availableLifecycleActions(blockedTask, "scotty")).toEqual([
      LifecycleAction.CLOSE,
      LifecycleAction.CANCEL,
    ]);
  });
});

describe("mutation results", () => {
  it("reports lifecycle cascades returned by the server", () => {
    expect(
      describeLifecycleMutation(LifecycleAction.CLOSE, "cm-parent", 2),
    ).toBe("Closed cm-parent and 2 descendants.");
  });

  it("reports authoritative dependency counts", () => {
    expect(
      describeDependencyMutation(create(IssueDetailSchema, {
        prerequisites: [
          create(RelatedIssueSchema, { id: "cm-one" }),
          create(RelatedIssueSchema, { id: "cm-two" }),
        ],
        dependents: [create(RelatedIssueSchema, { id: "cm-three" })],
      })),
    ).toBe("Dependency updated: 2 prerequisites and 1 dependent.");
  });
});

describe("lifecycleActionLabel", () => {
  it("describes lifecycle changes as human actions", () => {
    expect(lifecycleActionLabel(LifecycleAction.CLOSE)).toBe("Mark done");
    expect(lifecycleActionLabel(LifecycleAction.REOPEN)).toBe("Reopen");
    expect(lifecycleActionLabel(LifecycleAction.CANCEL)).toBe("Cancel");
  });
});

describe("checkpointDecisionPresentation", () => {
  it("projects readiness from browser-visible checkpoint state", () => {
    const ready = create(IssueSummarySchema, {
      lifecycle: IssueLifecycle.OPEN,
      status: IssueStatus.READY,
      type: IssueType.CHECKPOINT,
    });
    const blocked = create(IssueSummarySchema, {
      ...ready,
      blocked: true,
      status: IssueStatus.BLOCKED,
    });
    const claimed = create(IssueSummarySchema, {
      ...ready,
      activeClaim: create(ActiveClaimSchema, { actor: "scotty" }),
      status: IssueStatus.IN_PROGRESS,
    });
    const waiting = create(IssueSummarySchema, {
      ...ready,
      status: IssueStatus.WAITING,
      waiting: create(WaitingStateSchema, {
        reason: "vendor access",
        since: create(TimestampSchema, { seconds: 200n }),
      }),
    });

    expect(checkpointDecisionPresentation(ready)).toEqual({
      state: "ready",
    });
    expect(checkpointDecisionPresentation(blocked)).toEqual({
      state: "waiting",
      message: "Waiting for dependencies to be completed.",
    });
    expect(checkpointDecisionPresentation(claimed)).toEqual({
      state: "waiting",
      message: "Claimed by scotty; checkpoint decisions are unavailable.",
    });
    expect(checkpointDecisionPresentation(waiting)).toEqual({
      state: "waiting",
      message: "This checkpoint is not actionable while it is waiting.",
    });
    expect(
      checkpointDecisionPresentation(
        create(IssueSummarySchema, {
          ...ready,
          lifecycle: IssueLifecycle.CLOSED,
          status: IssueStatus.CLOSED,
        }),
      ),
    ).toEqual({ state: "hidden" });
    expect(
      checkpointDecisionPresentation(
        create(IssueSummarySchema, { ...ready, type: IssueType.TASK }),
      ),
    ).toEqual({ state: "hidden" });
  });
});

describe("issue journal presentation", () => {
  const retainedSnapshot = create(LogEntrySchema, {
    id: "log-retained",
    payload: {
      case: "stateSnapshot",
      value: {
        body: { source: "Current State" },
      },
    },
  });
  const olderSnapshot = create(LogEntrySchema, {
    id: "log-older",
    payload: {
      case: "stateSnapshot",
      value: {
        body: { source: "Earlier State" },
        nextAction: { source: "Resume the investigation." },
      },
    },
  });
  const post = create(LogEntrySchema, {
    id: "log-post",
    payload: {
      case: "post",
      value: {
        actor: "observer",
        body: { source: "Diagnostic" },
      },
    },
  });
  const state = create(StateRecordSchema, {
    body: { source: "Current State" },
    snapshotLogEntryId: "log-retained",
  });

  it("suppresses only the snapshot pinned as current State", () => {
    expect(visibleIssueLogEntries(
      [retainedSnapshot, post, olderSnapshot],
      state,
    )?.map(({ id }) => id)).toEqual(["log-post", "log-older"]);
  });

  it("keeps the full Log when terminal issues have no current State", () => {
    const terminal = create(IssueSummarySchema, {
      lifecycle: IssueLifecycle.CLOSED,
    });

    const currentState = currentIssueState(terminal, state);
    expect(currentState).toBeUndefined();
    expect(visibleIssueLogEntries(
      [retainedSnapshot, post, olderSnapshot],
      currentState,
    )?.map(({ id }) => id)).toEqual([
      "log-retained",
      "log-post",
      "log-older",
    ]);
  });

  it("projects planned next actions only from State snapshots", () => {
    expect(logEntryPresentation(olderSnapshot)).toMatchObject({
      body: { source: "Earlier State" },
      kind: "State snapshot",
      nextAction: { source: "Resume the investigation." },
    });
    expect(logEntryPresentation(post)).toMatchObject({
      body: { source: "Diagnostic" },
      kind: undefined,
      nextAction: undefined,
    });
  });
});

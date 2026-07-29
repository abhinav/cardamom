import type { IssueSummary } from "./gen/cardamom/private/v1/issue_pb.ts";
import {
  IssueLifecycle,
  IssueType,
} from "./gen/cardamom/private/v1/issue_pb.ts";

/** LifecycleAction identifies one browser-visible custody or lifecycle control. */
export const LifecycleAction = {
  CLAIM: "claim",
  RELEASE: "release",
  CLOSE: "close",
  REOPEN: "reopen",
  CANCEL: "cancel",
} as const;

/** LifecycleAction is a browser-visible custody or lifecycle control. */
export type LifecycleAction =
  (typeof LifecycleAction)[keyof typeof LifecycleAction];

/** availableLifecycleActions derives controls from lifecycle and issue eligibility. */
export function availableLifecycleActions(
  issue: IssueSummary,
  actor: string,
): LifecycleAction[] {
  if (
    issue.lifecycle === IssueLifecycle.CLOSED ||
    issue.lifecycle === IssueLifecycle.CANCELLED
  ) {
    return [LifecycleAction.REOPEN];
  }
  if (issue.lifecycle !== IssueLifecycle.OPEN) {
    return [];
  }
  if (issue.type === IssueType.CHECKPOINT) {
    return [LifecycleAction.CANCEL];
  }

  const normalizedActor = actor.trim();
  let custodyAction: LifecycleAction | undefined;
  if (issue.activeClaim === undefined) {
    if (!issue.blocked) {
      custodyAction = LifecycleAction.CLAIM;
    }
  } else if (
    issue.activeClaim.actor === normalizedActor &&
    normalizedActor !== ""
  ) {
    custodyAction = LifecycleAction.RELEASE;
  }

  return [
    ...(custodyAction === undefined ? [] : [custodyAction]),
    LifecycleAction.CLOSE,
    LifecycleAction.CANCEL,
  ];
}

/** lifecycleActionLabel returns the command label for one lifecycle action. */
export function lifecycleActionLabel(action: LifecycleAction): string {
  switch (action) {
    case LifecycleAction.CLAIM:
      return "Claim";
    case LifecycleAction.RELEASE:
      return "Release";
    case LifecycleAction.CLOSE:
      return "Mark done";
    case LifecycleAction.REOPEN:
      return "Reopen";
    case LifecycleAction.CANCEL:
      return "Cancel";
  }
}

/** describeLifecycleMutation reports the primary issue and cascade result. */
export function describeLifecycleMutation(
  action: LifecycleAction,
  issueID: string,
  cascadeCount: number,
): string {
  const completedAction = completedLifecycleActionLabel(action);
  if (cascadeCount === 0) {
    return `${completedAction} ${issueID}.`;
  }
  const noun = cascadeCount === 1 ? "descendant" : "descendants";
  return `${completedAction} ${issueID} and ${cascadeCount} ${noun}.`;
}

function completedLifecycleActionLabel(action: LifecycleAction): string {
  switch (action) {
    case LifecycleAction.CLAIM:
      return "Claimed";
    case LifecycleAction.RELEASE:
      return "Released";
    case LifecycleAction.CLOSE:
      return "Closed";
    case LifecycleAction.REOPEN:
      return "Reopened";
    case LifecycleAction.CANCEL:
      return "Cancelled";
  }
}

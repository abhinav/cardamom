import { X } from "lucide-react";

import { IssueStatus } from "./gen/cardamom/private/v1/issue_pb.ts";

/** IssueState names the stable semantic hook shared by every status view. */
type IssueState =
  | "ready"
  | "blocked"
  | "in-progress"
  | "waiting"
  | "closed"
  | "cancelled"
  | "unknown";

/**
 * issueStatusPresentation supplies the shared visible and visual identity of
 * an issue status.
 */
export function issueStatusPresentation(
  status: IssueStatus,
): { label: string; state: IssueState } {
  switch (status) {
    case IssueStatus.READY:
      return { label: "Ready", state: "ready" };
    case IssueStatus.BLOCKED:
      return { label: "Blocked", state: "blocked" };
    case IssueStatus.IN_PROGRESS:
      return { label: "In progress", state: "in-progress" };
    case IssueStatus.WAITING:
      return { label: "Waiting", state: "waiting" };
    case IssueStatus.CLOSED:
      return { label: "Closed", state: "closed" };
    case IssueStatus.CANCELLED:
      return { label: "Cancelled", state: "cancelled" };
    default:
      return { label: "Unknown", state: "unknown" };
  }
}

/**
 * IssueStatusBadge renders status where the surrounding layout does not name
 * the state.
 */
export function IssueStatusBadge({ status }: { status: IssueStatus }) {
  const presentation = issueStatusPresentation(status);
  return (
    <span className="state-badge" data-issue-state={presentation.state}>
      {presentation.label}
    </span>
  );
}

/**
 * IssueStatusDot reinforces a nearby status label in space-constrained
 * graphs.
 */
export function IssueStatusDot({ status }: { status: IssueStatus }) {
  const presentation = issueStatusPresentation(status);
  return (
    <span
      className="issue-state-dot"
      data-issue-state={presentation.state}
      aria-label={presentation.label}
      title={presentation.label}
    >
      {status === IssueStatus.CANCELLED ? (
        <X aria-hidden="true" strokeWidth={3} />
      ) : null}
    </span>
  );
}

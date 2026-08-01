import { Link } from "react-router";

import { issuePath } from "../board-scope.ts";
import type { RelatedIssue } from "../gen/cardamom/private/v1/issue_pb.ts";
import { IssueStatusDot } from "../issue-status.tsx";

/** IssueReferenceLink presents one compact status-bearing issue destination. */
export function IssueReferenceLink({
  current = false,
  issue,
}: {
  current?: boolean;
  issue: RelatedIssue;
}) {
  return (
    <Link
      className="issue-reference-link"
      to={issuePath(issue.boardId, issue.id)}
      aria-current={current ? "page" : undefined}
    >
      <span className="issue-reference-title">
        <IssueStatusDot status={issue.status} />
        <span>{issue.title}</span>
      </span>
      <small>{issue.id}</small>
    </Link>
  );
}

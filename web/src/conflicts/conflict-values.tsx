import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { issuePath } from "../board-scope.ts";
import { IssueReferencePill } from "../issue-reference-pill.tsx";
import type { RelatedIssue } from "../gen/cardamom/private/v1/issue_pb.ts";
import type {
  ConflictIssue,
  ConflictUnit,
  ConflictValue,
} from "../gen/cardamom/private/v1/conflicts_pb.ts";

/** ConflictIssueLinks retains ordinary hover details and copy controls. */
export function ConflictIssueLinks({
  issues,
}: {
  issues: readonly RelatedIssue[];
}) {
  return (
    <div className="conflict-issues">
      {issues.map((issue) => (
        <IssueReferencePill
          key={`${issue.source?.sourceId ?? ""}:${issue.boardId}:${issue.id}`}
          issueID={issue.id}
          issue={issue}
        >
          <a href={issuePath(issue.boardId, issue.id, issue.source)}>
            {issue.id}
          </a>
        </IssueReferencePill>
      ))}
    </div>
  );
}

/** conflictUnitLabel names the field without exposing storage vocabulary. */
export function conflictUnitLabel(unit?: ConflictUnit): string {
  const labels: Record<string, string> = {
    "issue.title.set": "Title",
    "issue.kind.set": "Type",
    "issue.lifecycle.set": "Lifecycle",
    "issue.priority.set": "Priority",
    "issue.waiting.set": "Waiting",
    "issue.summary.set": "Summary",
    "issue.details.set": "Details",
    "issue.state": "State",
    "issue.result": "Result",
    "issue.checkpoint": "Checkpoint decision",
    "issue.label": "Label",
    "issue.dependency": "Dependency",
    "issue.containment": "Parent",
    "board.external_key": "External key",
    "board.pin": "Pin",
    "board.name.set": "Board name",
    "board.description.set": "Board description",
    "board.archive.set": "Archive state",
    "board.issue_id_prefix.set": "Issue ID prefix override",
    "board.issue_id_strategy.set": "Issue ID strategy override",
    "board.summary_limit.set": "Summary limit override",
    "board.attachment_limit.set": "Attachment limit override",
    "board.pin_limit.set": "Pin limit override",
  };
  const label =
    unit === undefined ? "Conflict" : (labels[unit.kind] ?? unit.kind);
  return unit?.key ? `${label}: ${unit.key}` : label;
}

/** ConflictValueContent presents the complete selected assignment, including absence. */
export function ConflictValueContent({ value }: { value: ConflictValue }) {
  const body = value.value;
  const reference = (uid: string) => (
    <RetainedReference uid={uid} issues={value.issues} />
  );
  switch (body.case) {
    case "text":
      return <pre>{body.value.text ?? "No value"}</pre>;
    case "overrideText":
      return <p>{body.value.text ?? "Inherit setting"}</p>;
    case "overrideLimit":
      return <p>{body.value.value?.toString() ?? "Inherit limit"}</p>;
    case "kind":
      return <p>{body.value}</p>;
    case "priority":
      return <p>Priority {body.value}</p>;
    case "lifecycle":
      return (
        <p>
          {body.value.lifecycle}
          {body.value.closedAt && <> since {formatTime(body.value.closedAt)}</>}
        </p>
      );
    case "waiting":
      return body.value.reason === undefined ? (
        <p>Not waiting</p>
      ) : (
        <>
          <p>Waiting since {formatTime(body.value.since)}</p>
          <pre>{body.value.reason}</pre>
        </>
      );
    case "archive":
      return body.value.actor === undefined ? (
        <p>Active board</p>
      ) : (
        <p>
          Archived by {body.value.actor} at {formatTime(body.value.at)}
        </p>
      );
    case "pin":
      return (
        <p>
          {body.value.present
            ? `Pinned at position ${body.value.position}`
            : "Not pinned"}
        </p>
      );
    case "membership":
      return (
        <div>
          <p>{body.value.present ? "Present" : "Removed"}</p>
          {reference(body.value.issueUid)}
          {body.value.relatedUid && (
            <>
              <span>
                {value.unit?.kind === "issue.containment"
                  ? "Parent"
                  : "Prerequisite"}
              </span>
              {reference(body.value.relatedUid)}
            </>
          )}
        </div>
      );
    case "checkpoint":
      return (
        <>
          <p>
            {body.value.outcome} at {formatTime(body.value.decidedAt)}
          </p>
          <pre>{body.value.reason}</pre>
        </>
      );
    case "state":
      return body.value.body === undefined ? (
        <p>No State</p>
      ) : (
        <>
          {body.value.author !== undefined && (
            <p>Author: {body.value.author}</p>
          )}
          {body.value.updatedAt && (
            <p>Updated: {formatTime(body.value.updatedAt)}</p>
          )}
          {body.value.snapshotLogEntryId && (
            <p>Snapshot: {body.value.snapshotLogEntryId}</p>
          )}
          <pre>{body.value.body}</pre>
          {body.value.nextAction !== undefined && (
            <>
              <h4>Next action</h4>
              <pre>{body.value.nextAction}</pre>
            </>
          )}
        </>
      );
    default:
      return <p>Value unavailable. Refresh the conflict.</p>;
  }
}

function RetainedReference({
  uid,
  issues,
}: {
  uid: string;
  issues: readonly ConflictIssue[];
}) {
  const reference = issues.find((value) => value.uid === uid)?.issue;
  return reference === undefined ? (
    <p>Unavailable issue ({uid})</p>
  ) : (
    <ConflictIssueLinks issues={[reference]} />
  );
}

function formatTime(value?: Timestamp) {
  return value === undefined
    ? "Unknown time"
    : new Date(Number(value.seconds) * 1000).toLocaleString();
}

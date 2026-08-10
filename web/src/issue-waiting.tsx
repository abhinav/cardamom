import {
  IssueStatus,
  type IssueSummary,
} from "./gen/cardamom/private/v1/issue_pb.ts";

type IssueWaitingSummary = Pick<IssueSummary, "status" | "waiting">;

/** IssueWaitingReason presents waiting context at collection or detail density. */
export function IssueWaitingReason({
  issue,
  mode = "compact",
}: {
  issue: IssueWaitingSummary;
  mode?: "compact" | "detail";
}) {
  const waiting = issue.status === IssueStatus.WAITING
    ? issue.waiting
    : undefined;
  if (waiting === undefined) {
    return null;
  }
  const label = `Waiting: ${waiting.reason}`;
  if (mode === "compact") {
    return (
      <span className="issue-waiting-reason-compact" title={label}>
        {label}
      </span>
    );
  }
  const since = waiting.since;
  const date = since === undefined
    ? undefined
    : new Date(Number(since.seconds) * 1000 + since.nanos / 1_000_000);
  return (
    <div className="issue-waiting-reason-detail">
      <strong>{label}</strong>
      {date !== undefined && (
        <span className="issue-waiting-since">
          Waiting since <time dateTime={date.toISOString()}>{waitingDateTime.format(date)}</time>
        </span>
      )}
    </div>
  );
}

const waitingDateTime = new Intl.DateTimeFormat(undefined, {
  dateStyle: "medium",
  timeStyle: "short",
});

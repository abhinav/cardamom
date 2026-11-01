import { boardScopePath } from "../board-scope.ts";
import type { SourceRef } from "../gen/cardamom/private/v1/source_pb.ts";

/** conflictHref preserves board and aggregate source identity during recovery. */
export function conflictHref(
  boardId: string,
  source?: SourceRef,
  issueId?: string,
) {
  const base = `${boardScopePath({ kind: "board", boardId, source })}/conflicts`;
  return issueId === undefined
    ? base
    : `${base}?${new URLSearchParams({ issue: issueId })}`;
}

/** ConflictLink is a compact recovery entry point beside unresolved values. */
export function ConflictLink({
  boardId,
  source,
  issueId,
}: {
  boardId: string;
  source?: SourceRef;
  issueId?: string;
}) {
  return (
    <a
      className="state-badge"
      data-issue-state="unresolved"
      href={conflictHref(boardId, source, issueId)}
    >
      Review conflicts
    </a>
  );
}

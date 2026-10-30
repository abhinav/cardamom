import { Link } from "react-router";

import { issuePath } from "../board-scope.ts";
import type {
  IssueReferenceCandidate,
  IssueReferenceMarker,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import type { SourceRef } from "../gen/cardamom/private/v1/source_pb.ts";
import {
  IssueReferencePill,
  type IssueReferencePreview,
} from "../issue-reference-pill.tsx";

/** ReferenceMarker shows interpretations of an ID without choosing one. */
export function ReferenceMarker({ marker, source, loadIssue }: {
  marker: IssueReferenceMarker;
  source?: SourceRef;
  loadIssue: (candidate: IssueReferenceCandidate) => Promise<IssueReferencePreview>;
}) {
  return (
    <section className="issue-reference-marker" aria-labelledby="reference-marker-title">
      <IssueReferencePill issueID={marker.id} issue={{ marker }}>
        <span>{marker.id}</span>
      </IssueReferencePill>
      <h1 id="reference-marker-title">Ambiguous issue reference</h1>
      <p>This ID was used by more than one issue. It may refer to any of these:</p>
      <ul>
        {marker.candidates.map((candidate) => (
          <li key={`${candidate.boardId}:${candidate.id}`}>
            <IssueReferencePill
              issueID={candidate.id}
              loadIssue={candidate.available ? () => loadIssue(candidate) : undefined}
            >
              {candidate.available ? (
                <Link to={issuePath(
                  candidate.boardId, candidate.id, candidate.source ?? source,
                )}>
                  {candidate.id}
                </Link>
              ) : <span>{candidate.id}</span>}
            </IssueReferencePill>
            <span>
              {candidate.available
                ? candidate.title ?? "Title unresolved"
                : "Issue unavailable locally"}
            </span>
            <small>Board: {candidate.boardId}</small>
          </li>
        ))}
      </ul>
    </section>
  );
}

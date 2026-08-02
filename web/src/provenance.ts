import type { BoardSummary, Project } from "./gen/cardamom/private/v1/project_pb.ts";
import type { IssueSummary } from "./gen/cardamom/private/v1/issue_pb.ts";
import type { SourceRef } from "./gen/cardamom/private/v1/source_pb.ts";
import type { BoardScopeSelection } from "./board-scope.ts";

/** ProvenanceCatalog resolves source-qualified browser records for display. */
export interface ProvenanceCatalog {
  boards: readonly BoardSummary[];
  projects: readonly Project[];
}

/** Provenance identifies the visible ownership path for one issue. */
export interface Provenance {
  source?: string;
  project?: string;
  board?: string;
}

/** issueProvenance resolves source, project, and board labels for an issue. */
export function issueProvenance(
  issue: IssueSummary,
  catalog: ProvenanceCatalog,
): Provenance {
  const source = issue.source;
  const board = findBoard(catalog.boards, issue.boardId, source);
  const project = board === undefined
    ? undefined
    : findProject(catalog.projects, board.projectId, source);
  return {
    source: sourceLabel(source),
    project: project?.name ?? board?.projectId,
    board: board?.name ?? issue.boardId,
  };
}

/** visibleIssueProvenance applies the design's scope-sensitive path. */
export function visibleIssueProvenance(
  issue: IssueSummary,
  catalog: ProvenanceCatalog,
  scope: BoardScopeSelection,
): string | undefined {
  const provenance = issueProvenance(issue, catalog);
  if (scope.kind !== "all") {
    return undefined;
  }
  const parts = [
    scope.sourceId === undefined ? provenance.source : undefined,
    scope.projectId === undefined ? provenance.project : undefined,
    provenance.board,
  ].filter((part): part is string => part !== undefined && part !== "");
  return parts.length === 0 ? undefined : parts.join(" / ");
}

/** sourceLabel returns the configured source alias used by the dashboard. */
export function sourceLabel(source: SourceRef | undefined): string | undefined {
  const value = source?.sourceId.trim();
  return value === undefined || value === "" ? undefined : value;
}

function findBoard(
  boards: readonly BoardSummary[],
  boardId: string,
  source: SourceRef | undefined,
): BoardSummary | undefined {
  return boards.find((board) =>
    board.id === boardId &&
    (source === undefined || board.source?.sourceId === source.sourceId),
  );
}

function findProject(
  projects: readonly Project[],
  projectId: string,
  source: SourceRef | undefined,
): Project | undefined {
  return projects.find((project) =>
    project.id === projectId &&
    (source === undefined || project.source?.sourceId === source.sourceId),
  );
}

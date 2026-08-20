import { useTransport } from "@connectrpc/connect-query";
import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { matchPath, useLocation } from "react-router";

import type { BoardScopeSelection } from "./board-scope.ts";
import type { BoardSummary } from "./gen/cardamom/private/v1/project_pb.ts";
import type { SourceRef } from "./gen/cardamom/private/v1/source_pb.ts";
import { IssueService } from "./gen/cardamom/private/v1/issue_pb.ts";
import { boardForIdentity } from "./provenance.ts";
import { unaryRouteQueryOptions } from "./query-runtime.ts";

interface IssueTitle {
  id: string;
  boardId: string;
  title: string;
  source?: SourceRef;
}

interface DocumentTitleProps {
  boardName: string;
  boards: readonly BoardSummary[];
  selection: BoardScopeSelection;
}

/** DocumentTitle keeps the browser title aligned with route query data. */
export function DocumentTitle({ boardName, boards, selection }: DocumentTitleProps) {
  const transport = useTransport();
  const pathname = useLocation().pathname;
  const issueId = issueRouteId(pathname);
  const issueRequest = useQuery({
    ...unaryRouteQueryOptions(
      IssueService.method.getIssue,
      {
        issueId: issueId ?? "",
        ...(selection.kind !== "board"
          ? {}
          : {
              boardId: selection.boardId,
              ...(selection.source === undefined
                ? {}
                : { source: selection.source }),
            }),
      },
      transport,
    ),
    enabled: issueId !== undefined,
    select: (response) => response.issue?.issue,
  });
  const issue =
    issueRequest.data?.id === issueId ? issueRequest.data : undefined;
  const title = routeDocumentTitle(pathname, boardName, boards, issue);

  useEffect(() => {
    document.title = title;
  }, [title]);

  return null;
}

/** routeDocumentTitle derives a title without retaining prior route metadata. */
export function routeDocumentTitle(
  pathname: string,
  boardName: string,
  boards: readonly Pick<BoardSummary, "id" | "name" | "source">[],
  issue?: IssueTitle,
): string {
  if (
    matchPath(
      { path: "/board/:boardId/settings", end: true },
      pathname,
    ) !== null
  ) {
    return `Settings - ${boardName} - Cardamom`;
  }

  const issueId = issueRouteId(pathname);
  if (issueId === undefined) {
    return `${boardName} - Cardamom`;
  }

  if (issue?.id !== issueId) {
    return `${issueId} - ${boardName} - Cardamom`;
  }
  const issueBoardName =
    boardForIdentity(boards, issue.boardId, issue.source)?.name ?? issue.boardId;
  return `${issue.id}: ${issue.title} - ${issueBoardName} - Cardamom`;
}

function issueRouteId(pathname: string): string | undefined {
  return matchPath(
    { path: "/source/:sourceId/board/:boardId/issue/:issueId", end: true },
    pathname,
  )?.params.issueId ?? matchPath(
    { path: "/board/:boardId/issue/:issueId", end: true },
    pathname,
  )?.params.issueId;
}

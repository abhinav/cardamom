import { useTransport } from "@connectrpc/connect-query";
import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { matchPath, useLocation } from "react-router";

import type { BoardSummary } from "./gen/cardamom/private/v1/project_pb.ts";
import { IssueService } from "./gen/cardamom/private/v1/issue_pb.ts";
import { unaryRouteQueryOptions } from "./query-runtime.ts";

interface IssueTitle {
  id: string;
  boardId: string;
  title: string;
}

interface DocumentTitleProps {
  boardName: string;
  boards: readonly BoardSummary[];
}

/** DocumentTitle keeps the browser title aligned with route query data. */
export function DocumentTitle({ boardName, boards }: DocumentTitleProps) {
  const transport = useTransport();
  const pathname = useLocation().pathname;
  const issueId = issueRouteId(pathname);
  const issueRequest = useQuery({
    ...unaryRouteQueryOptions(
      IssueService.method.getIssue,
      { issueId: issueId ?? "" },
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
  boards: readonly Pick<BoardSummary, "id" | "name">[],
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
    boards.find((board) => board.id === issue.boardId)?.name ?? issue.boardId;
  return `${issue.id}: ${issue.title} - ${issueBoardName} - Cardamom`;
}

function issueRouteId(pathname: string): string | undefined {
  return matchPath(
    { path: "/issues/:issueId", end: true },
    pathname,
  )?.params.issueId;
}

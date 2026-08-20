import { create } from "@bufbuild/protobuf";

import {
  AllBoardsSchema,
  AllSourcesSchema,
  BoardScopeSchema,
  type BoardScope,
} from "./gen/cardamom/private/v1/scope_pb.ts";
import type { BoardSummary, Project } from "./gen/cardamom/private/v1/project_pb.ts";
import {
  SourceRefSchema,
  type SourceCatalogEntry,
  type SourceRef,
} from "./gen/cardamom/private/v1/source_pb.ts";

/** ResolvedBoardScope identifies a board or aggregate scope in a route. */
export type ResolvedBoardScope =
  | { kind: "all"; sourceId?: string; projectId?: string }
  | { kind: "board"; boardId: string; source?: SourceRef };

/** BoardScopeSelection includes routes that do not identify board scope. */
export type BoardScopeSelection =
  | ResolvedBoardScope
  | { kind: "ambiguous"; boardId: string }
  | { kind: "unresolved" };

/** BoardScopeCatalog contains bootstrap metadata needed to resolve routes. */
export interface BoardScopeCatalog {
  /** aggregate reports whether the catalog comes from an aggregate server. */
  aggregate?: boolean;
  boards: readonly BoardSummary[];
  projects: readonly Project[];
  sources: readonly SourceCatalogEntry[];
}

/** BoardPage is a collection or settings page within one route scope. */
export type BoardPage =
  | "board"
  | "list"
  | "approvals"
  | "routines"
  | "settings";

/** routeBoardScope derives board identity from the current URL. */
export function routeBoardScope(
  pathname: string,
  search = new URLSearchParams(),
): BoardScopeSelection {
  const segments = pathname.split("/");
  if (segments[1] === "all") {
    return {
      kind: "all",
      ...(nonblankSearchValue(search, "source") === undefined
        ? {}
        : { sourceId: nonblankSearchValue(search, "source") }),
      ...(nonblankSearchValue(search, "project") === undefined
        ? {}
        : { projectId: nonblankSearchValue(search, "project") }),
    };
  }
  try {
    if (
      segments[1] === "source" &&
      segments[2] !== undefined &&
      segments[2] !== "" &&
      segments[3] === "board" &&
      segments[4] !== undefined &&
      segments[4] !== ""
    ) {
      return {
        kind: "board",
        boardId: decodeURIComponent(segments[4]),
        source: create(SourceRefSchema, {
          sourceId: decodeURIComponent(segments[2]),
        }),
      };
    }
    if (
      segments[1] === "board" &&
      segments[2] !== undefined &&
      segments[2] !== ""
    ) {
      return { kind: "board", boardId: decodeURIComponent(segments[2]) };
    }
    return { kind: "unresolved" };
  } catch {
    return { kind: "unresolved" };
  }
}

/** routeBoardPage returns the collection page retained across scope changes. */
export function routeBoardPage(pathname: string): BoardPage {
  const segments = pathname.split("/");
  const scope = routeBoardScope(pathname);
  let page = segments[2];
  if (scope.kind === "board") {
    page = segments[1] === "source" ? segments[5] : segments[3];
  }
  if (
    page === "list" ||
    page === "approvals" ||
    page === "routines" ||
    (page === "settings" && scope.kind === "board")
  ) {
    return page;
  }
  return "board";
}

/** boardScopePath builds one canonical collection or settings path. */
export function boardScopePath(
  selection: ResolvedBoardScope,
  page: BoardPage = "board",
): string {
  let base = "/all";
  if (selection.kind === "board") {
    const sourceID = sourceRouteID(selection.source);
    base = sourceID === undefined
      ? `/board/${encodeURIComponent(selection.boardId)}`
      : `/source/${encodeURIComponent(sourceID)}/board/${encodeURIComponent(selection.boardId)}`;
  }
  if (page === "board" || (page === "settings" && selection.kind === "all")) {
    return base;
  }
  return `${base}/${page}`;
}

/** boardScopeSearch serializes aggregate source and project scope state. */
export function boardScopeSearch(selection: ResolvedBoardScope): string {
  if (selection.kind !== "all") {
    return "";
  }
  const search = new URLSearchParams();
  if (selection.sourceId !== undefined) {
    search.set("source", selection.sourceId);
  }
  if (selection.projectId !== undefined) {
    search.set("project", selection.projectId);
  }
  const encoded = search.toString();
  return encoded === "" ? "" : `?${encoded}`;
}

/** boardScopeHref builds a canonical path with aggregate query state. */
export function boardScopeHref(
  selection: ResolvedBoardScope,
  page: BoardPage = "board",
): string {
  return boardScopePath(selection, page) + boardScopeSearch(selection);
}

/** resolveBoardScopeSelection enriches a route board with its catalog source. */
export function resolveBoardScopeSelection(
  selection: BoardScopeSelection,
  catalog: BoardScopeCatalog,
): BoardScopeSelection {
  if (selection.kind !== "board") {
    return selection;
  }
  const routeSourceID = sourceRouteID(selection.source);
  const matches = catalog.boards.filter((board) =>
    board.id === selection.boardId &&
    (routeSourceID === undefined || board.source?.sourceId === routeSourceID)
  );
  if (matches.length > 1 && catalog.aggregate) {
    return { kind: "ambiguous", boardId: selection.boardId };
  }
  return {
    ...selection,
    ...(matches[0]?.source === undefined ? {} : { source: matches[0].source }),
  };
}

/** issuePath builds the canonical route for one board-owned issue. */
export function issuePath(
  boardId: string,
  issueId: string,
  source?: SourceRef,
): string {
  return `${boardScopePath({ kind: "board", boardId, source })}/issue/${encodeURIComponent(issueId)}`;
}

/** attachmentPath builds the canonical raw route for one board-owned attachment. */
export function attachmentPath(
  boardId: string,
  attachmentId: string,
  source?: SourceRef,
): string {
  return `${boardScopePath({ kind: "board", boardId, source })}/attachment/${encodeURIComponent(attachmentId)}`;
}

export function toBoardScopeMessage(
  selection: BoardScopeSelection,
  catalog?: BoardScopeCatalog,
): BoardScope | undefined {
  if (selection.kind === "unresolved" || selection.kind === "ambiguous") {
    return undefined;
  }
  if (selection.kind === "all") {
    if (catalog?.aggregate) {
      if (selection.projectId !== undefined) {
        const project = catalog.projects.find(
          (candidate) =>
            candidate.id === selection.projectId &&
            (selection.sourceId === undefined ||
              candidate.source?.sourceId === selection.sourceId),
        );
        if (project?.source !== undefined) {
          return create(BoardScopeSchema, {
            source: project.source,
            selection: { case: "projectId", value: project.id },
          });
        }
      }
      if (selection.sourceId !== undefined) {
        const source = catalog.sources.find(
          (candidate) => candidate.source?.sourceId === selection.sourceId,
        )?.source;
        if (source !== undefined) {
          return create(BoardScopeSchema, {
            source,
            selection: { case: "allBoards", value: create(AllBoardsSchema) },
          });
        }
      }
      return create(BoardScopeSchema, {
        selection: {
          case: "allSources",
          value: create(AllSourcesSchema),
        },
      });
    }
    return create(BoardScopeSchema, {
      selection: {
        case: "allBoards",
        value: create(AllBoardsSchema),
      },
    });
  }
  const source = selection.source ?? catalog?.boards.find((candidate) =>
    candidate.id === selection.boardId
  )?.source;
  return create(BoardScopeSchema, {
    ...(source === undefined ? {} : { source }),
    selection: { case: "boardId", value: selection.boardId },
  });
}

export function scopeKey(selection: BoardScopeSelection): string {
  if (selection.kind === "unresolved") {
    return "unresolved";
  }
  if (selection.kind === "ambiguous") {
    return `ambiguous:${selection.boardId}`;
  }
  if (selection.kind === "all") {
    if (selection.sourceId === undefined && selection.projectId === undefined) {
      return "all";
    }
    return `all:${selection.sourceId ?? ""}:${selection.projectId ?? ""}`;
  }
  return selection.source?.sourceId === undefined
    ? `board:${selection.boardId}`
    : `board:${selection.source.sourceId}:${selection.boardId}`;
}

function nonblankSearchValue(
  search: URLSearchParams,
  key: string,
): string | undefined {
  const value = search.get(key)?.trim();
  return value === undefined || value === "" ? undefined : value;
}

function sourceRouteID(source: SourceRef | undefined): string | undefined {
  const sourceID = source?.sourceId.trim();
  return sourceID === undefined || sourceID === "" ? undefined : sourceID;
}

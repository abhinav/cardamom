import { create } from "@bufbuild/protobuf";

import {
  AllBoardsSchema,
  AllSourcesSchema,
  BoardScopeSchema,
  type BoardScope,
} from "./gen/cardamom/private/v1/scope_pb.ts";
import type { BoardSummary, Project } from "./gen/cardamom/private/v1/project_pb.ts";
import type {
  BoardRef,
  SourceCatalogEntry,
} from "./gen/cardamom/private/v1/source_pb.ts";

/** ResolvedBoardScope identifies a board or aggregate scope in a route. */
export type ResolvedBoardScope =
  | { kind: "all"; sourceId?: string; projectId?: string }
  | { kind: "board"; boardId: string; boardRef?: BoardRef };

/** BoardScopeSelection includes routes that do not identify board scope. */
export type BoardScopeSelection =
  | ResolvedBoardScope
  | { kind: "ambiguous"; boardId: string }
  | { kind: "unresolved" };

/** BoardScopeCatalog contains bootstrap metadata needed to resolve routes. */
export interface BoardScopeCatalog {
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
  if (segments[1] !== "board" || segments[2] === undefined || segments[2] === "") {
    return { kind: "unresolved" };
  }
  try {
    return { kind: "board", boardId: decodeURIComponent(segments[2]) };
  } catch {
    return { kind: "unresolved" };
  }
}

/** routeBoardPage returns the collection page retained across scope changes. */
export function routeBoardPage(pathname: string): BoardPage {
  const segments = pathname.split("/");
  const scope = routeBoardScope(pathname);
  const page = scope.kind === "board" ? segments[3] : segments[2];
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
  const base = selection.kind === "all"
    ? "/all"
    : `/board/${encodeURIComponent(selection.boardId)}`;
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

/** resolveBoardScopeSelection enriches a route board with its catalog ref. */
export function resolveBoardScopeSelection(
  selection: BoardScopeSelection,
  catalog: BoardScopeCatalog,
): BoardScopeSelection {
  if (selection.kind !== "board") {
    return selection;
  }
  const matches = catalog.boards.filter((board) => board.id === selection.boardId);
  if (matches.length > 1 && catalog.sources.length > 0) {
    return { kind: "ambiguous", boardId: selection.boardId };
  }
  return {
    ...selection,
    ...(matches[0]?.ref === undefined ? {} : { boardRef: matches[0].ref }),
  };
}

/** issuePath builds the canonical route for one board-owned issue. */
export function issuePath(boardId: string, issueId: string): string {
  return `${boardScopePath({ kind: "board", boardId })}/issue/${encodeURIComponent(issueId)}`;
}

/** attachmentPath builds the canonical raw route for one board-owned attachment. */
export function attachmentPath(boardId: string, attachmentId: string): string {
  return `${boardScopePath({ kind: "board", boardId })}/attachment/${encodeURIComponent(attachmentId)}`;
}

export function toBoardScopeMessage(
  selection: BoardScopeSelection,
  catalog?: BoardScopeCatalog,
): BoardScope | undefined {
  if (selection.kind === "unresolved" || selection.kind === "ambiguous") {
    return undefined;
  }
  if (selection.kind === "all") {
    if (catalog !== undefined && catalog.sources.length > 0) {
      if (selection.projectId !== undefined) {
        const project = catalog.projects.find(
          (candidate) =>
            candidate.id === selection.projectId &&
            (selection.sourceId === undefined ||
              candidate.ref?.source?.sourceId === selection.sourceId),
        );
        if (project?.ref !== undefined) {
          return create(BoardScopeSchema, {
            selection: { case: "project", value: project.ref },
          });
        }
      }
      if (selection.sourceId !== undefined) {
        const source = catalog.sources.find(
          (candidate) => candidate.source?.sourceId === selection.sourceId,
        )?.source;
        if (source !== undefined) {
          return create(BoardScopeSchema, {
            selection: { case: "source", value: source },
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
  const board = selection.boardRef ?? catalog?.boards.find(
    (candidate) => candidate.id === selection.boardId,
  )?.ref;
  if (board !== undefined) {
    return create(BoardScopeSchema, {
      selection: { case: "board", value: board },
    });
  }
  return create(BoardScopeSchema, {
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
  return selection.boardRef?.source?.sourceId === undefined
    ? `board:${selection.boardId}`
    : `board:${selection.boardRef.source.sourceId}:${selection.boardId}`;
}

function nonblankSearchValue(
  search: URLSearchParams,
  key: string,
): string | undefined {
  const value = search.get(key)?.trim();
  return value === undefined || value === "" ? undefined : value;
}

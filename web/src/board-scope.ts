import { create } from "@bufbuild/protobuf";

import {
  AllBoardsSchema,
  BoardScopeSchema,
  type BoardScope,
} from "./gen/cardamom/private/v1/scope_pb.ts";

/** ResolvedBoardScope identifies a board or aggregate scope in a route. */
export type ResolvedBoardScope =
  | { kind: "all" }
  | { kind: "board"; boardId: string };

/** BoardScopeSelection includes routes that do not identify board scope. */
export type BoardScopeSelection =
  | ResolvedBoardScope
  | { kind: "unresolved" };

/** BoardPage is a collection or settings page within one route scope. */
export type BoardPage =
  | "board"
  | "list"
  | "approvals"
  | "routines"
  | "settings";

/** routeBoardScope derives board identity from the current URL. */
export function routeBoardScope(pathname: string): BoardScopeSelection {
  const segments = pathname.split("/");
  if (segments[1] === "all") {
    return { kind: "all" };
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

export function toBoardScopeMessage(
  selection: BoardScopeSelection,
): BoardScope | undefined {
  if (selection.kind === "unresolved") {
    return undefined;
  }
  if (selection.kind === "all") {
    return create(BoardScopeSchema, {
      selection: {
        case: "allBoards",
        value: create(AllBoardsSchema),
      },
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
  return selection.kind === "all" ? "all" : `board:${selection.boardId}`;
}

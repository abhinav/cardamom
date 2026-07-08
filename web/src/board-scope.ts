import { create } from "@bufbuild/protobuf";

import {
  AllBoardsSchema,
  BoardScopeSchema,
  type BoardScope,
} from "./gen/cardamom/private/v1/scope_pb.ts";

interface AvailableBoard {
  id: string;
}

/** BoardScopePreference is an explicit browser selection persisted between sessions. */
export type BoardScopePreference =
  | { kind: "all" }
  | { kind: "board"; boardId: string };

/** BoardScopeSelection adds the unresolved state used for an ambiguous catalog. */
export type BoardScopeSelection =
  | BoardScopePreference
  | { kind: "unresolved" };

export function resolveBoardScope(
  persisted: BoardScopePreference | undefined,
  boards: readonly AvailableBoard[],
  serverDefaultBoardId: string | undefined,
): BoardScopeSelection {
  if (persisted !== undefined) {
    return persisted;
  }
  if (
    serverDefaultBoardId !== undefined &&
    boards.some((board) => board.id === serverDefaultBoardId)
  ) {
    return { kind: "board", boardId: serverDefaultBoardId };
  }
  if (boards.length === 1 && boards[0] !== undefined) {
    return { kind: "board", boardId: boards[0].id };
  }
  return boards.length === 0 ? { kind: "all" } : { kind: "unresolved" };
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

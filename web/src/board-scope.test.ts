import { describe, expect, it } from "vitest";

import {
  resolveBoardScope,
  scopeKey,
  toBoardScopeMessage,
} from "./board-scope.ts";

const boards = [
  { id: "board-1", projectId: "project-1", name: "Primary" },
  { id: "board-2", projectId: "project-1", name: "Secondary" },
];

describe("resolveBoardScope", () => {
  it("keeps a persisted board that is still available", () => {
    expect(
      resolveBoardScope(
        { kind: "board", boardId: "board-2" },
        boards,
        "board-1",
      ),
    ).toEqual({ kind: "board", boardId: "board-2" });
  });

  it("keeps a stale explicit board so the shell can report it unavailable", () => {
    expect(
      resolveBoardScope(
        { kind: "board", boardId: "missing" },
        boards,
        "board-2",
      ),
    ).toEqual({ kind: "board", boardId: "missing" });
  });

  it("uses a valid server default or the only available board", () => {
    expect(resolveBoardScope(undefined, boards, "board-2")).toEqual({
      kind: "board",
      boardId: "board-2",
    });
    expect(resolveBoardScope(undefined, [boards[0]!], "missing")).toEqual({
      kind: "board",
      boardId: "board-1",
    });
  });

  it("leaves multiple boards unresolved and permits an empty aggregate", () => {
    expect(resolveBoardScope(undefined, boards, "missing")).toEqual({
      kind: "unresolved",
    });
    expect(resolveBoardScope(undefined, [], undefined)).toEqual({ kind: "all" });
  });

  it("preserves the explicit all-boards scope", () => {
    expect(resolveBoardScope({ kind: "all" }, boards, "board-1")).toEqual({
      kind: "all",
    });
  });
});

describe("board scope boundary", () => {
  it("builds generated protobuf selections and stable request keys", () => {
    const all = toBoardScopeMessage({ kind: "all" });
    const board = toBoardScopeMessage({ kind: "board", boardId: "board-2" });

    expect(all?.selection.case).toBe("allBoards");
    expect(board?.selection).toEqual({ case: "boardId", value: "board-2" });
    expect(scopeKey({ kind: "all" })).toBe("all");
    expect(scopeKey({ kind: "board", boardId: "board-2" })).toBe("board:board-2");
    expect(toBoardScopeMessage({ kind: "unresolved" })).toBeUndefined();
    expect(scopeKey({ kind: "unresolved" })).toBe("unresolved");
  });
});

import { describe, expect, it } from "vitest";

import {
  boardScopePath,
  routeBoardPage,
  routeBoardScope,
  scopeKey,
  toBoardScopeMessage,
} from "./board-scope.ts";

describe("board scope routes", () => {
  it("derives board scope only from canonical route prefixes", () => {
    expect(routeBoardScope("/")).toEqual({ kind: "unresolved" });
    expect(routeBoardScope("/board/board-1")).toEqual({
      kind: "board",
      boardId: "board-1",
    });
    expect(routeBoardScope("/board/board%20one/list")).toEqual({
      kind: "board",
      boardId: "board one",
    });
    expect(routeBoardScope("/all/approvals")).toEqual({ kind: "all" });
    expect(routeBoardScope("/alligator")).toEqual({ kind: "unresolved" });
    expect(routeBoardScope("/board/%")).toEqual({ kind: "unresolved" });
    expect(routeBoardScope("/issues/cm-task")).toEqual({
      kind: "unresolved",
    });
  });

  it("builds canonical board and all-board collection routes", () => {
    expect(boardScopePath({ kind: "board", boardId: "board one" }, "board"))
      .toBe("/board/board%20one");
    expect(boardScopePath({ kind: "board", boardId: "board-1" }, "list"))
      .toBe("/board/board-1/list");
    expect(boardScopePath({ kind: "board", boardId: "board-1" }, "settings"))
      .toBe("/board/board-1/settings");
    expect(boardScopePath({ kind: "all" }, "board")).toBe("/all");
    expect(boardScopePath({ kind: "all" }, "routines")).toBe(
      "/all/routines",
    );
    expect(boardScopePath({ kind: "all" }, "settings")).toBe("/all");
  });

  it("identifies the collection page preserved by board selection", () => {
    expect(routeBoardPage("/")).toBe("board");
    expect(routeBoardPage("/board/board-1/list")).toBe("list");
    expect(routeBoardPage("/all/approvals")).toBe("approvals");
    expect(routeBoardPage("/board/board-1/settings")).toBe("settings");
    expect(routeBoardPage("/board/board-1/issue/cm-task")).toBe("board");
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

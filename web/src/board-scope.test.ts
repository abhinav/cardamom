import { describe, expect, it } from "vitest";
import { create } from "@bufbuild/protobuf";

import {
  attachmentPath,
  boardScopeHref,
  boardScopePath,
  boardScopeSearch,
  issuePath,
  routeBoardPage,
  routeBoardScope,
  resolveBoardScopeSelection,
  scopeKey,
  toBoardScopeMessage,
} from "./board-scope.ts";
import {
  SourceCatalogEntrySchema,
  SourceRefSchema,
} from "./gen/cardamom/private/v1/source_pb.ts";
import {
  BoardSummarySchema,
  ProjectSchema,
} from "./gen/cardamom/private/v1/project_pb.ts";

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
    expect(routeBoardScope("/source/backup/board/board%20one/list")).toEqual({
      kind: "board",
      boardId: "board one",
      source: create(SourceRefSchema, { sourceId: "backup" }),
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
    expect(boardScopePath({
      kind: "board",
      boardId: "board-1",
      source: create(SourceRefSchema, { sourceId: "backup" }),
    }, "list")).toBe("/source/backup/board/board-1/list");
    expect(boardScopePath({ kind: "all" }, "board")).toBe("/all");
    expect(boardScopePath({ kind: "all" }, "routines")).toBe(
      "/all/routines",
    );
    expect(boardScopePath({ kind: "all" }, "settings")).toBe("/all");
  });

  it("builds canonical board-owned entity routes", () => {
    expect(issuePath("board one", "cm/task")).toBe(
      "/board/board%20one/issue/cm%2Ftask",
    );
    expect(issuePath(
      "board one",
      "cm/task",
      create(SourceRefSchema, { sourceId: "backup" }),
    )).toBe("/source/backup/board/board%20one/issue/cm%2Ftask");
    expect(attachmentPath("board one", "att/value")).toBe(
      "/board/board%20one/attachment/att%2Fvalue",
    );
    expect(attachmentPath(
      "board one",
      "att/value",
      create(SourceRefSchema, { sourceId: "backup" }),
    )).toBe("/source/backup/board/board%20one/attachment/att%2Fvalue");
  });

  it("identifies the collection page preserved by board selection", () => {
    expect(routeBoardPage("/")).toBe("board");
    expect(routeBoardPage("/board/board-1/list")).toBe("list");
    expect(routeBoardPage("/all/approvals")).toBe("approvals");
    expect(routeBoardPage("/board/board-1/settings")).toBe("settings");
    expect(routeBoardPage("/board/board-1/issue/cm-task")).toBe("board");
    expect(routeBoardPage("/source/backup/board/board-1/list")).toBe("list");
    expect(routeBoardPage("/source/backup/board/board-1/issue/cm-task"))
      .toBe("board");
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

  it("keeps aggregate source and project filters in canonical query state", () => {
    const source = create(SourceRefSchema, {
      sourceId: "builder",
      storeLineageId: "lineage-builder",
    });
    const catalog = {
      aggregate: true,
      sources: [create(SourceCatalogEntrySchema, { source })],
      projects: [
        create(ProjectSchema, {
          id: "project-1",
          name: "Build",
          source,
        }),
      ],
      boards: [
        create(BoardSummarySchema, {
          id: "board-1",
          projectId: "project-1",
          name: "Release",
          source,
        }),
      ],
    };
    const selection = routeBoardScope(
      "/all/list",
      new URLSearchParams("source=builder&project=project-1"),
    );

    expect(selection).toEqual({
      kind: "all",
      sourceId: "builder",
      projectId: "project-1",
    });
    if (selection.kind !== "all") {
      throw new Error("expected the aggregate route to resolve");
    }
    expect(boardScopeSearch(selection)).toBe("?source=builder&project=project-1");
    expect(boardScopeHref(selection, "list")).toBe(
      "/all/list?source=builder&project=project-1",
    );
    expect(toBoardScopeMessage(selection, catalog)?.selection.case).toBe(
      "projectId",
    );

    const boardSelection = resolveBoardScopeSelection(
      routeBoardScope("/board/board-1"),
      catalog,
    );
    if (boardSelection.kind !== "board") {
      throw new Error("expected the unique board route to resolve");
    }
    expect(toBoardScopeMessage(boardSelection, catalog)?.selection.case).toBe(
      "boardId",
    );
  });

  it("resolves equal board IDs by the source encoded in the route", () => {
    const first = create(SourceRefSchema, {
      sourceId: "first",
      storeLineageId: "lineage-first",
    });
    const second = create(SourceRefSchema, {
      sourceId: "second",
      storeLineageId: "lineage-second",
    });
    const catalog = {
      aggregate: true,
      sources: [
        create(SourceCatalogEntrySchema, { source: first }),
        create(SourceCatalogEntrySchema, { source: second }),
      ],
      projects: [],
      boards: [
        create(BoardSummarySchema, {
          id: "board-copied",
          projectId: "project-first",
          name: "Original",
          source: first,
        }),
        create(BoardSummarySchema, {
          id: "board-copied",
          projectId: "project-second",
          name: "Restored",
          source: second,
        }),
      ],
    };

    expect(resolveBoardScopeSelection(
      routeBoardScope("/board/board-copied"),
      catalog,
    )).toEqual({ kind: "ambiguous", boardId: "board-copied" });

    const selection = resolveBoardScopeSelection(
      routeBoardScope("/source/second/board/board-copied"),
      catalog,
    );
    expect(selection).toEqual({
      kind: "board",
      boardId: "board-copied",
      source: second,
    });
    expect(toBoardScopeMessage(selection, catalog)?.source).toEqual(second);
    expect(scopeKey(selection)).toBe("board:second:board-copied");
  });

  it("uses all boards for a local catalog that identifies its source", () => {
    const source = create(SourceRefSchema, {
      sourceId: "local",
      storeLineageId: "lineage-local",
    });
    const catalog = {
      aggregate: false,
      sources: [create(SourceCatalogEntrySchema, { source })],
      projects: [
        create(ProjectSchema, {
          id: "project-1",
          name: "Local",
          source,
        }),
      ],
      boards: [
        create(BoardSummarySchema, {
          id: "board-1",
          projectId: "project-1",
          name: "Primary",
          source,
        }),
      ],
    };

    const scope = toBoardScopeMessage({ kind: "all" }, catalog);

    expect(scope?.selection.case).toBe("allBoards");
  });
});

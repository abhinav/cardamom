import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";

import { routeDocumentTitle } from "./document-title.tsx";
import { SourceRefSchema } from "./gen/cardamom/private/v1/source_pb.ts";

const boards = [
  { id: "board-primary", name: "Primary" },
  { id: "board-secondary", name: "Secondary" },
];

describe("route document title", () => {
  it("identifies board routes with the selected board name", () => {
    expect(routeDocumentTitle("/board/board-primary", "Primary", boards)).toBe(
      "Primary - Cardamom",
    );
    expect(routeDocumentTitle("/all/list", "All boards", boards)).toBe(
      "All boards - Cardamom",
    );
  });

  it("identifies settings before the selected board name", () => {
    expect(
      routeDocumentTitle("/board/board-primary/settings", "Primary", boards),
    ).toBe("Settings - Primary - Cardamom");
  });

  it("adds issue metadata after the matching issue loads", () => {
    expect(
      routeDocumentTitle(
        "/board/board-secondary/issue/cm-title",
        "Secondary",
        boards,
      ),
    ).toBe("cm-title - Secondary - Cardamom");

    expect(
      routeDocumentTitle("/board/board-secondary/issue/cm-title", "Secondary", boards, {
        id: "cm-title",
        boardId: "board-secondary",
        title: "Set route-aware browser titles",
      }),
    ).toBe(
      "cm-title: Set route-aware browser titles - Secondary - Cardamom",
    );
  });

  it("does not retain issue metadata across route transitions", () => {
    const previousIssue = {
      id: "cm-previous",
      boardId: "board-primary",
      title: "Previous issue",
    };

    expect(
      routeDocumentTitle(
        "/board/board-primary/issue/cm-current",
        "Primary",
        boards,
        previousIssue,
      ),
    ).toBe("cm-current - Primary - Cardamom");
    expect(
      routeDocumentTitle(
        "/board/board-primary/settings",
        "Primary",
        boards,
        previousIssue,
      ),
    ).toBe("Settings - Primary - Cardamom");
  });

  it("uses issue source identity when copied boards share an ID", () => {
    const first = create(SourceRefSchema, { sourceId: "first" });
    const second = create(SourceRefSchema, { sourceId: "second" });

    expect(routeDocumentTitle(
      "/source/second/board/board-copied/issue/card-1",
      "Restored",
      [
        { id: "board-copied", name: "Original", source: first },
        { id: "board-copied", name: "Restored", source: second },
      ],
      {
        id: "card-1",
        boardId: "board-copied",
        title: "Continue restored work",
        source: second,
      },
    )).toBe(
      "card-1: Continue restored work - Restored - Cardamom",
    );
  });
});

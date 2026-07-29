import { describe, expect, it } from "vitest";

import { routeDocumentTitle } from "./document-title.tsx";

const boards = [
  { id: "board-primary", name: "Primary" },
  { id: "board-secondary", name: "Secondary" },
];

describe("route document title", () => {
  it("identifies board routes with the selected board name", () => {
    expect(routeDocumentTitle("/", "Primary", boards)).toBe(
      "Primary - Cardamom",
    );
    expect(routeDocumentTitle("/list", "All boards", boards)).toBe(
      "All boards - Cardamom",
    );
  });

  it("identifies settings before the selected board name", () => {
    expect(routeDocumentTitle("/configuration", "Primary", boards)).toBe(
      "Settings - Primary - Cardamom",
    );
  });

  it("adds issue metadata after the matching issue loads", () => {
    expect(
      routeDocumentTitle("/issues/cm-title", "All boards", boards),
    ).toBe("cm-title - All boards - Cardamom");

    expect(
      routeDocumentTitle("/issues/cm-title", "All boards", boards, {
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
        "/issues/cm-current",
        "Primary",
        boards,
        previousIssue,
      ),
    ).toBe("cm-current - Primary - Cardamom");
    expect(
      routeDocumentTitle("/configuration", "Primary", boards, previousIssue),
    ).toBe("Settings - Primary - Cardamom");
  });
});

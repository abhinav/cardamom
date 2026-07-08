import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  BoardSettingsContent,
  boardSettingsLoaded,
  boardSettingsBoardId,
  boardSettingsUpdateInput,
} from "./board-settings.tsx";
import type { Board } from "./gen/cardamom/private/v1/project_pb.ts";

describe("Board settings workflow", () => {
  it("is available only for a concrete board selection", () => {
    expect(boardSettingsBoardId({ kind: "board", boardId: "board-1" })).toBe(
      "board-1",
    );
    expect(boardSettingsBoardId({ kind: "all" })).toBeUndefined();
    expect(boardSettingsBoardId({ kind: "unresolved" })).toBeUndefined();
  });

  it("renders only board settings on the read surface", () => {
    const markup = renderToStaticMarkup(
      BoardSettingsContent({
        board: board("Board One", "Initial **context**"),
        draft: {
          name: "Board One",
          descriptionSource: "Initial **context**",
        },
        mode: "read",
        submission: { kind: "idle" },
        onBeginEdit: vi.fn(),
        onCancelEdit: vi.fn(),
        onChangeDraft: vi.fn(),
        onDismiss: vi.fn(),
        onSubmit: vi.fn(),
      }),
    );

    expect(markup).toContain("<h3>Board One</h3>");
    expect(markup).toContain("<p>context</p>");
    expect(markup).toContain(">Close</button>");
    expect(markup).toContain(">Edit</button>");
    expect(markup).not.toContain("Attachments");
    expect(markup).not.toContain("Add a file");
    expect(markup).not.toContain('type="file"');
    expect(markup).not.toContain(">Upload</button>");
  });

  it("normalizes editable settings for UpdateBoard", () => {
    expect(boardSettingsUpdateInput(
      "board-1",
      "  browser-actor  ",
      "  Renamed board  ",
      "Updated **context**",
    )).toEqual({
      boardId: "board-1",
      name: "Renamed board",
      descriptionSource: "Updated **context**",
      context: { actor: "browser-actor" },
    });
  });

  it("preserves an active draft when board data refreshes", () => {
    const editing = {
      initialized: true,
      draft: {
        name: "Unsaved name",
        descriptionSource: "Unsaved description",
      },
      mode: "edit" as const,
      submission: { kind: "idle" as const },
    };

    expect(
      boardSettingsLoaded(
        editing,
        board("Changed elsewhere", "Changed context"),
      ),
    ).toBe(editing);
  });
});

function board(name: string, source: string): Board {
  return {
    $typeName: "cardamom.private.v1.Board",
    id: "board-1",
    projectId: "project-1",
    name,
    description: {
      $typeName: "cardamom.private.v1.MarkdownContent",
      source,
      renderedHtml: "<p>context</p>",
    },
  };
}

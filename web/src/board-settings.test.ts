import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  BoardLifecycleControls,
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
        onSubmit: vi.fn(),
      }),
    );

    expect(markup).toContain("<h3>Board One</h3>");
    expect(markup).toContain("<p>context</p>");
    expect(markup).toContain('aria-label="Edit board name"');
    expect(markup).toContain('aria-label="Edit board description"');
    expect(markup).not.toContain(">Close</button>");
    expect(markup).not.toContain(">Edit</button>");
    expect(markup).not.toContain("Attachments");
    expect(markup).not.toContain("Add a file");
    expect(markup).not.toContain('type="file"');
    expect(markup).not.toContain(">Upload</button>");
  });

  it("keeps an archived board readable without an edit action", () => {
    const value = board("Retired", "Archived context");
    value.archived = {
      $typeName: "cardamom.private.v1.BoardArchive",
      actor: "captain",
    };
    const markup = renderToStaticMarkup(BoardSettingsContent({
      board: value,
      draft: { name: value.name, descriptionSource: "Archived context" },
      mode: "read",
      submission: { kind: "idle" },
      onBeginEdit: vi.fn(),
      onCancelEdit: vi.fn(),
      onChangeDraft: vi.fn(),
      onSubmit: vi.fn(),
    }));

    expect(markup).toContain("Retired");
    expect(markup).not.toContain('aria-label="Edit board name"');
    expect(markup).not.toContain('aria-label="Edit board description"');
  });

  it("edits the board name without presenting the description field", () => {
    const markup = renderToStaticMarkup(BoardSettingsContent({
      board: board("Board One", "Initial context"),
      draft: { name: "Board One", descriptionSource: "Initial context" },
      mode: "name",
      submission: { kind: "idle" },
      onBeginEdit: vi.fn(),
      onCancelEdit: vi.fn(),
      onChangeDraft: vi.fn(),
      onSubmit: vi.fn(),
    }));

    expect(markup).toContain(">Name</span>");
    expect(markup).toContain(">Save name</button>");
    expect(markup).not.toContain("Description (Markdown)");
    expect(markup).not.toContain("<textarea");
  });

  it("edits the board description without presenting the name field", () => {
    const markup = renderToStaticMarkup(BoardSettingsContent({
      board: board("Board One", "Initial context"),
      draft: { name: "Board One", descriptionSource: "Initial context" },
      mode: "description",
      submission: { kind: "idle" },
      onBeginEdit: vi.fn(),
      onCancelEdit: vi.fn(),
      onChangeDraft: vi.fn(),
      onSubmit: vi.fn(),
    }));

    expect(markup).toContain("Description (Markdown)");
    expect(markup).toContain(">Save description</button>");
    expect(markup).not.toContain(">Name</span>");
    expect(markup).not.toContain('type="text"');
  });

  it("keeps archival controls collapsed until requested", () => {
    const collapsed = renderToStaticMarkup(BoardLifecycleControls({
      archived: false,
      expanded: false,
      reason: "",
      onArchive: vi.fn(),
      onBeginArchive: vi.fn(),
      onCancelArchive: vi.fn(),
      onReasonChange: vi.fn(),
      onUnarchive: vi.fn(),
    }));
    const expanded = renderToStaticMarkup(BoardLifecycleControls({
      archived: false,
      expanded: true,
      reason: "",
      onArchive: vi.fn(),
      onBeginArchive: vi.fn(),
      onCancelArchive: vi.fn(),
      onReasonChange: vi.fn(),
      onUnarchive: vi.fn(),
    }));

    expect(collapsed).toContain('aria-label="Archive board"');
    expect(collapsed).not.toContain("Archive reason (optional)");
    expect(expanded).toContain("Archive reason (optional)");
    expect(expanded).toContain(">Archive</button>");
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
      mode: "name" as const,
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

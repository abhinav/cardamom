import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { SettingsControl } from "./app.tsx";
import { defaultBoardView } from "./issue-collection.ts";
import { IssueControls } from "./issue-views.tsx";
import { defaultPreferences } from "./preferences.ts";

describe("responsive toolbar", () => {
  it("uses accessible compact collection controls", () => {
    const markup = renderToStaticMarkup(createElement(IssueControls, {
      mode: "board",
      view: {
        ...defaultBoardView,
        filters: { ...defaultBoardView.filters, label: "area:web" },
      },
      grouping: "status",
      updateFilters: vi.fn(),
      updateGrouping: vi.fn(),
      updateView: vi.fn(),
      createIssue: vi.fn(),
    }));

    expect(markup.match(/name="issue-collection-options"/g)).toHaveLength(2);
    expect(markup).toContain('aria-label="Search issues"');
    expect(markup).toContain('title="Search issues"');
    expect(markup).toContain("lucide-search");
    expect(markup).toContain('aria-label="Filters, 1 active"');
    expect(markup).toContain('title="Filters"');
    expect(markup).toContain("lucide-list-filter");
    expect(markup).toContain('aria-label="View options"');
    expect(markup).toContain('title="View options"');
    expect(markup).toContain("lucide-sliders-horizontal");
    expect(markup).toContain('aria-label="Create issue"');
    expect(markup).toContain('title="Create issue"');
    expect(markup).toContain("lucide-plus");
  });

  it("presents global settings as an accessible header icon", () => {
    const markup = renderToStaticMarkup(createElement(SettingsControl, {
      preferences: {
        ...defaultPreferences,
        boardView: {
          ...defaultPreferences.boardView,
          showEmptyColumns: true,
        },
      },
      openConfiguration: vi.fn(),
      selectedBoard: undefined,
      updatePreferences: vi.fn(),
      version: "v1.2.3",
    }));

    expect(markup).toContain('aria-label="Settings"');
    expect(markup).toContain('title="Settings"');
    expect(markup).toContain("lucide-settings");
    expect(markup).not.toContain("session-board-control");
    expect(markup).not.toContain(">All boards<");
    expect(markup).not.toContain(">Board settings<");
    expect(markup).toContain('type="checkbox"');
    expect(markup).toContain('checked=""');
    expect(markup).toContain("Show empty columns");
    expect(markup).toContain("Cardamom version v1.2.3");
  });
});

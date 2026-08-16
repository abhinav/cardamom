// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import {
  createElement,
} from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import { isCollectionRoute } from "./app.tsx";
import {
  CollectionSearchControlView,
} from "./collection-search-control.tsx";
import {
  IssueStatus,
  IssueType,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import {
  clearIssueFilters,
  defaultBoardView,
  defaultListView,
  type IssueViewPreferences,
} from "./issue-collection.ts";
import { IssueControls } from "./issue-views.tsx";

afterEach(cleanup);

describe("issue collection toolbar", () => {
  it.each([
    ["board", defaultBoardView],
    ["list", defaultListView],
  ] as const)("keeps the %s issue count inline with its controls", (mode, view) => {
    const markup = renderToStaticMarkup(createElement(IssueControls, {
      loadedCount: 12,
      mode,
      totalCount: 37,
      updateFilters: vi.fn(),
      updateView: vi.fn(),
      view,
    }));

    expect(markup).toContain('role="toolbar"');
    expect(markup).toContain('aria-label="Issue collection controls"');
    expect(markup).toContain("12/37 issues");
  });

  it.each([
    ["board", defaultBoardView],
    ["list", defaultListView],
  ] as const)(
    "shows one adaptive %s search control without a popover",
    (mode, defaultView) => {
      const activeView = {
        ...defaultView,
        filters: {
          ...defaultView.filters,
          query: "filter panel",
        },
      };

      const emptyMarkup = renderToStaticMarkup(createElement(IssueControls, {
        mode,
        updateFilters: vi.fn(),
        updateView: vi.fn(),
        view: defaultView,
      }));
      const activeMarkup = renderToStaticMarkup(createElement(IssueControls, {
        mode,
        updateFilters: vi.fn(),
        updateView: vi.fn(),
        view: activeView,
      }));

      expect(emptyMarkup).toContain('aria-label="Search issues"');
      expect(emptyMarkup).not.toContain("collection-search-query");
      expect(emptyMarkup).not.toContain('aria-label="Clear search"');
      expect(emptyMarkup).not.toContain('placeholder="Search issue titles"');
      expect(emptyMarkup).not.toContain("collection-search-panel");
      expect(activeMarkup).toContain('aria-label="Search issues"');
      expect(activeMarkup).toContain(
        '<span class="collection-search-query" title="filter panel">filter panel</span>',
      );
      expect(activeMarkup).toContain('aria-label="Clear search"');
      expect(activeMarkup).not.toContain('placeholder="Search issue titles"');
      expect(activeMarkup).not.toContain("collection-search-panel");
    },
  );

  it.each([
    ["board", defaultBoardView, "all"],
    ["list", defaultListView, "current"],
  ] as const)(
    "clears every %s filter including title search",
    (mode, defaultView, activeLifecycle) => {
      const filteredView = {
        ...defaultView,
        filters: {
          lifecycle: activeLifecycle,
          status: IssueStatus.IN_PROGRESS,
          type: IssueType.TASK,
          actor: "worker-a",
          label: "area:web",
          query: "filter panel",
        },
      };
      const searchOnlyView = {
        ...defaultView,
        filters: {
          ...defaultView.filters,
          query: "filter panel",
        },
      };

      expect(clearIssueFilters(mode)).toEqual(defaultView.filters);

      expect(clearButtonDisabled(mode, defaultView)).toBe(true);
      expect(clearButtonDisabled(mode, filteredView)).toBe(false);
      expect(clearButtonDisabled(mode, searchOnlyView)).toBe(false);
    },
  );
});

describe("collection search control", () => {
  it("opens the inline editor and focuses its input", () => {
    const beginEditing = vi.fn();
    render(
      <CollectionSearchControlView
        filters={defaultBoardView.filters}
        setFilters={vi.fn()}
        editing={false}
        beginEditing={beginEditing}
        endEditing={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByLabelText("Search issues"));

    expect(beginEditing).toHaveBeenCalledOnce();

    cleanup();
    const { container } = render(
      <CollectionSearchControlView
        filters={{ ...defaultBoardView.filters, query: "filter panel" }}
        setFilters={vi.fn()}
        editing
        beginEditing={vi.fn()}
        endEditing={vi.fn()}
      />,
    );
    const input = screen.getByLabelText("Search issue titles");

    expect(input).toHaveFocus();
    expect(input).toHaveValue("filter panel");
    expect(container.querySelector(".collection-search-query")).toBeNull();
    expect(container.querySelector(".collection-search-panel")).toBeNull();
  });

  it("clears only the title search", () => {
    const filters = {
      ...defaultBoardView.filters,
      lifecycle: "all" as const,
      status: IssueStatus.IN_PROGRESS,
      type: IssueType.TASK,
      actor: "worker-a",
      label: "area:web",
      query: "filter panel",
    };
    const setFilters = vi.fn();
    render(
      <CollectionSearchControlView
        filters={filters}
        setFilters={setFilters}
        editing
        beginEditing={vi.fn()}
        endEditing={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByLabelText("Clear search"));

    expect(setFilters).toHaveBeenCalledExactlyOnceWith(
      {
        ...filters,
        query: "",
      },
      "replace",
    );
  });

  it("replaces the current history entry while editing title search", () => {
    const setFilters = vi.fn();
    render(
      <CollectionSearchControlView
        filters={defaultBoardView.filters}
        setFilters={setFilters}
        editing
        beginEditing={vi.fn()}
        endEditing={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByLabelText("Search issue titles"), {
      target: { value: "route filters" },
    });

    expect(setFilters).toHaveBeenCalledExactlyOnceWith(
      { ...defaultBoardView.filters, query: "route filters" },
      "replace",
    );
  });

  it("collapses only when focus leaves the complete control", () => {
    const endEditing = vi.fn();
    const control = CollectionSearchControlView({
      filters: defaultBoardView.filters,
      setFilters: vi.fn(),
      editing: true,
      beginEditing: vi.fn(),
      endEditing,
    });
    const onBlur = control.props.onBlur as (event: {
      currentTarget: { contains: (target: unknown) => boolean };
      relatedTarget: unknown;
    }) => void;
    const relatedTarget = {};

    onBlur({
      currentTarget: { contains: () => true },
      relatedTarget,
    });
    expect(endEditing).not.toHaveBeenCalled();

    onBlur({
      currentTarget: { contains: () => false },
      relatedTarget,
    });
    expect(endEditing).toHaveBeenCalledOnce();

    const closedEndEditing = vi.fn();
    const closedControl = CollectionSearchControlView({
      filters: defaultBoardView.filters,
      setFilters: vi.fn(),
      editing: false,
      beginEditing: vi.fn(),
      endEditing: closedEndEditing,
    });
    const closedOnBlur = closedControl.props.onBlur as typeof onBlur;

    closedOnBlur({
      currentTarget: { contains: () => false },
      relatedTarget,
    });
    expect(closedEndEditing).not.toHaveBeenCalled();
  });
});

describe("collection shell", () => {
  it.each([
    ["/", false],
    ["/board/board-1", true],
    ["/board/board-1/list", true],
    ["/all", true],
    ["/all/list", true],
    ["/board/board-1/approvals", false],
    ["/all/routines", false],
    ["/board/board-1/issue/cm-123", false],
  ])("classifies %s collection ownership", (pathname, expected) => {
    expect(isCollectionRoute(pathname)).toBe(expected);
  });
});

function clearButtonDisabled(
  mode: "board" | "list",
  view: IssueViewPreferences,
): boolean {
  render(
    <IssueControls
      mode={mode}
      updateFilters={vi.fn()}
      updateView={vi.fn()}
      view={view}
    />,
  );
  fireEvent.click(screen.getByLabelText(/^Filters/));
  const disabled = screen.getByRole("button", {
    name: "Clear filters",
  }).hasAttribute("disabled");
  cleanup();
  return disabled;
}

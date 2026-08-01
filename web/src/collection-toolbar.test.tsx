import {
  Children,
  createElement,
  isValidElement,
  type ReactElement,
  type ReactNode,
} from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

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
} from "./issue-collection.ts";
import { IssueControls } from "./issue-views.tsx";

describe("issue collection toolbar", () => {
  it.each([
    ["board", defaultBoardView],
    ["list", defaultListView],
  ] as const)("keeps the %s issue count inline with its controls", (mode, view) => {
    const markup = renderToStaticMarkup(createElement(IssueControls, {
      loadedCount: 12,
      mode,
      totalCount: 37,
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
        updateView: vi.fn(),
        view: defaultView,
      }));
      const activeMarkup = renderToStaticMarkup(createElement(IssueControls, {
        mode,
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

      const defaultMarkup = renderToStaticMarkup(createElement(IssueControls, {
        mode,
        updateView: vi.fn(),
        view: defaultView,
      }));
      const filteredMarkup = renderToStaticMarkup(createElement(IssueControls, {
        mode,
        updateView: vi.fn(),
        view: filteredView,
      }));
      const searchOnlyMarkup = renderToStaticMarkup(createElement(IssueControls, {
        mode,
        updateView: vi.fn(),
        view: searchOnlyView,
      }));
      const defaultButton = defaultMarkup.match(
        /<button[^>]*>Clear filters<\/button>/,
      );
      const filteredButton = filteredMarkup.match(
        /<button[^>]*>Clear filters<\/button>/,
      );
      const searchOnlyButton = searchOnlyMarkup.match(
        /<button[^>]*>Clear filters<\/button>/,
      );

      expect(defaultButton).toHaveLength(1);
      expect(defaultButton?.[0]).toContain("disabled");
      expect(filteredButton).toHaveLength(1);
      expect(filteredButton?.[0]).not.toContain("disabled");
      expect(searchOnlyButton).toHaveLength(1);
      expect(searchOnlyButton?.[0]).not.toContain("disabled");
    },
  );
});

describe("collection search control", () => {
  it("opens the inline editor and focuses its input", () => {
    const beginEditing = vi.fn();
    const closedControl = CollectionSearchControlView({
      filters: defaultBoardView.filters,
      setFilters: vi.fn(),
      editing: false,
      beginEditing,
      endEditing: vi.fn(),
    });

    elementWithAriaLabel(closedControl, "Search issues").props.onClick?.({});

    expect(beginEditing).toHaveBeenCalledOnce();

    const editingControl = CollectionSearchControlView({
      filters: {
        ...defaultBoardView.filters,
        query: "filter panel",
      },
      setFilters: vi.fn(),
      editing: true,
      beginEditing: vi.fn(),
      endEditing: vi.fn(),
    });
    const input = elementWithAriaLabel(
      editingControl,
      "Search issue titles",
    );
    const markup = renderToStaticMarkup(editingControl);

    expect(input.props.autoFocus).toBe(true);
    expect(markup).toContain('value="filter panel"');
    expect(markup).not.toContain("collection-search-query");
    expect(markup).not.toContain("collection-search-panel");
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
    const control = CollectionSearchControlView({
      filters,
      setFilters,
      editing: true,
      beginEditing: vi.fn(),
      endEditing: vi.fn(),
    });

    elementWithAriaLabel(control, "Clear search").props.onClick?.({});

    expect(setFilters).toHaveBeenCalledExactlyOnceWith({
      ...filters,
      query: "",
    });
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

interface TestElementProps {
  autoFocus?: boolean;
  children?: ReactNode;
  "aria-label"?: string;
  onClick?: (event: unknown) => void;
}

function elementWithAriaLabel(
  root: ReactNode,
  label: string,
): ReactElement<TestElementProps> {
  const element = findElementWithAriaLabel(root, label);
  if (element === undefined) {
    throw new Error(`No element has aria-label ${label}`);
  }
  return element;
}

function findElementWithAriaLabel(
  root: ReactNode,
  label: string,
): ReactElement<TestElementProps> | undefined {
  if (!isValidElement<TestElementProps>(root)) {
    return undefined;
  }
  if (root.props["aria-label"] === label) {
    return root;
  }
  for (const child of Children.toArray(root.props.children)) {
    const element = findElementWithAriaLabel(child, label);
    if (element !== undefined) {
      return element;
    }
  }
  return undefined;
}

import {
  Children,
  isValidElement,
  type ReactElement,
  type ReactNode,
} from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  BoardSelectorBoardRow,
  BoardSelectorView,
  groupBoardsBySourceAndProject,
} from "./board-selector.tsx";
import { create } from "@bufbuild/protobuf";
import {
  BoardSummarySchema,
  ProjectSchema,
} from "./gen/cardamom/private/v1/project_pb.ts";
import {
  BoardRefSchema,
  ProjectRefSchema,
  SourceCatalogEntrySchema,
  SourceRefSchema,
} from "./gen/cardamom/private/v1/source_pb.ts";

const projects = [
  { id: "project-z", name: "Zebra" },
  { id: "project-a", name: "Alpha" },
  { id: "project-m", name: "Middle" },
];

const boards = [
  { id: "board-z", projectId: "project-z", name: "Solo" },
  { id: "board-a2", projectId: "project-a", name: "Web" },
  { id: "board-a1", projectId: "project-a", name: "API" },
  { id: "board-m", projectId: "project-m", name: "Operations" },
];

describe("board selector", () => {
  it("groups aggregate boards by source and then project", () => {
    const builder = create(SourceRefSchema, {
      sourceId: "builder",
      storeLineageId: "lineage-builder",
    });
    const laptop = create(SourceRefSchema, {
      sourceId: "laptop",
      storeLineageId: "lineage-laptop",
    });
    const groups = groupBoardsBySourceAndProject(
      [
        create(SourceCatalogEntrySchema, { source: laptop }),
        create(SourceCatalogEntrySchema, { source: builder }),
      ],
      [
        create(ProjectSchema, {
          id: "project-1",
          name: "Build",
          ref: create(ProjectRefSchema, { source: builder, projectId: "project-1" }),
        }),
        create(ProjectSchema, {
          id: "project-1",
          name: "Build",
          ref: create(ProjectRefSchema, { source: laptop, projectId: "project-1" }),
        }),
      ],
      [
        create(BoardSummarySchema, {
          id: "builder-board",
          projectId: "project-1",
          name: "Builder board",
          ref: create(BoardRefSchema, { source: builder, boardId: "builder-board" }),
        }),
        create(BoardSummarySchema, {
          id: "laptop-board",
          projectId: "project-1",
          name: "Laptop board",
          ref: create(BoardRefSchema, { source: laptop, boardId: "laptop-board" }),
        }),
      ],
    );

    expect(groups.map((group) => group.sourceId)).toEqual(["builder", "laptop"]);
    expect(groups.map((group) => group.projects[0]?.boards[0]?.name)).toEqual([
      "Builder board",
      "Laptop board",
    ]);
  });

  it("sorts projects and keeps grouping based on the complete catalog", () => {
    const allMarkup = renderSelector("");
    const filteredMarkup = renderSelector("web");

    expect(allMarkup.indexOf(">Alpha<")).toBeLessThan(
      allMarkup.indexOf(">Middle<"),
    );
    expect(allMarkup.indexOf(">Middle<")).toBeLessThan(
      allMarkup.indexOf(">Zebra<"),
    );
    expect(allMarkup).toContain(">Alpha<");
    expect(allMarkup).toContain(">2 boards<");
    expect(allMarkup.match(/board-selector-project-heading/g)).toHaveLength(1);

    expect(filteredMarkup).toContain(">Alpha<");
    expect(filteredMarkup).toContain(">2 boards<");
    expect(filteredMarkup).toContain(">Web<");
    expect(filteredMarkup).not.toContain(">API<");
    expect(filteredMarkup.match(/board-selector-project-heading/g))
      .toHaveLength(1);
  });

  it("marks the selected scope and shows board and project labels", () => {
    const markup = renderToStaticMarkup(BoardSelectorView({
      boards,
      open: true,
      projects,
      query: "",
      selection: { kind: "board", boardId: "board-m" },
      onDismiss: vi.fn(),
      onOpenBoardSettings: vi.fn(),
      onQueryChange: vi.fn(),
      onSelectScope: vi.fn(),
      onToggle: vi.fn(),
    }));

    expect(markup).toContain(
      '<span class="board-selector-trigger-primary">Operations</span>',
    );
    expect(markup).toContain(
      '<span class="board-selector-trigger-secondary">Middle</span>',
    );
    expect(markup).toContain('aria-label="Select board scope: Operations"');
    expect(markup).toMatch(
      /aria-current="true" aria-label="Select Operations"/,
    );
    expect(markup).toContain("lucide-check");
  });

  it("preserves board selection without exposing board settings", () => {
    const markup = renderToStaticMarkup(BoardSelectorView({
      boards,
      open: true,
      projects,
      query: "",
      selection: { kind: "board", boardId: "board-m" },
      onDismiss: vi.fn(),
      onQueryChange: vi.fn(),
      onSelectScope: vi.fn(),
      onToggle: vi.fn(),
    }));

    expect(markup).toContain('aria-label="Select Operations"');
    expect(markup).not.toContain("Open settings for");
  });

  it("exposes settings only for the board named by the current scope", () => {
    const markup = renderToStaticMarkup(BoardSelectorView({
      boards,
      open: true,
      projects,
      query: "",
      selection: { kind: "board", boardId: "board-m" },
      onDismiss: vi.fn(),
      onOpenBoardSettings: vi.fn(),
      onQueryChange: vi.fn(),
      onSelectScope: vi.fn(),
      onToggle: vi.fn(),
    }));

    expect(markup).toContain("Open settings for Operations");
    expect(markup).not.toContain("Open settings for Web");
    expect(markup).not.toContain("Open settings for API");
    expect(markup).not.toContain("Open settings for Solo");
  });

  it("exposes toggle, selection, settings, and Escape interactions", () => {
    const onDismiss = vi.fn();
    const onOpenBoardSettings = vi.fn();
    const onSelectScope = vi.fn();
    const onToggle = vi.fn();
    const view = BoardSelectorView({
      boards,
      open: true,
      projects,
      query: "",
      selection: { kind: "all" },
      onDismiss,
      onOpenBoardSettings,
      onQueryChange: vi.fn(),
      onSelectScope,
      onToggle,
    });

    elementWithAriaLabel(view, "Select board scope: All boards")
      .props.onClick?.();
    expect(onToggle).toHaveBeenCalledOnce();

    elementWithAriaLabel(view, "Select All boards").props.onClick?.();
    expect(onSelectScope).toHaveBeenNthCalledWith(1, {
      kind: "all",
    });

    const boardRow = BoardSelectorBoardRow({
      board: boards[1]!,
      projectName: "Alpha",
      selectedBoardId: "board-a2",
      onOpenBoardSettings,
      onSelectScope,
    });
    elementWithAriaLabel(boardRow, "Select Web").props.onClick?.();
    expect(onSelectScope).toHaveBeenNthCalledWith(2, {
      kind: "board",
      boardId: "board-a2",
    });

    elementWithAriaLabel(boardRow, "Open settings for Web").props.onClick?.();
    expect(onOpenBoardSettings).toHaveBeenCalledExactlyOnceWith("board-a2");

    const preventDefault = vi.fn();
    const stopPropagation = vi.fn();
    const onKeyDown = view.props.onKeyDown as (event: {
      key: string;
      preventDefault: () => void;
      stopPropagation: () => void;
    }) => void;
    onKeyDown({
      key: "Escape",
      preventDefault,
      stopPropagation,
    });
    expect(preventDefault).toHaveBeenCalledOnce();
    expect(stopPropagation).toHaveBeenCalledOnce();
    expect(onDismiss).toHaveBeenCalledOnce();
  });
});

function renderSelector(query: string): string {
  return renderToStaticMarkup(BoardSelectorView({
    boards,
    open: true,
    projects,
    query,
    selection: { kind: "all" },
    onDismiss: vi.fn(),
    onOpenBoardSettings: vi.fn(),
    onQueryChange: vi.fn(),
    onSelectScope: vi.fn(),
    onToggle: vi.fn(),
  }));
}

interface TestElementProps {
  children?: ReactNode;
  "aria-label"?: string;
  onClick?: () => void;
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

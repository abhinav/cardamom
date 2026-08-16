// @vitest-environment jsdom

import { create } from "@bufbuild/protobuf";
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router";

import {
  type BoardConfigurationTarget,
  BoardPickerRoute,
  BoardSelector,
  BoardSelectorBoardRow,
  BoardSelectorView,
  catalogBoards,
  groupBoardsBySourceAndProject,
} from "./board-selector.tsx";
import type { ResolvedBoardScope } from "./board-scope.ts";
import {
  BoardSummarySchema,
  ProjectSchema,
} from "./gen/cardamom/private/v1/project_pb.ts";
import {
  SourceCatalogEntrySchema,
  SourceRefSchema,
  type SourceCatalogEntry,
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

afterEach(cleanup);

describe("board selector", () => {
  it("focuses search when opened and restores trigger focus after Escape", async () => {
    render(
      <MemoryRouter>
        <BoardSelector
          aggregate={false}
          boards={boards}
          projects={projects}
          selection={{ kind: "all" }}
          onSelectScope={vi.fn()}
        />
      </MemoryRouter>,
    );
    const trigger = screen.getByRole("button", {
      name: "Select board scope: All boards",
    });

    fireEvent.click(trigger);
    await waitFor(() => {
      expect(screen.getByRole("searchbox", {
        name: "Search boards and projects",
      })).toHaveFocus();
    });

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(trigger).toHaveFocus();
    });
  });

  it("groups archived visibility with the board search controls", () => {
    const { container } = render(
      <MemoryRouter>
        <BoardPickerRoute aggregate={false} boards={boards} projects={projects} />
      </MemoryRouter>,
    );
    const filters = container.querySelector(".board-picker-filters");

    expect(filters).not.toBeNull();
    expect(filters?.querySelector(".board-picker-search")).not.toBeNull();
    expect(filters?.querySelector(".board-picker-archived")).not.toBeNull();
  });

  it("keeps archived boards out of quick selection and behind catalog opt-in", () => {
    const archived = {
      id: "board-old",
      projectId: "project-a",
      name: "Retired",
      archived: {
        $typeName: "cardamom.private.v1.BoardArchive" as const,
        actor: "captain",
      },
    };
    expect(catalogBoards([...boards, archived], false)).toEqual(boards);
    expect(catalogBoards([...boards, archived], true)).toContain(archived);

    renderSelector({
      boards: [...boards, archived],
      selection: { kind: "board", boardId: archived.id },
      onOpenBoardConfiguration: vi.fn(),
    });

    expect(screen.getByText("Retired")).toBeInTheDocument();
    expect(screen.getByText("Alpha · Archived")).toBeInTheDocument();
    expect(screen.queryByLabelText("Select Retired")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Board settings" }))
      .not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View all boards" })).toBeInTheDocument();
  });

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
        create(ProjectSchema, { id: "project-1", name: "Build", source: builder }),
        create(ProjectSchema, { id: "project-1", name: "Build", source: laptop }),
      ],
      [
        create(BoardSummarySchema, {
          id: "builder-board",
          projectId: "project-1",
          name: "Builder board",
          source: builder,
        }),
        create(BoardSummarySchema, {
          id: "laptop-board",
          projectId: "project-1",
          name: "Laptop board",
          source: laptop,
        }),
      ],
    );

    expect(groups.map((group) => group.sourceId)).toEqual(["builder", "laptop"]);
    expect(groups.map((group) => group.projects[0]?.boards[0]?.name)).toEqual([
      "Builder board",
      "Laptop board",
    ]);
  });

  it("sorts projects while filtering boards against the complete catalog", () => {
    renderSelector();
    expect(screen.getAllByRole("button", { name: /^Select (?!All boards)/ })
      .map((button) => button.getAttribute("aria-label"))).toEqual([
        "Select API",
        "Select Web",
        "Select Operations",
        "Select Solo",
      ]);
    expect(projectHeadings()).toEqual(["Alpha2 boards"]);

    cleanup();
    renderSelector({ query: "web" });
    expect(projectHeadings()).toEqual(["Alpha2 boards"]);
    expect(screen.getByLabelText("Select Web")).toBeInTheDocument();
    expect(screen.queryByLabelText("Select API")).not.toBeInTheDocument();
  });

  it("shows direct-server boards when bootstrap includes store lineage", () => {
    const source = create(SourceCatalogEntrySchema, {
      source: create(SourceRefSchema, { storeLineageId: "lineage-local" }),
    });
    renderSelector({ sources: [source] });

    expect(screen.getByLabelText("Select Operations")).toBeInTheDocument();
    expect(screen.getByLabelText("Select Web")).toBeInTheDocument();
  });

  it("marks the selected scope and shows board and project labels", () => {
    renderSelector({ selection: { kind: "board", boardId: "board-m" } });

    expect(screen.getByLabelText("Select board scope: Operations")).toHaveTextContent(
      "OperationsMiddle",
    );
    expect(screen.getByLabelText("Select Operations")).toHaveAttribute(
      "aria-current",
      "true",
    );
  });

  it("preserves board selection without exposing board settings", () => {
    renderSelector({ selection: { kind: "board", boardId: "board-m" } });

    expect(screen.getByLabelText("Select Operations")).toBeInTheDocument();
    expect(screen.queryByText("Board settings")).not.toBeInTheDocument();
  });

  it.each([
    { kind: "board", boardId: "board-m" } as const,
    { kind: "all" } as const,
  ])("exposes context for every board from $kind scope", (selection) => {
    renderSelector({ selection, onOpenBoardConfiguration: vi.fn() });

    for (const name of ["Operations", "Web", "API", "Solo"]) {
      expect(screen.getByLabelText(`Configure ${name}`)).toBeInTheDocument();
    }
  });

  it("keeps context and selection as separate interactions", () => {
    const onOpenBoardConfiguration = vi.fn();
    const onSelectScope = vi.fn();
    renderSelector({ onOpenBoardConfiguration, onSelectScope });

    fireEvent.click(screen.getByLabelText("Select All boards"));
    expect(onSelectScope).toHaveBeenCalledExactlyOnceWith({ kind: "all" });

    cleanup();
    const source = create(SourceRefSchema, {
      sourceId: "builder",
      storeLineageId: "lineage-builder",
    });
    const sourceBoard = { ...boards[1]!, source };
    render(
      <BoardSelectorBoardRow
        board={sourceBoard}
        projectName="Alpha"
        selectedBoardId="board-a2"
        onOpenBoardConfiguration={onOpenBoardConfiguration}
        onSelectScope={onSelectScope}
      />,
    );

    fireEvent.click(screen.getByLabelText("Select Web"));
    expect(onSelectScope).toHaveBeenNthCalledWith(2, {
      kind: "board",
      boardId: "board-a2",
      source,
    });
    fireEvent.click(screen.getByLabelText("Configure Web"));
    expect(onOpenBoardConfiguration).toHaveBeenCalledExactlyOnceWith(sourceBoard);
    expect(onSelectScope).toHaveBeenCalledTimes(2);
  });
});

interface SelectorOverrides {
  boards?: readonly (typeof boards[number] | {
    id: string;
    projectId: string;
    name: string;
    archived: {
      $typeName: "cardamom.private.v1.BoardArchive";
      actor: string;
    };
  })[];
  query?: string;
  selection?: { kind: "all" } | { kind: "board"; boardId: string };
  sources?: readonly SourceCatalogEntry[];
  onOpenBoardConfiguration?: (board: BoardConfigurationTarget) => void;
  onSelectScope?: (selection: ResolvedBoardScope) => void;
}

function renderSelector(overrides: SelectorOverrides = {}) {
  return render(
    <MemoryRouter>
      <BoardSelectorView
        aggregate={false}
        boards={overrides.boards ?? boards}
        open
        projects={projects}
        query={overrides.query ?? ""}
        selection={overrides.selection ?? { kind: "all" }}
        sources={overrides.sources}
        onDismiss={vi.fn()}
        onOpenBoardConfiguration={overrides.onOpenBoardConfiguration}
        onQueryChange={vi.fn()}
        onSelectScope={overrides.onSelectScope ?? vi.fn()}
        onToggle={vi.fn()}
      />
    </MemoryRouter>,
  );
}

function projectHeadings(): string[] {
  const dialog = screen.getByRole("dialog");
  return [...dialog.querySelectorAll(".board-selector-project-heading")]
    .map((heading) => heading.textContent ?? "");
}

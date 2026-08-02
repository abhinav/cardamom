import type { Ref } from "react";
import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import {
  Check,
  ChevronDown,
  Search,
  Settings,
} from "lucide-react";
import { Link } from "react-router";

import {
  boardScopePath,
  type BoardScopeSelection,
  type ResolvedBoardScope,
} from "./board-scope.ts";

interface AvailableProject {
  id: string;
  name: string;
}

interface AvailableBoard {
  id: string;
  projectId: string;
  name: string;
}

interface BoardSelectorProps {
  boards: readonly AvailableBoard[];
  projects: readonly AvailableProject[];
  selection: BoardScopeSelection;
  onOpenBoardSettings?: (boardId: string) => void;
  onSelectScope: (selection: ResolvedBoardScope) => void;
}

interface BoardPickerRouteProps {
  boards: readonly AvailableBoard[];
  projects: readonly AvailableProject[];
}

/** BoardPickerRoute renders the complete board catalog at the root route. */
export function BoardPickerRoute({
  boards,
  projects,
}: BoardPickerRouteProps) {
  const [query, setQuery] = useState("");
  const units = visibleProjectUnits(projects, boards, query);

  return (
    <section className="board-picker" aria-labelledby="board-picker-title">
      <header className="board-picker-header">
        <div>
          <p className="route-kicker">Scope</p>
          <h1 id="board-picker-title">Boards</h1>
        </div>
        <Link className="board-picker-all" to={boardScopePath({ kind: "all" })}>
          <span>All boards</span>
          <span>{catalogSummary(projects.length, boards.length)}</span>
        </Link>
      </header>
      <label className="board-picker-search">
        <Search aria-hidden="true" />
        <span className="sr-only">Search boards and projects</span>
        <input
          type="search"
          value={query}
          placeholder="Find a board or project"
          onChange={(event) => setQuery(event.currentTarget.value)}
        />
      </label>
      <div className="board-picker-projects">
        {units.map((unit) => (
          <section
            key={unit.project.id}
            className="board-picker-project"
            aria-labelledby={`board-picker-${unit.project.id}`}
          >
            <header>
              <h2 id={`board-picker-${unit.project.id}`}>
                {unit.project.name}
              </h2>
              <span>
                {unit.totalBoardCount}{" "}
                {unit.totalBoardCount === 1 ? "board" : "boards"}
              </span>
            </header>
            <div className="board-picker-links">
              {unit.boards.map((board) => (
                <Link
                  key={board.id}
                  to={boardScopePath({ kind: "board", boardId: board.id })}
                >
                  <span>{board.name}</span>
                  <span>{board.id}</span>
                </Link>
              ))}
            </div>
          </section>
        ))}
        {units.length === 0 && (
          <p className="board-picker-empty">
            No boards or projects match your search.
          </p>
        )}
      </div>
    </section>
  );
}

export function BoardSelector({
  boards,
  projects,
  selection,
  onOpenBoardSettings,
  onSelectScope,
}: BoardSelectorProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const dialogId = useId();
  const dismiss = useCallback(() => {
    setOpen(false);
    setQuery("");
    triggerRef.current?.focus();
  }, []);

  useEffect(() => {
    if (!open) {
      return;
    }
    searchRef.current?.focus();
    const closeOnOutsidePointer = (event: PointerEvent) => {
      if (
        event.target instanceof Node &&
        !rootRef.current?.contains(event.target)
      ) {
        dismiss();
      }
    };
    document.addEventListener("pointerdown", closeOnOutsidePointer);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsidePointer);
    };
  }, [dismiss, open]);

  return (
    <BoardSelectorView
      boards={boards}
      dialogId={dialogId}
      open={open}
      projects={projects}
      query={query}
      searchRef={searchRef}
      selection={selection}
      triggerRef={triggerRef}
      onDismiss={dismiss}
      onOpenBoardSettings={
        onOpenBoardSettings === undefined
          ? undefined
          : (boardId) => {
              dismiss();
              onOpenBoardSettings(boardId);
            }
      }
      onQueryChange={setQuery}
      onSelectScope={(nextSelection) => {
        onSelectScope(nextSelection);
        dismiss();
      }}
      onToggle={() => {
        if (open) {
          dismiss();
        } else {
          setOpen(true);
        }
      }}
      rootRef={rootRef}
    />
  );
}

interface BoardSelectorViewProps extends BoardSelectorProps {
  dialogId?: string;
  open: boolean;
  query: string;
  rootRef?: Ref<HTMLDivElement>;
  searchRef?: Ref<HTMLInputElement>;
  triggerRef?: Ref<HTMLButtonElement>;
  onDismiss: () => void;
  onQueryChange: (query: string) => void;
  onToggle: () => void;
}

export function BoardSelectorView({
  boards,
  dialogId = "board-selector-dialog",
  open,
  projects,
  query,
  rootRef,
  searchRef,
  selection,
  triggerRef,
  onDismiss,
  onOpenBoardSettings,
  onQueryChange,
  onSelectScope,
  onToggle,
}: BoardSelectorViewProps) {
  const labels = selectedScopeLabels(projects, boards, selection);
  const units = visibleProjectUnits(projects, boards, query);
  const allBoardsSelected = selection.kind === "all";
  const storeSummary = catalogSummary(projects.length, boards.length);

  return (
    <div
      className="board-selector"
      ref={rootRef}
      onKeyDown={(event) => {
        if (event.key !== "Escape" || !open) {
          return;
        }
        event.preventDefault();
        event.stopPropagation();
        onDismiss();
      }}
    >
      <button
        ref={triggerRef}
        type="button"
        className="board-selector-trigger"
        aria-controls={dialogId}
        aria-expanded={open}
        aria-haspopup="dialog"
        aria-label={`Select board scope: ${labels.primary}`}
        onClick={onToggle}
      >
        <span className="board-selector-trigger-copy">
          <span className="board-selector-trigger-primary">
            {labels.primary}
          </span>
          <span className="board-selector-trigger-secondary">
            {labels.secondary}
          </span>
        </span>
        <ChevronDown
          className="board-selector-disclosure"
          data-open={open}
          aria-hidden="true"
        />
      </button>
      {open && (
        <section
          id={dialogId}
          className="board-selector-dialog"
          role="dialog"
          aria-labelledby={`${dialogId}-title`}
        >
          <h2 id={`${dialogId}-title`} className="sr-only">
            Select board scope
          </h2>
          <label className="board-selector-search">
            <Search aria-hidden="true" />
            <span className="sr-only">Search boards and projects</span>
            <input
              ref={searchRef}
              type="search"
              value={query}
              placeholder="Find a board or project"
              aria-label="Search boards and projects"
              onChange={(event) => onQueryChange(event.currentTarget.value)}
            />
          </label>
          <div className="board-selector-results">
            <div className="board-selector-row">
              <button
                type="button"
                className="board-selector-option"
                aria-current={allBoardsSelected || undefined}
                aria-label="Select All boards"
                onClick={() => onSelectScope({ kind: "all" })}
              >
                <SelectionMark selected={allBoardsSelected} />
                <span className="board-selector-option-copy">
                  <span className="board-selector-option-primary">
                    All boards
                  </span>
                  <span className="board-selector-option-secondary">
                    {storeSummary}
                  </span>
                </span>
              </button>
              <span className="board-selector-action-space" aria-hidden="true" />
            </div>
            {units.map((unit) =>
              unit.totalBoardCount === 1
                ? (
                    <BoardSelectorBoardRow
                      key={unit.boards[0]?.id}
                      board={unit.boards[0]!}
                      projectName={unit.project.name}
                      selectedBoardId={
                        selection.kind === "board"
                          ? selection.boardId
                          : undefined
                      }
                      onOpenBoardSettings={onOpenBoardSettings}
                      onSelectScope={onSelectScope}
                    />
                  )
                : (
                    <section
                      key={unit.project.id}
                      className="board-selector-project"
                      aria-labelledby={`${dialogId}-${unit.project.id}`}
                    >
                      <div
                        id={`${dialogId}-${unit.project.id}`}
                        className="board-selector-project-heading"
                      >
                        <span>{unit.project.name}</span>
                        <span>
                          {unit.totalBoardCount}{" "}
                          {unit.totalBoardCount === 1 ? "board" : "boards"}
                        </span>
                      </div>
                      {unit.boards.map((board) => (
                        <BoardSelectorBoardRow
                          key={board.id}
                          board={board}
                          projectName={unit.project.name}
                          selectedBoardId={
                            selection.kind === "board"
                              ? selection.boardId
                              : undefined
                          }
                          onOpenBoardSettings={onOpenBoardSettings}
                          onSelectScope={onSelectScope}
                        />
                      ))}
                    </section>
                  ),
            )}
            {units.length === 0 && (
              <p className="board-selector-empty">
                No boards or projects match your search.
              </p>
            )}
          </div>
        </section>
      )}
    </div>
  );
}

interface BoardSelectorBoardRowProps {
  board: AvailableBoard;
  projectName: string;
  selectedBoardId: string | undefined;
  onOpenBoardSettings?: (boardId: string) => void;
  onSelectScope: (selection: ResolvedBoardScope) => void;
}

export function BoardSelectorBoardRow({
  board,
  projectName,
  selectedBoardId,
  onOpenBoardSettings,
  onSelectScope,
}: BoardSelectorBoardRowProps) {
  const selected = board.id === selectedBoardId;
  return (
    <div className="board-selector-row">
      <button
        type="button"
        className="board-selector-option"
        aria-current={selected || undefined}
        aria-label={`Select ${board.name}`}
        onClick={() =>
          onSelectScope({ kind: "board", boardId: board.id })
        }
      >
        <SelectionMark selected={selected} />
        <span className="board-selector-option-copy">
          <span className="board-selector-option-primary">{board.name}</span>
          <span className="board-selector-option-secondary">
            {projectName}
          </span>
        </span>
      </button>
      {onOpenBoardSettings === undefined || !selected ? (
        <span className="board-selector-action-space" aria-hidden="true" />
      ) : (
        <button
          type="button"
          className="board-selector-action"
          aria-label={`Open settings for ${board.name}`}
          title={`Board settings: ${board.name}`}
          onClick={() => onOpenBoardSettings(board.id)}
        >
          <Settings aria-hidden="true" />
        </button>
      )}
    </div>
  );
}

function SelectionMark({ selected }: { selected: boolean }) {
  return (
    <span className="board-selector-selection-mark" aria-hidden="true">
      {selected && <Check />}
    </span>
  );
}

interface ProjectUnit {
  project: AvailableProject;
  boards: AvailableBoard[];
  totalBoardCount: number;
}

function visibleProjectUnits(
  projects: readonly AvailableProject[],
  boards: readonly AvailableBoard[],
  query: string,
): ProjectUnit[] {
  const projectById = new Map(
    projects.map((project) => [project.id, project]),
  );
  const boardsByProject = new Map<string, AvailableBoard[]>();
  for (const board of boards) {
    const projectBoards = boardsByProject.get(board.projectId) ?? [];
    projectBoards.push(board);
    boardsByProject.set(board.projectId, projectBoards);
  }

  const normalizedQuery = query.trim().toLocaleLowerCase();
  return [...boardsByProject.entries()]
    .map(([projectId, projectBoards]) => {
      const project = projectById.get(projectId) ?? {
        id: projectId,
        name: projectId,
      };
      const projectMatches = normalizedName(project.name).includes(
        normalizedQuery,
      );
      const visibleBoards = projectBoards
        .filter(
          (board) =>
            normalizedQuery === "" ||
            projectMatches ||
            normalizedName(board.name).includes(normalizedQuery),
        )
        .sort(compareByName);
      return {
        project,
        boards: visibleBoards,
        totalBoardCount: projectBoards.length,
      };
    })
    .filter((unit) => unit.boards.length > 0)
    .sort((left, right) => compareByName(left.project, right.project));
}

function selectedScopeLabels(
  projects: readonly AvailableProject[],
  boards: readonly AvailableBoard[],
  selection: BoardScopeSelection,
): { primary: string; secondary: string } {
  if (selection.kind === "all") {
    return {
      primary: "All boards",
      secondary: catalogSummary(projects.length, boards.length),
    };
  }
  if (selection.kind === "unresolved") {
    return {
      primary: "Select a board",
      secondary: catalogSummary(projects.length, boards.length),
    };
  }
  const board = boards.find((candidate) => candidate.id === selection.boardId);
  if (board === undefined) {
    return {
      primary: selection.boardId,
      secondary: "Board unavailable",
    };
  }
  return {
    primary: board.name,
    secondary:
      projects.find((project) => project.id === board.projectId)?.name ??
      board.projectId,
  };
}

function catalogSummary(projectCount: number, boardCount: number): string {
  const projectLabel = projectCount === 1 ? "project" : "projects";
  const boardLabel = boardCount === 1 ? "board" : "boards";
  return `${projectCount} ${projectLabel} / ${boardCount} ${boardLabel}`;
}

function normalizedName(name: string): string {
  return name.toLocaleLowerCase();
}

function compareByName(
  left: { id: string; name: string },
  right: { id: string; name: string },
): number {
  return left.name.localeCompare(right.name, undefined, {
    sensitivity: "base",
  }) || left.id.localeCompare(right.id);
}

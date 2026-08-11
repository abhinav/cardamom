import type { Ref, ReactNode } from "react";
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
  boardScopeHref,
  type BoardScopeSelection,
  type ResolvedBoardScope,
} from "./board-scope.ts";
import type { BoardSummary, Project } from "./gen/cardamom/private/v1/project_pb.ts";
import { SourceHealth, type SourceCatalogEntry } from "./gen/cardamom/private/v1/source_pb.ts";

interface AvailableProject {
  id: string;
  name: string;
  source?: Project["source"];
}

interface AvailableBoard {
  id: string;
  projectId: string;
  name: string;
  source?: BoardSummary["source"];
  /** archived is present when the board is readable but omitted from active discovery. */
  archived?: BoardSummary["archived"];
}

interface AvailableSource {
  source?: SourceCatalogEntry["source"];
  health?: SourceHealth;
  diagnostic?: string;
  version?: string;
}

interface BoardSelectorProps {
  aggregate: boolean;
  boards: readonly AvailableBoard[];
  projects: readonly AvailableProject[];
  sources?: readonly AvailableSource[];
  selection: BoardScopeSelection;
  onOpenBoardSettings?: (boardId: string) => void;
  onSelectScope: (selection: ResolvedBoardScope) => void;
}

interface BoardPickerRouteProps {
  aggregate: boolean;
  boards: readonly AvailableBoard[];
  projects: readonly AvailableProject[];
  sources?: readonly AvailableSource[];
}

/** BoardPickerRoute renders the complete board catalog at the root route. */
export function BoardPickerRoute({
  aggregate,
  boards,
  projects,
  sources = [],
}: BoardPickerRouteProps) {
  const [query, setQuery] = useState("");
  const [showArchived, setShowArchived] = useState(false);
  const visibleBoards = catalogBoards(boards, showArchived);
  const units = visibleProjectUnits(projects, visibleBoards, query);

  return (
    <section className="board-picker" aria-labelledby="board-picker-title">
      <header className="board-picker-header">
        <div>
          <p className="route-kicker">Scope</p>
          <h1 id="board-picker-title">Boards</h1>
        </div>
        <Link className="board-picker-all" to={boardScopeHref({ kind: "all" })}>
          <span>{aggregate ? "All sources" : "All boards"}</span>
          <span>{catalogSummary(projects.length, boards.length)}</span>
        </Link>
      </header>
      <div className="board-picker-filters">
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
        <label className="board-picker-archived">
          <input
            type="checkbox"
            checked={showArchived}
            onChange={(event) => setShowArchived(event.currentTarget.checked)}
          />
          <span>Show archived</span>
        </label>
      </div>
      <div className="board-picker-projects">
        {aggregate && <SourceHeadings sources={sources} />}
        {units.map((unit) => (
          <section
            key={`${unit.sourceId ?? "local"}:${unit.project.id}`}
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
                <Link key={board.id} to={boardScopePath({ kind: "board", boardId: board.id })}>
                  <span>{board.name}</span>
                  <span>{board.archived === undefined ? board.id : `${board.id} · Archived`}</span>
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

/**
 * catalogBoards defaults the Boards page to active discovery. Opting in
 * preserves the complete catalog so an archived board remains navigable.
 */
export function catalogBoards(
  boards: readonly AvailableBoard[],
  showArchived: boolean,
): readonly AvailableBoard[] {
  return showArchived ? boards : boards.filter((board) => board.archived === undefined);
}

export function BoardSelector({
  aggregate,
  boards,
  projects,
  sources = [],
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
      aggregate={aggregate}
      boards={boards}
      dialogId={dialogId}
      open={open}
      projects={projects}
      sources={sources}
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
  aggregate,
  boards,
  dialogId = "board-selector-dialog",
  open,
  projects,
  sources = [],
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
  const labels = selectedScopeLabels(
    aggregate,
    projects,
    boards,
    sources,
    selection,
  );
  // The selector's labels use the full catalog so an explicitly selected
  // archived board remains identified, while quick switching stays active-only.
  const activeBoards = boards.filter((board) => board.archived === undefined);
  const selectedArchivedBoard = selection.kind === "board"
    ? boards.find(
        (board) => board.id === selection.boardId && board.archived !== undefined,
      )
    : undefined;
  const units = visibleProjectUnits(projects, activeBoards, query);
  const allBoardsSelected = selection.kind === "all";
  const storeSummary = catalogSummary(projects.length, activeBoards.length);

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
                aria-label={`Select ${aggregate ? "All sources" : "All boards"}`}
                onClick={() => onSelectScope({ kind: "all" })}
              >
                <SelectionMark selected={allBoardsSelected} />
                <span className="board-selector-option-copy">
                  <span className="board-selector-option-primary">
                    {aggregate ? "All sources" : "All boards"}
                  </span>
                  <span className="board-selector-option-secondary">
                    {storeSummary}
                  </span>
                </span>
              </button>
              <span className="board-selector-action-space" aria-hidden="true" />
            </div>
            {aggregate
              ? renderAggregateUnits(
                sources,
                units,
                selection,
                onOpenBoardSettings,
                onSelectScope,
              )
              : units.map((unit) =>
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
                          key={`${unit.sourceId ?? "local"}:${unit.project.id}`}
                          className="board-selector-project"
                          aria-labelledby={`${dialogId}-${unit.project.id}`}
                        >
                          <div
                            id={`${dialogId}-${unit.project.id}`}
                            className="board-selector-project-heading"
                          >
                            <span>{unit.project.name}</span>
                            <span>
                              {unit.totalBoardCount} {unit.totalBoardCount === 1 ? "board" : "boards"}
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
          {selectedArchivedBoard !== undefined && onOpenBoardSettings !== undefined && (
            <button
              type="button"
              className="board-selector-catalog-link"
              onClick={() => onOpenBoardSettings(selectedArchivedBoard.id)}
            >
              Board settings
            </button>
          )}
          <a className="board-selector-catalog-link" href="/" onClick={onDismiss}>
            View all boards
          </a>
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
  // Without a concrete board scope, each row owns the settings entry point for
  // its board. Concrete scope keeps that action on the selected row alone.
  const showSettings = onOpenBoardSettings !== undefined &&
    (selectedBoardId === undefined || selected);
  return (
    <div className="board-selector-row">
      <button
        type="button"
        className="board-selector-option"
        aria-current={selected || undefined}
        aria-label={`Select ${board.name}`}
        onClick={() =>
          onSelectScope({ kind: "board", boardId: board.id, source: board.source })
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
      {showSettings ? (
        <button
          type="button"
          className="board-selector-action"
          aria-label={`Open settings for ${board.name}`}
          title={`Board settings: ${board.name}`}
          onClick={() => onOpenBoardSettings(board.id)}
        >
          <Settings aria-hidden="true" />
        </button>
      ) : (
        <span className="board-selector-action-space" aria-hidden="true" />
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

export interface ProjectUnit {
  project: AvailableProject;
  boards: AvailableBoard[];
  sourceId?: string;
  sourceName?: string;
  totalBoardCount: number;
}

/** SourceProjectGroup is one source and its visible project groups. */
export interface SourceProjectGroup {
  sourceId: string;
  sourceName: string;
  projects: readonly ProjectUnit[];
}

function visibleProjectUnits(
  projects: readonly AvailableProject[],
  boards: readonly AvailableBoard[],
  query: string,
): ProjectUnit[] {
  const projectById = new Map(
    projects.map((project) => [projectKey(project.source?.sourceId, project.id), project]),
  );
  const boardsByProject = new Map<string, AvailableBoard[]>();
  for (const board of boards) {
    const key = projectKey(board.source?.sourceId, board.projectId);
    const projectBoards = boardsByProject.get(key) ?? [];
    projectBoards.push(board);
    boardsByProject.set(key, projectBoards);
  }

  const normalizedQuery = query.trim().toLocaleLowerCase();
  return [...boardsByProject.entries()]
    .map(([key, projectBoards]) => {
      const sourceId = projectBoards[0]?.source?.sourceId;
      const projectId = projectBoards[0]?.projectId ?? key;
      const project = projectById.get(projectKey(sourceId, projectId)) ?? {
        id: projectId,
        name: projectId,
      };
      const sourceName = sourceId ?? "Local";
      const projectMatches = normalizedName(project.name).includes(
        normalizedQuery,
      );
      const sourceMatches = normalizedName(sourceName).includes(normalizedQuery);
      const visibleBoards = projectBoards
        .filter(
          (board) =>
            normalizedQuery === "" ||
            projectMatches ||
            sourceMatches ||
            normalizedName(board.name).includes(normalizedQuery),
        )
        .sort(compareByName);
      return {
        project,
        boards: visibleBoards,
        sourceId,
        sourceName,
        totalBoardCount: projectBoards.length,
      };
    })
    .filter((unit) => unit.boards.length > 0)
    .sort((left, right) =>
      (left.sourceName ?? "").localeCompare(right.sourceName ?? "", undefined, { sensitivity: "base" }) ||
      compareByName(left.project, right.project));
}

/** groupBoardsBySourceAndProject preserves source-qualified grouping identity. */
export function groupBoardsBySourceAndProject(
  sources: readonly AvailableSource[],
  projects: readonly AvailableProject[],
  boards: readonly AvailableBoard[],
): SourceProjectGroup[] {
  const units = visibleProjectUnits(projects, boards, "");
  return sources.map((source) => ({
    sourceId: source.source?.sourceId ?? "",
    sourceName: source.source?.sourceId ?? "Unknown source",
    projects: units.filter(
      (unit) => unit.sourceId === source.source?.sourceId,
    ),
  })).filter((group) => group.projects.length > 0)
    .sort((left, right) => compareByName(
      { id: left.sourceId, name: left.sourceName },
      { id: right.sourceId, name: right.sourceName },
    ));
}

function renderAggregateUnits(
  sources: readonly AvailableSource[],
  units: readonly ProjectUnit[],
  selection: BoardScopeSelection,
  onOpenBoardSettings: ((boardId: string) => void) | undefined,
  onSelectScope: (selection: ResolvedBoardScope) => void,
): ReactNode[] {
  return sources.flatMap((source) => {
    const sourceId = source.source?.sourceId ?? "";
    const sourceUnits = units.filter((unit) => unit.sourceId === sourceId);
    if (sourceUnits.length === 0) {
      return [
        <section className="board-selector-source" key={`source:${sourceId}`}>
          <div className="board-selector-source-heading">
            <span>{sourceId || "Unknown source"}</span>
            <span>{sourceHealthLabel(source.health ?? SourceHealth.UNSPECIFIED)}</span>
          </div>
        </section>,
      ];
    }
    return [
      <section className="board-selector-source" key={`source:${sourceId}`}>
        <button
          type="button"
          className="board-selector-scope-option"
          aria-current={selection.kind === "all" && selection.sourceId === sourceId || undefined}
          onClick={() => onSelectScope({ kind: "all", sourceId })}
        >
          {sourceId || "Unknown source"}
        </button>
        {sourceUnits.map((unit) => (
          <section className="board-selector-project" key={`${sourceId}:${unit.project.id}`}>
            <button
              type="button"
              className="board-selector-scope-option board-selector-project-heading"
              aria-current={selection.kind === "all" && selection.projectId === unit.project.id || undefined}
              onClick={() => onSelectScope({ kind: "all", sourceId, projectId: unit.project.id })}
            >
              <span>{unit.project.name}</span>
              <span>{unit.totalBoardCount} {unit.totalBoardCount === 1 ? "board" : "boards"}</span>
            </button>
            {unit.boards.map((board) => (
              <BoardSelectorBoardRow
                key={board.id}
                board={board}
                projectName={unit.project.name}
                selectedBoardId={selection.kind === "board" ? selection.boardId : undefined}
                onOpenBoardSettings={onOpenBoardSettings}
                onSelectScope={onSelectScope}
              />
            ))}
          </section>
        ))}
      </section>,
    ];
  });
}

function SourceHeadings({ sources }: { sources: readonly AvailableSource[] }) {
  return (
    <div className="board-picker-sources">
      {sources.map((source) => (
        <span key={source.source?.sourceId} className="metadata-chip">
          {source.source?.sourceId ?? "Unknown source"}
        </span>
      ))}
    </div>
  );
}

function selectedScopeLabels(
  aggregate: boolean,
  projects: readonly AvailableProject[],
  boards: readonly AvailableBoard[],
  sources: readonly AvailableSource[],
  selection: BoardScopeSelection,
): { primary: string; secondary: string } {
  if (selection.kind === "all") {
    if (selection.projectId !== undefined) {
      const project = projects.find((candidate) =>
        candidate.id === selection.projectId &&
        (selection.sourceId === undefined ||
          candidate.source?.sourceId === selection.sourceId),
      );
      return {
        primary: project?.name ?? selection.projectId,
        secondary: selection.sourceId ?? "Project boards",
      };
    }
    if (selection.sourceId !== undefined) {
      return {
        primary: selection.sourceId,
        secondary: `${boards.filter((board) => board.source?.sourceId === selection.sourceId).length} boards`,
      };
    }
    return {
      primary: aggregate ? "All sources" : "All boards",
      secondary: catalogSummary(projects.length, boards.length),
    };
  }
  if (selection.kind === "unresolved") {
    return {
      primary: "Select a board",
      secondary: catalogSummary(projects.length, boards.length),
    };
  }
  if (selection.kind === "ambiguous") {
    return { primary: selection.boardId, secondary: "Board ID is ambiguous" };
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
      `${board.source?.sourceId === undefined ? "" : `${board.source.sourceId} / `}` +
	  (projects.find((project) =>
        project.id === board.projectId &&
        project.source?.sourceId === board.source?.sourceId,
      )?.name ??
      board.projectId) +
      (board.archived === undefined ? "" : " · Archived")
  };
}

function projectKey(sourceId: string | undefined, projectId: string): string {
  return `${sourceId ?? "local"}:${projectId}`;
}

function sourceHealthLabel(health: SourceHealth): string {
  switch (health) {
    case SourceHealth.HEALTHY:
      return "Healthy";
    case SourceHealth.DEGRADED:
      return "Degraded";
    case SourceHealth.UNAVAILABLE:
      return "Unavailable";
    default:
      return "Unknown";
  }
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

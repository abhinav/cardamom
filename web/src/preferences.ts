import {
  defaultBoardView,
  defaultListView,
  parseBoardView,
  parseListView,
  type BoardViewPreferences,
  type IssueViewPreferences,
} from "./issue-collection.ts";

export type ThemePreference = "light" | "dark";
export type RelationFocus = "dependencies" | "hierarchy" | "dependents";

export interface Preferences {
  actor: string;
  theme: ThemePreference;
  boardView: BoardViewPreferences;
  collapsedIssueDetailsBoardIds: string[];
  listView: IssueViewPreferences;
  relationFocus: RelationFocus;
  relationsOpen: boolean;
}

export interface PreferencesStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

export const defaultPreferences: Preferences = {
  actor: "",
  theme: "dark",
  boardView: defaultBoardView,
  collapsedIssueDetailsBoardIds: [],
  listView: defaultListView,
  relationFocus: "hierarchy",
  relationsOpen: true,
};

const storageKey = "cardamom.preferences";
const storageVersion = 6;

export function loadPreferences(storage: PreferencesStorage): Preferences {
  try {
    const persisted = storage.getItem(storageKey);
    if (persisted === null) {
      return defaultPreferences;
    }
    const value: unknown = JSON.parse(persisted);
    if (!isPersistedPreferences(value)) {
      return defaultPreferences;
    }
    const shellPreferences = {
      actor: value.actor,
      theme: value.theme,
    };
    if (value.version === 1) {
      return {
        ...shellPreferences,
        boardView: defaultBoardView,
        collapsedIssueDetailsBoardIds: [],
        listView: defaultListView,
        relationFocus: "hierarchy",
        relationsOpen: true,
      };
    }
    const boardView = parseBoardView(value.boardView);
    const listView = parseListView(value.listView);
    return {
      ...shellPreferences,
      boardView: { ...boardView, filters: defaultBoardView.filters },
      collapsedIssueDetailsBoardIds:
        value.version === 2
          ? []
          : parseCollapsedIssueDetails(
              value.collapsedIssueDetailsBoardIds,
            ),
      listView: { ...listView, filters: defaultListView.filters },
      relationFocus:
        value.version >= 6
          ? parseRelationFocus(value.relationFocus)
          : "hierarchy",
      relationsOpen:
        value.version >= 4 &&
        typeof value.relationsOpen === "boolean"
          ? value.relationsOpen
          : true,
    };
  } catch {
    return defaultPreferences;
  }
}

export function savePreferences(
  storage: PreferencesStorage,
  preferences: Preferences,
): void {
  try {
    storage.setItem(
      storageKey,
      JSON.stringify({
        version: storageVersion,
        actor: preferences.actor,
        theme: preferences.theme,
        boardView: {
          grouping: preferences.boardView.grouping,
          showEmptyColumns: preferences.boardView.showEmptyColumns,
          sort: preferences.boardView.sort,
          direction: preferences.boardView.direction,
        },
        collapsedIssueDetailsBoardIds:
          preferences.collapsedIssueDetailsBoardIds,
        listView: {
          sort: preferences.listView.sort,
          direction: preferences.listView.direction,
        },
        relationFocus: preferences.relationFocus,
        relationsOpen: preferences.relationsOpen,
      }),
    );
  } catch {
    // Preferences remain active for this session when storage is unavailable.
  }
}

/** setIssueDetailsCollapsed updates the shared Details disclosure for one board. */
export function setIssueDetailsCollapsed(
  preferences: Preferences,
  boardId: string,
  collapsed: boolean,
): Preferences {
  const collapsedBoards = new Set(
    preferences.collapsedIssueDetailsBoardIds,
  );
  if (collapsed) {
    collapsedBoards.add(boardId);
  } else {
    collapsedBoards.delete(boardId);
  }
  return {
    ...preferences,
    collapsedIssueDetailsBoardIds: [...collapsedBoards],
  };
}

function isPersistedPreferences(value: unknown): value is Preferences & {
  version: 1 | 2 | 3 | 4 | 5 | 6;
} {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return (
    (candidate.version === 1 ||
      candidate.version === 2 ||
      candidate.version === 3 ||
      candidate.version === 4 ||
      candidate.version === 5 ||
      candidate.version === storageVersion) &&
    typeof candidate.actor === "string" &&
    isTheme(candidate.theme)
  );
}

/** parseRelationFocus defaults absent or unsupported preferences to Hierarchy. */
export function parseRelationFocus(value: unknown): RelationFocus {
  return value === "dependencies" || value === "dependents"
    ? value
    : "hierarchy";
}

function parseCollapsedIssueDetails(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return [
    ...new Set(
      value.filter(
        (boardId): boardId is string =>
          typeof boardId === "string" && boardId !== "",
      ),
    ),
  ];
}

function isTheme(value: unknown): value is ThemePreference {
  return value === "light" || value === "dark";
}

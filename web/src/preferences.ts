import type { BoardScopePreference } from "./board-scope.ts";
import {
  defaultBoardView,
  defaultListView,
  parseBoardView,
  parseListView,
  type BoardViewPreferences,
  type IssueViewPreferences,
} from "./issue-collection.ts";

export type ThemePreference = "light" | "dark";

export interface Preferences {
  actor: string;
  theme: ThemePreference;
  boardScope?: BoardScopePreference;
  boardView: BoardViewPreferences;
  listView: IssueViewPreferences;
}

export interface PreferencesStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

export const defaultPreferences: Preferences = {
  actor: "",
  theme: "dark",
  boardView: defaultBoardView,
  listView: defaultListView,
};

const storageKey = "cardamom.preferences";
const storageVersion = 2;

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
      ...(value.boardScope === undefined ? {} : { boardScope: value.boardScope }),
    };
    if (value.version === 1) {
      return {
        ...shellPreferences,
        boardView: defaultBoardView,
        listView: defaultListView,
      };
    }
    return {
      ...shellPreferences,
      boardView: parseBoardView(value.boardView),
      listView: parseListView(value.listView),
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
      JSON.stringify({ version: storageVersion, ...preferences }),
    );
  } catch {
    // Preferences remain active for this session when storage is unavailable.
  }
}

function isPersistedPreferences(value: unknown): value is Preferences & {
  version: 1 | 2;
} {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return (
    (candidate.version === 1 || candidate.version === storageVersion) &&
    typeof candidate.actor === "string" &&
    isTheme(candidate.theme) &&
    (candidate.boardScope === undefined || isBoardScope(candidate.boardScope))
  );
}

function isTheme(value: unknown): value is ThemePreference {
  return value === "light" || value === "dark";
}

function isBoardScope(value: unknown): value is BoardScopePreference {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return (
    candidate.kind === "all" ||
    (candidate.kind === "board" &&
      typeof candidate.boardId === "string" &&
      candidate.boardId !== "")
  );
}

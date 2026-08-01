import { describe, expect, it } from "vitest";

import {
  defaultPreferences,
  loadPreferences,
  savePreferences,
  setIssueDetailsCollapsed,
  type PreferencesStorage,
} from "./preferences.ts";
import { defaultBoardView, defaultListView } from "./issue-collection.ts";

describe("preferences", () => {
  it("round trips presentation preferences without route identity or filters", () => {
    const storage = new MemoryStorage();
    const preferences = {
      actor: "captain",
      theme: "dark" as const,
      boardView: {
        ...defaultBoardView,
        grouping: "type" as const,
        showEmptyColumns: true,
        filters: { ...defaultBoardView.filters, label: "urgent" },
      },
      collapsedIssueDetailsBoardIds: ["board-1"],
      listView: {
        ...defaultListView,
        sort: "title" as const,
        direction: "ascending" as const,
      },
      relationsOpen: false,
    };

    savePreferences(storage, preferences);

    expect(loadPreferences(storage)).toEqual({
      ...preferences,
      boardView: {
        ...preferences.boardView,
        filters: defaultBoardView.filters,
      },
      listView: {
        ...preferences.listView,
        filters: defaultListView.filters,
      },
    });
    expect(storage.getItem("cardamom.preferences")).not.toBeNull();
    expect(storage.getItem("cardamom.preferences")).not.toContain("filters");
  });

  it("drops legacy board scope while preserving shell preferences", () => {
    const storage = new MemoryStorage(
      JSON.stringify({
        version: 1,
        actor: "captain",
        theme: "light",
        boardScope: { kind: "board", boardId: "board-1" },
        boardView: {
          ...defaultBoardView,
          filters: { ...defaultBoardView.filters, lifecycle: "open" },
          sort: "priority",
          direction: "ascending",
        },
        listView: {
          ...defaultListView,
          filters: { ...defaultListView.filters, actor: "stale-actor" },
          sort: "created",
          direction: "ascending",
        },
      }),
    );

    expect(loadPreferences(storage)).toEqual({
      actor: "captain",
      theme: "light",
      boardView: defaultBoardView,
      collapsedIssueDetailsBoardIds: [],
      listView: defaultListView,
      relationsOpen: true,
    });
  });

  it("expands Relations by default and remembers later choices", () => {
    const noPreference = new MemoryStorage();
    const previousVersion = new MemoryStorage(
      JSON.stringify({
        version: 3,
        actor: "captain",
        theme: "dark",
      }),
    );
    const currentVersion = new MemoryStorage();

    expect(loadPreferences(noPreference).relationsOpen).toBe(true);
    expect(loadPreferences(previousVersion).relationsOpen).toBe(true);

    savePreferences(currentVersion, {
      ...defaultPreferences,
      relationsOpen: false,
    });

    expect(loadPreferences(currentVersion).relationsOpen).toBe(false);

    savePreferences(currentVersion, {
      ...defaultPreferences,
      relationsOpen: true,
    });

    expect(loadPreferences(currentVersion).relationsOpen).toBe(true);
  });

  it("shares Details disclosure by board without changing other boards", () => {
    const boardOneCollapsed = setIssueDetailsCollapsed(
      defaultPreferences,
      "board-1",
      true,
    );
    const bothCollapsed = setIssueDetailsCollapsed(
      boardOneCollapsed,
      "board-2",
      true,
    );

    expect(bothCollapsed.collapsedIssueDetailsBoardIds).toEqual([
      "board-1",
      "board-2",
    ]);
    expect(
      setIssueDetailsCollapsed(bothCollapsed, "board-1", false)
        .collapsedIssueDetailsBoardIds,
    ).toEqual(["board-2"]);
  });

  it("recovers defaults from malformed or unsupported persisted data", () => {
    const malformed = new MemoryStorage("not json");
    const unsupported = new MemoryStorage(
      JSON.stringify({
        version: 99,
        actor: "stale",
        theme: "infrared",
      }),
    );

    expect(loadPreferences(malformed)).toEqual(defaultPreferences);
    expect(loadPreferences(unsupported)).toEqual(defaultPreferences);
  });

  it("does not write board identity supplied by a legacy caller", () => {
    const storage = new MemoryStorage();
    const legacyPreferences = {
      ...defaultPreferences,
      boardScope: { kind: "board" as const, boardId: "board-1" },
    };

    savePreferences(storage, legacyPreferences);

    expect(storage.getItem("cardamom.preferences")).not.toContain(
      "boardScope",
    );
  });

  it("defaults to dark and tolerates unavailable browser storage", () => {
    const unavailable: PreferencesStorage = {
      getItem() {
        throw new Error("storage disabled");
      },
      setItem() {
        throw new Error("storage disabled");
      },
    };

    expect(defaultPreferences.theme).toBe("dark");
    expect(loadPreferences(unavailable)).toEqual(defaultPreferences);
    expect(() => savePreferences(unavailable, defaultPreferences)).not.toThrow();
  });
});

class MemoryStorage implements PreferencesStorage {
  readonly #values = new Map<string, string>();

  constructor(initial?: string) {
    if (initial !== undefined) {
      this.#values.set("cardamom.preferences", initial);
    }
  }

  getItem(key: string): string | null {
    return this.#values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.#values.set(key, value);
  }
}

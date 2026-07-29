import { describe, expect, it } from "vitest";

import {
  defaultPreferences,
  loadPreferences,
  savePreferences,
  type PreferencesStorage,
} from "./preferences.ts";
import { defaultBoardView, defaultListView } from "./issue-collection.ts";

describe("preferences", () => {
  it("round trips actor, theme, and board scope through browser storage", () => {
    const storage = new MemoryStorage();
    const preferences = {
      actor: "captain",
      theme: "dark" as const,
      boardScope: { kind: "board" as const, boardId: "board-1" },
      boardView: {
        ...defaultBoardView,
        grouping: "type" as const,
        filters: { ...defaultBoardView.filters, label: "urgent" },
      },
      listView: {
        ...defaultListView,
        sort: "title" as const,
        direction: "ascending" as const,
      },
    };

    savePreferences(storage, preferences);

    expect(loadPreferences(storage)).toEqual(preferences);
    expect(storage.getItem("cardamom.preferences")).not.toBeNull();
  });

  it("resets version 1 collection views while preserving shell preferences", () => {
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
      boardScope: { kind: "board", boardId: "board-1" },
      boardView: defaultBoardView,
      listView: defaultListView,
    });
  });

  it("recovers defaults from malformed or unsupported persisted data", () => {
    const malformed = new MemoryStorage("not json");
    const unsupported = new MemoryStorage(
      JSON.stringify({
        version: 99,
        actor: "stale",
        theme: "infrared",
        boardScope: { kind: "board", boardId: "board-1" },
      }),
    );

    expect(loadPreferences(malformed)).toEqual(defaultPreferences);
    expect(loadPreferences(unsupported)).toEqual(defaultPreferences);
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

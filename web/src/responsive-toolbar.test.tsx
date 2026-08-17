// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SettingsControl } from "./app.tsx";
import { defaultBoardView } from "./issue-collection.ts";
import { IssueControls } from "./issue-views.tsx";
import { defaultPreferences } from "./preferences.ts";

afterEach(cleanup);

describe("responsive toolbar", () => {
  it("uses accessible compact collection controls", () => {
    render(
      <IssueControls
        mode="board"
        view={{
          ...defaultBoardView,
          filters: { ...defaultBoardView.filters, label: "area:web" },
        }}
        grouping="status"
        updateFilters={vi.fn()}
        updateGrouping={vi.fn()}
        updateView={vi.fn()}
        createIssue={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("Search issues")).toHaveAttribute(
      "title",
      "Search issues",
    );
    expect(screen.getByLabelText("Filters, 1 active")).toHaveAttribute(
      "title",
      "Filters",
    );
    expect(screen.getByLabelText("View options")).toHaveAttribute(
      "title",
      "View options",
    );
    expect(screen.getByLabelText("Create issue")).toHaveAttribute(
      "title",
      "Create issue",
    );
  });

  it("presents global settings as an accessible header icon and popover", () => {
    const updatePreferences = vi.fn();
    render(
      <SettingsControl
        preferences={{
          ...defaultPreferences,
          boardView: {
            ...defaultPreferences.boardView,
            showEmptyColumns: true,
          },
        }}
        openConfiguration={vi.fn()}
        selectedBoard={undefined}
        updatePreferences={updatePreferences}
        version="v1.2.3"
      />,
    );
    const trigger = screen.getByLabelText("Settings");

    expect(trigger).toHaveAttribute("title", "Settings");
    expect(screen.queryByText("Show empty columns")).not.toBeInTheDocument();

    fireEvent.click(trigger);

    expect(screen.getByRole("checkbox", { name: "Show empty columns" }))
      .toBeChecked();
    expect(screen.getByText("Cardamom version v1.2.3")).toBeInTheDocument();
    expect(screen.queryByText("All boards")).not.toBeInTheDocument();
    expect(screen.queryByText("Board settings")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Switch to light theme" }));
    expect(updatePreferences).toHaveBeenCalledWith(expect.objectContaining({
      theme: "light",
    }));
  });
});

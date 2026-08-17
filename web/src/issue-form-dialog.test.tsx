// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { IssueFormDialog } from "./issue-form-dialog.tsx";

afterEach(cleanup);

describe("issue form dialog", () => {
  it("keeps the heading and actions outside the scrollable form body", () => {
    render(
      <IssueFormDialog
        actions={<button type="submit">Save issue</button>}
        description="cm-task"
        onDismiss={vi.fn()}
        title="Edit issue"
        titleId="edit-issue-title"
        onSubmit={vi.fn()}
      >
        <label>
          Title
          <input name="title" />
        </label>
      </IssueFormDialog>,
    );

    const dialog = screen.getByRole("dialog", { name: "Edit issue" });
    const form = dialog.querySelector("form");
    const body = dialog.querySelector(".issue-form-dialog-body");
    const actions = dialog.querySelector(".issue-form-dialog-actions");

    expect(form).not.toBeNull();
    expect(body?.parentElement).toBe(form);
    expect(actions?.parentElement).toBe(form);
    expect(body?.compareDocumentPosition(actions!)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(screen.getByRole("button", { name: "Save issue" })).toBeInTheDocument();
  });

  it("dismisses from the keyboard", () => {
    const onDismiss = vi.fn();
    render(
      <IssueFormDialog
        actions={<button type="submit">Save issue</button>}
        description="cm-task"
        onDismiss={onDismiss}
        title="Edit issue"
        titleId="edit-issue-title"
        onSubmit={vi.fn()}
      >
        <label>
          Title
          <input name="title" />
        </label>
      </IssueFormDialog>,
    );

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });

    expect(onDismiss).toHaveBeenCalledOnce();
  });
});

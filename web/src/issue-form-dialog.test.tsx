import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { IssueFormDialog } from "./issue-form-dialog.tsx";

describe("issue form dialog", () => {
  it("keeps the heading and actions outside the scrollable form body", () => {
    const markup = renderToStaticMarkup(
      <IssueFormDialog
        actions={<button type="submit">Save issue</button>}
        description="cm-task"
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

    expect(markup).toContain('role="dialog"');
    expect(markup).toContain('aria-labelledby="edit-issue-title"');
    expect(markup).toContain('class="modal-header issue-form-dialog-header"');
    expect(markup).toContain(
      'class="workflow-form issue-form-dialog-body"',
    );
    expect(markup).toContain(
      'class="modal-actions issue-form-dialog-actions"',
    );

    const formStart = markup.indexOf("<form");
    const bodyStart = markup.indexOf("issue-form-dialog-body");
    const actionsStart = markup.indexOf("issue-form-dialog-actions");
    const formEnd = markup.indexOf("</form>");
    expect(formStart).toBeGreaterThan(-1);
    expect(bodyStart).toBeGreaterThan(formStart);
    expect(actionsStart).toBeGreaterThan(bodyStart);
    expect(formEnd).toBeGreaterThan(actionsStart);
  });
});

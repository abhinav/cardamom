import type { FormEventHandler, ReactNode } from "react";

import "./issue-form-dialog.css";

interface IssueFormDialogProps {
  actions: ReactNode;
  children: ReactNode;
  description: ReactNode;
  onSubmit: FormEventHandler<HTMLFormElement>;
  title: string;
  titleId: string;
}

/** IssueFormDialog keeps form actions reachable while its field region scrolls. */
export function IssueFormDialog({
  actions,
  children,
  description,
  onSubmit,
  title,
  titleId,
}: IssueFormDialogProps) {
  return (
    <div className="modal-backdrop">
      <section
        className="modal-panel issue-form-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
      >
        <header className="modal-header issue-form-dialog-header">
          <div>
            <h2 id={titleId}>{title}</h2>
            <p>{description}</p>
          </div>
        </header>
        <form className="issue-form-dialog-form" onSubmit={onSubmit}>
          <div className="workflow-form issue-form-dialog-body">
            {children}
          </div>
          <div className="modal-actions issue-form-dialog-actions">
            {actions}
          </div>
        </form>
      </section>
    </div>
  );
}

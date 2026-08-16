import type { FormEventHandler, ReactNode } from "react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

import "./issue-form-dialog.css";

interface IssueFormDialogProps {
  actions: ReactNode;
  children: ReactNode;
  description: ReactNode;
  onDismiss: () => void;
  onSubmit: FormEventHandler<HTMLFormElement>;
  title: string;
  titleId: string;
}

/** IssueFormDialog keeps form actions reachable while its field region scrolls. */
export function IssueFormDialog({
  actions,
  children,
  description,
  onDismiss,
  onSubmit,
  title,
  titleId,
}: IssueFormDialogProps) {
  return (
    <Dialog open onOpenChange={(open) => !open && onDismiss()}>
      <DialogContent
        className="issue-form-dialog max-h-[calc(100dvh-2rem)] max-w-2xl gap-0 overflow-hidden p-0"
        showCloseButton={false}
      >
        <DialogHeader className="issue-form-dialog-header p-5">
          <DialogTitle id={titleId}>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <form className="issue-form-dialog-form" onSubmit={onSubmit}>
          <div className="workflow-form issue-form-dialog-body">
            {children}
          </div>
          <div className="modal-actions issue-form-dialog-actions">
            {actions}
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

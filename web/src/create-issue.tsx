import { useMutation } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { X } from "lucide-react";
import type { FormEvent } from "react";
import { useState } from "react";

import type { AttachmentClient } from "./api.ts";
import {
  uploadAttachment,
  type AttachmentUploadFile,
  type AttachmentUploadProgress,
} from "./attachments.tsx";
import type { BoardScopeSelection } from "./board-scope.ts";
import { WatchResource } from "./gen/cardamom/private/v1/change_pb.ts";
import { IssueType } from "./gen/cardamom/private/v1/issue_pb.ts";
import { IssueFormDialog } from "./issue-form-dialog.tsx";
import { PlanningService } from "./gen/cardamom/private/v1/planning_pb.ts";
import {
  IssueMetadataFields,
  metadataLabels,
  type IssueMetadataDraft,
} from "./issue-metadata-form.tsx";
import { runInvalidatingMutation } from "./query-runtime.ts";

// IssueCreationDraft retains the user-facing form representation until the
// generated Connect request boundary normalizes each field.
export interface IssueCreationDraft extends IssueMetadataDraft {
  // Prerequisites are comma-, whitespace-, or newline-separated issue IDs.
  prerequisites: string;
}

/** issueCreationBoardId returns the writable board selected for issue creation. */
export function issueCreationBoardId(
  selection: BoardScopeSelection,
): string | undefined {
  return selection.kind === "board" ? selection.boardId : undefined;
}

/** issueCreationInput normalizes the user-facing draft at the RPC boundary. */
export function issueCreationInput(
  boardId: string,
  actor: string,
  draft: IssueCreationDraft,
) {
  return {
    boardId,
    title: draft.title.trim(),
    type: draft.type,
    priority: draft.priority,
    summarySource: optionalMarkdown(draft.summary),
    detailsSource: optionalMarkdown(draft.details),
    labels: metadataLabels(draft.labels),
    parentId: optionalValue(draft.parent),
    prerequisiteIds: separatedValues(draft.prerequisites, /[,\s]/),
    context: { actor: actor.trim() },
  };
}

interface CreateIssueDialogProps {
  actor: string;
  attachmentClient: AttachmentClient;
  boardId: string;
  onCreated: (issueId: string) => void;
  onDismiss: () => void;
}

/**
 * createIssueWithAttachments creates once before uploading staged files in order.
 * Creation failures return no issue ID; upload failures retain it and stop the queue.
 */
export async function createIssueWithAttachments(
  createIssue: () => Promise<string>,
  files: readonly AttachmentUploadFile[],
  upload: (
    issueId: string,
    file: AttachmentUploadFile,
    reportProgress: (progress: AttachmentUploadProgress) => void,
  ) => Promise<void>,
  reportProgress: (
    progress: AttachmentUploadProgress & {
      issueId: string;
      filename: string;
      fileIndex: number;
      totalCount: number;
    },
  ) => void,
) {
  let issueId: string;
  try {
    issueId = await createIssue();
  } catch (failure) {
    return {
      kind: "creation-error",
      message: failureMessage(failure),
    } as const;
  }

  for (const [fileIndex, file] of files.entries()) {
    const fileProgress = (progress: AttachmentUploadProgress) =>
      reportProgress({
        ...progress,
        issueId,
        filename: file.name,
        fileIndex,
        totalCount: files.length,
      });
    fileProgress({ sentBytes: 0, totalBytes: file.size });
    try {
      await upload(issueId, file, fileProgress);
    } catch (failure) {
      return {
        kind: "upload-error",
        issueId,
        filename: file.name,
        uploadedCount: fileIndex,
        totalCount: files.length,
        message: failureMessage(failure),
      } as const;
    }
  }

  return { kind: "success", issueId } as const;
}

export function CreateIssueDialog({
  actor,
  attachmentClient,
  boardId,
  onCreated,
  onDismiss,
}: CreateIssueDialogProps) {
  const queryClient = useQueryClient();
  const createIssue = useMutation(PlanningService.method.createIssue);
  const [draft, setDraft] = useState<IssueCreationDraft>(emptyDraft);
  const [files, setFiles] = useState<File[]>([]);
  const [fileInputKey, setFileInputKey] = useState(0);
  const [submission, setSubmission] = useState<SubmissionState>({ kind: "idle" });
  const submitting =
    submission.kind === "creating" || submission.kind === "uploading";
  const issueCreated = submission.kind === "upload-error";
  const fieldsDisabled = submitting || issueCreated;

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitting || issueCreated) {
      return;
    }
    setSubmission({ kind: "creating" });
    const result = await createIssueWithAttachments(
      async () => {
        const response = await runInvalidatingMutation(
          queryClient,
          [WatchResource.ISSUES, WatchResource.APPROVALS],
          () => createIssue.mutateAsync(issueCreationInput(boardId, actor, draft)),
        );
        if (response.issue === undefined) {
          throw new Error("CreateIssue response did not include the created issue");
        }
        return response.issue.id;
      },
      files,
      (issueId, file, reportProgress) =>
        runInvalidatingMutation(queryClient, [WatchResource.ISSUES], async () => {
          await uploadAttachment(
            attachmentClient,
            boardId,
            issueId,
            actor,
            file,
            reportProgress,
          );
        }),
      ({ issueId, filename, fileIndex, totalCount, sentBytes, totalBytes }) =>
        setSubmission({
          kind: "uploading",
          issueId,
          filename,
          fileIndex,
          totalCount,
          sentBytes,
          totalBytes,
        }),
    );
    switch (result.kind) {
      case "success":
        onCreated(result.issueId);
        return;
      case "creation-error":
        setSubmission(result);
        return;
      case "upload-error":
        setSubmission(result);
    }
  };

  return (
    <IssueFormDialog
      actions={
        <>
          <button
            type="button"
            className="secondary-button"
            disabled={submitting}
            onClick={onDismiss}
          >
            {issueCreated ? "Close" : "Cancel"}
          </button>
          {submission.kind === "upload-error" ? (
            <button type="button" onClick={() => onCreated(submission.issueId)}>
              Open {submission.issueId}
            </button>
          ) : (
            <button type="submit" disabled={submitting}>
              {submission.kind === "creating"
                ? "Creating"
                : submission.kind === "uploading"
                  ? "Uploading"
                  : "Create issue"}
            </button>
          )}
        </>
      }
      description={`Board ${boardId}`}
      title="Create issue"
      titleId="create-issue-title"
      onSubmit={submit}
    >
      <IssueMetadataFields
        autoFocusTitle
        disabled={fieldsDisabled}
        draft={draft}
        onChange={(field, value) =>
          setDraft((current) => ({ ...current, [field]: value }))
        }
      />
      <label className="form-field">
        <span>Prerequisites</span>
        <input
          type="text"
          disabled={fieldsDisabled}
          placeholder="Issue IDs"
          value={draft.prerequisites}
          onInput={(event) =>
            setDraft({ ...draft, prerequisites: event.currentTarget.value })
          }
        />
      </label>
      <label className="form-field form-field-wide">
        <span>Attachments</span>
        <input
          key={fileInputKey}
          type="file"
          multiple
          disabled={fieldsDisabled}
          onChange={(event) => {
            const selected = Array.from(event.currentTarget.files ?? []);
            setFiles((current) => [...current, ...selected]);
            setFileInputKey((key) => key + 1);
            setSubmission({ kind: "idle" });
          }}
        />
      </label>
      {files.length > 0 && !fieldsDisabled && (
        <div className="create-attachment-stage form-field-wide">
          <p>{files.length} {files.length === 1 ? "file" : "files"} staged</p>
          <ul aria-label="Staged attachments">
            {files.map((file, index) => (
              <li key={`${file.name}-${file.size}-${file.lastModified}-${index}`}>
                <span>{file.name}</span>
                <button
                  type="button"
                  className="create-attachment-remove secondary-button"
                  aria-label={`Remove ${file.name}`}
                  title={`Remove ${file.name}`}
                  disabled={fieldsDisabled}
                  onClick={() =>
                    setFiles((current) =>
                      current.filter((_, item) => item !== index),
                    )
                  }
                >
                  <X aria-hidden="true" size={15} strokeWidth={2} />
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
      {submission.kind === "creation-error" && (
        <p className="form-error form-field-wide" role="alert">
          {submission.message}
        </p>
      )}
      {submission.kind === "creating" && (
        <p className="create-issue-status form-field-wide" role="status">
          Creating issue...
        </p>
      )}
      {submission.kind === "uploading" && (
        <div className="create-issue-progress form-field-wide" role="status">
          <span>
            Issue {submission.issueId} created. Uploading{" "}
            {submission.filename} ({submission.fileIndex + 1} of{" "}
            {submission.totalCount}).
          </span>
          <progress
            value={submission.sentBytes}
            max={Math.max(submission.totalBytes, 1)}
          />
          <span>{submission.sentBytes} of {submission.totalBytes} bytes</span>
        </div>
      )}
      {submission.kind === "upload-error" && (
        <div className="create-issue-recovery form-field-wide" role="alert">
          <p>
            Issue <strong>{submission.issueId}</strong> was created. Upload
            failed for <strong>{submission.filename}</strong> after{" "}
            {submission.uploadedCount} of {submission.totalCount} files.
          </p>
          <p>{submission.message}</p>
          {submission.uploadedCount > 0 && (
            <p>Earlier uploads remain attached.</p>
          )}
        </div>
      )}
    </IssueFormDialog>
  );
}

type SubmissionState =
  | { kind: "idle" }
  | { kind: "creating" }
  | {
      kind: "uploading";
      issueId: string;
      filename: string;
      fileIndex: number;
      totalCount: number;
      sentBytes: number;
      totalBytes: number;
    }
  | { kind: "creation-error"; message: string }
  | {
      kind: "upload-error";
      issueId: string;
      filename: string;
      uploadedCount: number;
      totalCount: number;
      message: string;
    };

const emptyDraft: IssueCreationDraft = {
  title: "",
  type: IssueType.TASK,
  priority: 1,
  summary: "",
  details: "",
  labels: "",
  parent: "",
  prerequisites: "",
};

function separatedValues(value: string, separator: RegExp): string[] {
  return [...new Set(value.split(separator).map((part) => part.trim()).filter(Boolean))];
}

function optionalValue(value: string): string | undefined {
  const normalized = value.trim();
  return normalized === "" ? undefined : normalized;
}

function optionalMarkdown(value: string): string | undefined {
  return value === "" ? undefined : value;
}

function failureMessage(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure);
}

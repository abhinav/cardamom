import { useInfiniteQuery } from "@connectrpc/connect-query";
import {
  keepPreviousData,
  useQueryClient,
} from "@tanstack/react-query";
import type { FormEvent } from "react";
import { useEffect, useState } from "react";

import type { AttachmentClient } from "./api.ts";
import { AttachmentIdentity } from "./attachment-identity.tsx";
import { attachmentPath } from "./board-scope.ts";
import {
  AttachmentLifecycle,
  AttachmentService,
  BlobAvailability,
  type Attachment,
  type ListAttachmentsResponse,
} from "./gen/cardamom/private/v1/attachment_pb.ts";
import { WatchResource } from "./gen/cardamom/private/v1/change_pb.ts";
import { runInvalidatingMutation } from "./query-runtime.ts";

import "./attachments.css";

const attachmentPageSize = 100;

/** AttachmentUploadFile is the browser file surface needed by the upload operation. */
export interface AttachmentUploadFile {
  /** name is the portable presentation filename sent to the server. */
  name: string;

  /** size is the complete browser-observed byte count. */
  size: number;

  /** slice reads one server-bounded byte range. */
  slice(start?: number, end?: number): Blob;
}

/** AttachmentUploadProgress reports committed chunk progress for one file. */
export interface AttachmentUploadProgress {
  /** sentBytes is the number of bytes accepted by chunk RPCs. */
  sentBytes: number;

  /** totalBytes is the complete browser-observed file size. */
  totalBytes: number;
}

interface AttachmentContent {
  available: boolean;
  href?: string;
  label: string;
}

/** attachmentListInput selects the first stable metadata page for one owner. */
export function attachmentListInput(
  boardId: string,
  issueId: string | undefined,
) {
  return {
    boardId,
    ...(issueId === undefined ? {} : { issueId }),
    pageSize: attachmentPageSize,
    pageToken: "",
  };
}

/** nextAttachmentPageToken continues only while the server returns a token. */
export function nextAttachmentPageToken(
  page: ListAttachmentsResponse,
): string | undefined {
  return page.nextPageToken || undefined;
}

/** attachmentsFromPages preserves server page order in the visible list. */
export function attachmentsFromPages(
  pages: readonly ListAttachmentsResponse[],
): Attachment[] {
  return pages.flatMap((page) => page.attachments);
}

/** uploadAttachment publishes one browser file through the resumable RPC lifecycle. */
export async function uploadAttachment(
  client: Pick<
    AttachmentClient,
    | "beginAttachmentUpload"
    | "writeAttachmentChunk"
    | "commitAttachmentUpload"
    | "abortAttachmentUpload"
  >,
  boardId: string,
  issueId: string | undefined,
  actor: string,
  file: AttachmentUploadFile,
  reportProgress: (progress: AttachmentUploadProgress) => void,
): Promise<Attachment> {
  const mutation = { actor: actor.trim() };
  const begin = await client.beginAttachmentUpload({
    boardId,
    ...(issueId === undefined ? {} : { issueId }),
    filename: file.name,
    expectedSizeBytes: BigInt(file.size),
    mutation,
  });
  const uploadId = begin.upload?.id;
  if (uploadId === undefined || uploadId === "") {
    throw new Error("BeginAttachmentUpload response did not include an upload");
  }

  try {
    const maxAttachmentSize = safeByteLimit(
      begin.maxAttachmentSizeBytes,
      "attachment",
    );
    if (file.size > maxAttachmentSize) {
      throw new Error(
        `${file.name} exceeds the ${formatBytes(BigInt(maxAttachmentSize))} attachment limit`,
      );
    }
    const chunkSize = safeByteLimit(begin.maxChunkSizeBytes, "chunk");
    reportProgress({ sentBytes: 0, totalBytes: file.size });

    for (let offset = 0; offset < file.size; offset += chunkSize) {
      const end = Math.min(offset + chunkSize, file.size);
      const content = new Uint8Array(await file.slice(offset, end).arrayBuffer());
      await client.writeAttachmentChunk({
        uploadId,
        expectedOffset: BigInt(offset),
        content,
        mutation,
      });
      reportProgress({ sentBytes: end, totalBytes: file.size });
    }

    const committed = await client.commitAttachmentUpload({
      uploadId,
      mutation,
    });
    if (committed.attachment === undefined) {
      throw new Error(
        "CommitAttachmentUpload response did not include an attachment",
      );
    }
    return committed.attachment;
  } catch (failure) {
    try {
      await client.abortAttachmentUpload({ uploadId, mutation });
    } catch {
      // Preserve the upload failure; an unreachable abort is cleaned up by expiry.
    }
    throw failure;
  }
}

/** attachmentContent maps lifecycle and local availability to one download control. */
export function attachmentContent(
  attachment: Attachment,
  boardId: string,
): AttachmentContent {
  if (attachment.lifecycle === AttachmentLifecycle.REMOVED) {
    return { available: false, label: "Removed" };
  }
  switch (attachment.availability) {
    case BlobAvailability.PRESENT_UNVERIFIED:
    case BlobAvailability.VERIFIED:
      return {
        available: true,
        href: attachmentPath(boardId, attachment.id),
        label: "Download",
      };
    case BlobAvailability.MISSING:
      return { available: false, label: "Missing locally" };
    case BlobAvailability.SIZE_MISMATCH:
      return { available: false, label: "Size mismatch" };
    case BlobAvailability.DIGEST_MISMATCH:
      return { available: false, label: "Digest mismatch" };
    default:
      return { available: false, label: "Unavailable" };
  }
}

interface AttachmentPanelProps {
  actor: string;
  boardId: string;
  client: AttachmentClient;
  issueId?: string;
}

/** AttachmentPanel composes attachment reading and one-at-a-time browser uploads. */
export function AttachmentPanel({
  actor,
  boardId,
  client,
  issueId,
}: AttachmentPanelProps) {
  return (
    <>
      <AttachmentRecords boardId={boardId} issueId={issueId} />
      <AttachmentUploadPanel
        actor={actor}
        boardId={boardId}
        client={client}
        issueId={issueId}
      />
    </>
  );
}

interface AttachmentRecordsProps {
  boardId: string;
  issueId?: string;
}

/** AttachmentRecords renders one owner's stable records when any exist. */
export function AttachmentRecords({
  boardId,
  issueId,
}: AttachmentRecordsProps) {
  const request = useInfiniteQuery(
    AttachmentService.method.listAttachments,
    attachmentListInput(boardId, issueId),
    {
      pageParamKey: "pageToken",
      getNextPageParam: nextAttachmentPageToken,
      retry: false,
      refetchOnReconnect: false,
      refetchOnWindowFocus: false,
      placeholderData: keepPreviousData,
    },
  );
  const {
    fetchNextPage,
    hasNextPage,
    isError,
    isFetchingNextPage,
  } = request;
  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage && !isError) {
      void fetchNextPage();
    }
  }, [fetchNextPage, hasNextPage, isError, isFetchingNextPage]);
  const attachments =
    request.data === undefined || request.hasNextPage || request.isFetchingNextPage
      ? undefined
      : attachmentsFromPages(request.data.pages);
  if (attachments?.length === 0) {
    return null;
  }

  return (
    <section className="attachment-panel" aria-labelledby={`attachments-${issueId ?? boardId}`}>
      <div className="attachment-heading">
        <h2 id={`attachments-${issueId ?? boardId}`}>Attachments</h2>
        <span>{attachments?.length ?? 0}</span>
      </div>
      {request.isFetching && attachments === undefined && !request.isError && (
        <p className="attachment-state" role="status">Loading attachments...</p>
      )}
      {request.isError && attachments === undefined && (
        <div className="attachment-state" role="alert">
          <span>{request.error.message}</span>
          <button type="button" onClick={() => void request.refetch()}>
            Retry
          </button>
        </div>
      )}
      {attachments !== undefined && (
        <AttachmentList
          attachments={attachments}
          boardId={boardId}
          showIssue={issueId === undefined}
        />
      )}
    </section>
  );
}

/** AttachmentUploadPanel owns one-at-a-time browser uploads for one owner. */
export function AttachmentUploadPanel({
  actor,
  boardId,
  client,
  issueId,
}: AttachmentPanelProps) {
  const queryClient = useQueryClient();
  const [file, setFile] = useState<File>();
  const [fileInputKey, setFileInputKey] = useState(0);
  const [upload, setUpload] = useState<AttachmentUploadState>({ kind: "idle" });
  const actorMissing = actor.trim() === "";
  const uploading = upload.kind === "uploading";

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (file === undefined || uploading || actorMissing) {
      return;
    }
    setUpload({ kind: "uploading", sentBytes: 0, totalBytes: file.size });
    try {
      const attachment = await runInvalidatingMutation(
        queryClient,
        [WatchResource.ISSUES],
        () =>
          uploadAttachment(
            client,
            boardId,
            issueId,
            actor,
            file,
            ({ sentBytes, totalBytes }) =>
              setUpload({ kind: "uploading", sentBytes, totalBytes }),
          ),
      );
      setFile(undefined);
      setFileInputKey((key) => key + 1);
      setUpload({ kind: "success", filename: attachment.filename });
    } catch (failure) {
      setUpload({ kind: "error", message: failureMessage(failure) });
    }
  };

  return (
    <section className="attachment-upload-panel" aria-label="Attachment controls">
      <form className="attachment-upload" onSubmit={(event) => void submit(event)}>
        <label>
          <span>Add a file</span>
          <input
            key={fileInputKey}
            type="file"
            disabled={uploading}
            onChange={(event) => {
              setFile(event.currentTarget.files?.[0]);
              setUpload({ kind: "idle" });
            }}
          />
        </label>
        <button type="submit" disabled={file === undefined || uploading || actorMissing}>
          {uploading ? "Uploading" : "Upload"}
        </button>
      </form>
      {actorMissing && (
        <p className="attachment-note">Set an actor in Settings to upload.</p>
      )}
      <AttachmentUploadStatus state={upload} />
    </section>
  );
}

function AttachmentList({
  attachments,
  boardId,
  showIssue,
}: {
  attachments: readonly Attachment[];
  boardId: string;
  showIssue: boolean;
}) {
  if (attachments.length === 0) {
    return <p className="attachment-empty">No attachments.</p>;
  }
  return (
    <ul className="attachment-list">
      {attachments.map((attachment) => {
        const content = attachmentContent(attachment, boardId);
        return (
          <li key={attachment.id}>
            <AttachmentIdentity attachment={attachment} />
            <div className="attachment-metadata">
              <span>{attachment.mediaType || "Unknown media type"}</span>
              <span>{formatBytes(attachment.blob?.sizeBytes ?? 0n)}</span>
              {showIssue && attachment.issueId !== undefined && (
                <span>{attachment.issueId}</span>
              )}
              <span>{formatTimestamp(attachment.created?.at)}</span>
            </div>
            {content.available && content.href !== undefined ? (
              <a className="attachment-download" href={content.href} download={attachment.filename}>
                {content.label}
              </a>
            ) : (
              <span className="attachment-unavailable">{content.label}</span>
            )}
          </li>
        );
      })}
    </ul>
  );
}

type AttachmentUploadState =
  | { kind: "idle" }
  | { kind: "uploading"; sentBytes: number; totalBytes: number }
  | { kind: "success"; filename: string }
  | { kind: "error"; message: string };

function AttachmentUploadStatus({ state }: { state: AttachmentUploadState }) {
  if (state.kind === "idle") {
    return null;
  }
  if (state.kind === "uploading") {
    return (
      <div className="attachment-progress" role="status">
        <progress value={state.sentBytes} max={Math.max(state.totalBytes, 1)} />
        <span>
          {formatBytes(BigInt(state.sentBytes))} of {formatBytes(BigInt(state.totalBytes))}
        </span>
      </div>
    );
  }
  return (
    <p className="attachment-note" data-status={state.kind} role={state.kind === "error" ? "alert" : "status"}>
      {state.kind === "success" ? `${state.filename} uploaded.` : state.message}
    </p>
  );
}

function safeByteLimit(value: bigint, kind: string): number {
  if (value <= 0n || value > BigInt(Number.MAX_SAFE_INTEGER)) {
    throw new Error(`Server returned an invalid ${kind} byte limit`);
  }
  return Number(value);
}

function formatBytes(bytes: bigint): string {
  const units = ["B", "KB", "MB", "GB"];
  let value = Number(bytes);
  let unit = 0;
  while (value >= 1_024 && unit < units.length - 1) {
    value /= 1_024;
    unit++;
  }
  const digits = unit === 0 || value >= 10 ? 0 : 1;
  return `${value.toFixed(digits)} ${units[unit]}`;
}

function formatTimestamp(
  timestamp: { seconds: bigint; nanos: number } | undefined,
): string {
  if (timestamp === undefined) {
    return "Time unavailable";
  }
  const date = new Date(Number(timestamp.seconds) * 1_000 + timestamp.nanos / 1e6);
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium" }).format(date);
}

function failureMessage(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure);
}

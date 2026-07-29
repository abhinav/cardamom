import { create } from "@bufbuild/protobuf";
import { createRouterTransport } from "@connectrpc/connect";
import {
  createInfiniteQueryOptions,
  TransportProvider,
} from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import type { AttachmentClient } from "./api.ts";
import {
  AttachmentPanel,
  attachmentListInput,
  attachmentContent,
  attachmentsFromPages,
  nextAttachmentPageToken,
  uploadAttachment,
  type AttachmentUploadFile,
} from "./attachments.tsx";
import {
  AttachmentLifecycle,
  AttachmentService,
  BlobAvailability,
  ListAttachmentsResponseSchema,
  type Attachment,
} from "./gen/cardamom/private/v1/attachment_pb.ts";

describe("attachment web workflow", () => {
  it("presents each stable attachment identity beside its filename", async () => {
    const listedAttachment = attachment("incident-log.txt");
    const transport = createRouterTransport(({ service }) => {
      service(AttachmentService, {
        listAttachments: () => ({ attachments: [listedAttachment] }),
      });
    });
    const queryClient = new QueryClient();
    await queryClient.fetchInfiniteQuery(
      createInfiniteQueryOptions(
        AttachmentService.method.listAttachments,
        attachmentListInput("board-1", "cm-issue"),
        {
          transport,
          pageParamKey: "pageToken",
          getNextPageParam: nextAttachmentPageToken,
        },
      ),
    );

    const markup = renderToStaticMarkup(
      createElement(
        TransportProvider,
        { transport },
        createElement(
          QueryClientProvider,
          { client: queryClient },
          createElement(AttachmentPanel, {
            actor: "browser-actor",
            boardId: "board-1",
            client: {} as AttachmentClient,
            issueId: "cm-issue",
          }),
        ),
      ),
    );

    expect(markup).toContain("incident-log.txt");
    expect(markup).toContain("attachment-id");
    expect(markup).toContain(
      'aria-label="Copy attachment ID attachment-id"',
    );
    expect(markup).not.toContain("attachment:");
  });

  it("omits an empty Attachments section without removing upload controls", async () => {
    const transport = createRouterTransport(({ service }) => {
      service(AttachmentService, {
        listAttachments: () => ({ attachments: [] }),
      });
    });
    const queryClient = new QueryClient();
    await queryClient.fetchInfiniteQuery(
      createInfiniteQueryOptions(
        AttachmentService.method.listAttachments,
        attachmentListInput("board-1", "cm-empty"),
        {
          transport,
          pageParamKey: "pageToken",
          getNextPageParam: nextAttachmentPageToken,
        },
      ),
    );

    const markup = renderToStaticMarkup(
      createElement(
        TransportProvider,
        { transport },
        createElement(
          QueryClientProvider,
          { client: queryClient },
          createElement(AttachmentPanel, {
            actor: "browser-actor",
            boardId: "board-1",
            client: {} as AttachmentClient,
            issueId: "cm-empty",
          }),
        ),
      ),
    );

    expect(markup).not.toContain(">Attachments</h2>");
    expect(markup).not.toContain("No attachments.");
    expect(markup).toContain('aria-label="Attachment controls"');
    expect(markup).toContain("Add a file");
  });

  it("selects and combines every issue-associated metadata page", () => {
    const first = attachment("one.txt");
    const second = attachment("two.txt");
    const pages = [
      create(ListAttachmentsResponseSchema, {
        attachments: [first],
        nextPageToken: "next-page",
      }),
      create(ListAttachmentsResponseSchema, {
        attachments: [second],
        nextPageToken: "",
      }),
    ];

    expect(attachmentListInput("board-1", "cm-issue")).toEqual({
      boardId: "board-1",
      issueId: "cm-issue",
      pageSize: 100,
      pageToken: "",
    });
    expect(nextAttachmentPageToken(pages[0]!)).toBe("next-page");
    expect(nextAttachmentPageToken(pages[1]!)).toBeUndefined();
    expect(attachmentsFromPages(pages)).toEqual([first, second]);
  });

  it("uploads bytes in server-bounded chunks and commits the session", async () => {
    const beginAttachmentUploadRPC = vi.fn(async () => ({
      upload: { id: "upload-1" },
      maxChunkSizeBytes: 4n,
      maxAttachmentSizeBytes: 20n,
    }));
    const writeAttachmentChunkRPC = vi.fn(async () => ({
      upload: { id: "upload-1" },
    }));
    const committed = attachment("readme.txt");
    const commitAttachmentUploadRPC = vi.fn(async () => ({
      attachment: committed,
    }));
    const client = {
      beginAttachmentUpload: beginAttachmentUploadRPC,
      writeAttachmentChunk: writeAttachmentChunkRPC,
      commitAttachmentUpload: commitAttachmentUploadRPC,
      abortAttachmentUpload: vi.fn(),
    } as unknown as AttachmentClient;
    const progress = vi.fn();

    const result = await uploadAttachment(
      client,
      "board-1",
      "cm-issue",
      "  browser-actor  ",
      uploadFile("readme.txt", "abcdef"),
      progress,
    );

    expect(result).toBe(committed);
    expect(beginAttachmentUploadRPC).toHaveBeenCalledWith({
      boardId: "board-1",
      issueId: "cm-issue",
      filename: "readme.txt",
      expectedSizeBytes: 6n,
      mutation: { actor: "browser-actor" },
    });
    expect(writeAttachmentChunkRPC).toHaveBeenNthCalledWith(1, {
      uploadId: "upload-1",
      expectedOffset: 0n,
      content: new Uint8Array([97, 98, 99, 100]),
      mutation: { actor: "browser-actor" },
    });
    expect(writeAttachmentChunkRPC).toHaveBeenNthCalledWith(2, {
      uploadId: "upload-1",
      expectedOffset: 4n,
      content: new Uint8Array([101, 102]),
      mutation: { actor: "browser-actor" },
    });
    expect(progress.mock.calls).toEqual([
      [{ sentBytes: 0, totalBytes: 6 }],
      [{ sentBytes: 4, totalBytes: 6 }],
      [{ sentBytes: 6, totalBytes: 6 }],
    ]);
    expect(commitAttachmentUploadRPC).toHaveBeenCalledWith({
      uploadId: "upload-1",
      mutation: { actor: "browser-actor" },
    });
  });

  it("aborts a started upload when a chunk fails", async () => {
    const client = {
      beginAttachmentUpload: vi.fn(async () => ({
        upload: { id: "upload-1" },
        maxChunkSizeBytes: 4n,
        maxAttachmentSizeBytes: 20n,
      })),
      writeAttachmentChunk: vi.fn(async () => {
        throw new Error("connection lost");
      }),
      commitAttachmentUpload: vi.fn(),
      abortAttachmentUpload: vi.fn(async () => ({})),
    } as unknown as AttachmentClient;

    await expect(
      uploadAttachment(
        client,
        "board-1",
        undefined,
        "browser-actor",
        uploadFile("notes.txt", "content"),
        vi.fn(),
      ),
    ).rejects.toThrow("connection lost");
    expect(client.abortAttachmentUpload).toHaveBeenCalledWith({
      uploadId: "upload-1",
      mutation: { actor: "browser-actor" },
    });
  });

  it("offers the raw content route only when local bytes are usable", () => {
    expect(
      attachmentContent(
        attachment("diagram.png", BlobAvailability.VERIFIED),
        "board with spaces",
      ),
    ).toEqual({
      available: true,
      href: "/attachments/attachment-id/content?board_id=board%20with%20spaces",
      label: "Download",
    });
    expect(
      attachmentContent(
        attachment("missing.png", BlobAvailability.MISSING),
        "board-1",
      ),
    ).toEqual({
      available: false,
      label: "Missing locally",
    });
    expect(
      attachmentContent(
        attachment("corrupt.png", BlobAvailability.DIGEST_MISMATCH),
        "board-1",
      ),
    ).toEqual({
      available: false,
      label: "Digest mismatch",
    });
  });
});

function attachment(
  filename: string,
  availability = BlobAvailability.PRESENT_UNVERIFIED,
): Attachment {
  return {
    $typeName: "cardamom.private.v1.Attachment",
    id: "attachment-id",
    boardId: "board-1",
    blob: {
      $typeName: "cardamom.private.v1.BlobDescriptor",
      digest: `sha256:${"0".repeat(64)}`,
      sizeBytes: 6n,
    },
    filename,
    mediaType: "text/plain",
    lifecycle: AttachmentLifecycle.ACTIVE,
    availability,
  };
}

function uploadFile(name: string, content: string): AttachmentUploadFile {
  const blob = new Blob([content]);
  return {
    name,
    size: blob.size,
    slice: (start, end) => blob.slice(start, end),
  };
}

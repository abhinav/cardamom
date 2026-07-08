import { describe, expect, it, vi } from "vitest";

import type { AttachmentUploadFile } from "./attachments.tsx";
import {
  createIssueWithAttachments,
  issueCreationInput,
  issueCreationBoardId,
  type IssueCreationDraft,
} from "./create-issue.tsx";
import { IssueType } from "./gen/cardamom/private/v1/issue_pb.ts";

describe("Create issue workflow", () => {
  it("is available only for a concrete board selection", () => {
    expect(issueCreationBoardId({ kind: "board", boardId: "board-1" })).toBe(
      "board-1",
    );
    expect(issueCreationBoardId({ kind: "all" })).toBeUndefined();
    expect(issueCreationBoardId({ kind: "unresolved" })).toBeUndefined();
  });

  it("normalizes every accepted field for CreateIssue", () => {
    const draft: IssueCreationDraft = {
      title: "  Integrate browser routes  ",
      type: IssueType.WORKSTREAM,
      priority: 2,
      summary: "Connect every route.",
      details: "## Intent\n\nPreserve route context.",
      labels: " area:web, priority:high, area:web ",
      parent: " cm-parent ",
      prerequisites: "cm-first, cm-second\ncm-first",
    };

    expect(issueCreationInput(
      "board-1",
      "  browser-actor  ",
      draft,
    )).toEqual({
      boardId: "board-1",
      title: "Integrate browser routes",
      type: IssueType.WORKSTREAM,
      priority: 2,
      summarySource: "Connect every route.",
      detailsSource: "## Intent\n\nPreserve route context.",
      labels: ["area:web", "priority:high"],
      parentId: "cm-parent",
      prerequisiteIds: ["cm-first", "cm-second"],
      context: { actor: "browser-actor" },
    });
  });

  it("reports the created issue and failed file without repeating creation", async () => {
    const createIssue = vi.fn(async () => "cm-created");
    const uploads: string[] = [];
    const upload = vi.fn(async (_issueId: string, file: AttachmentUploadFile) => {
      uploads.push(file.name);
      if (file.name === "second.txt") {
        throw new Error("connection lost");
      }
    });

    const result = await createIssueWithAttachments(
      createIssue,
      [
        new File(["first"], "first.txt"),
        new File(["second"], "second.txt"),
        new File(["third"], "third.txt"),
      ],
      upload,
      vi.fn(),
    );

    expect(result).toEqual({
      kind: "upload-error",
      issueId: "cm-created",
      filename: "second.txt",
      uploadedCount: 1,
      totalCount: 3,
      message: "connection lost",
    });
    expect(createIssue).toHaveBeenCalledOnce();
    expect(upload).toHaveBeenCalledTimes(2);
    expect(upload).toHaveBeenNthCalledWith(
      1,
      "cm-created",
      expect.objectContaining({ name: "first.txt" }),
      expect.any(Function),
    );
    expect(upload).toHaveBeenNthCalledWith(
      2,
      "cm-created",
      expect.objectContaining({ name: "second.txt" }),
      expect.any(Function),
    );
    expect(uploads).toEqual(["first.txt", "second.txt"]);
  });

  it("does not start attachment uploads when issue creation fails", async () => {
    const upload = vi.fn();

    const result = await createIssueWithAttachments(
      async () => {
        throw new Error("title is required");
      },
      [new File(["draft"], "draft.txt")],
      upload,
      vi.fn(),
    );

    expect(result).toEqual({
      kind: "creation-error",
      message: "title is required",
    });
    expect(upload).not.toHaveBeenCalled();
  });

  it("returns the created issue after every staged file uploads", async () => {
    const uploads: string[] = [];

    const result = await createIssueWithAttachments(
      async () => "cm-created",
      [
        new File(["first"], "first.txt"),
        new File(["second"], "second.txt"),
      ],
      async (issueId, file) => {
        uploads.push(`${issueId}:${file.name}`);
      },
      vi.fn(),
    );

    expect(result).toEqual({ kind: "success", issueId: "cm-created" });
    expect(uploads).toEqual([
      "cm-created:first.txt",
      "cm-created:second.txt",
    ]);
  });
});

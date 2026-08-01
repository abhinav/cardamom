import { create } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";
import { describe, expect, it, vi } from "vitest";

import {
  ActionableCheckpointSchema,
  ResolveCheckpointResponseSchema,
} from "../gen/cardamom/private/v1/checkpoint_pb.ts";
import { MarkdownContentSchema } from "../gen/cardamom/private/v1/content_pb.ts";
import {
  IssueContextSchema,
  CheckpointDecisionSchema,
  CheckpointOutcome,
  IssueLifecycle,
  RelatedIssueSchema,
  IssueSummarySchema,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import {
  approvalPresentation,
  checkpointOutcome,
  resolveActionableCheckpoint,
} from "./approvals.tsx";

describe("Approval card", () => {
  it("shows only the checkpoint details needed for a human decision", () => {
    const description = create(MarkdownContentSchema, {
      source: "Concise checkpoint description.",
      renderedHtml: "<p>Concise checkpoint description.</p>",
    });
    const checkpoint = create(ActionableCheckpointSchema, {
      checkpoint: create(IssueSummarySchema, {
        id: "cm-check",
        boardId: "private-board",
        title: "Release production",
        priority: 1,
      }),
      summary: description,
      context: create(IssueContextSchema, {
        boardDescription: create(MarkdownContentSchema, {
          source: "Agent board instructions",
          renderedHtml: "<p>Agent board instructions</p>",
        }),
      }),
      blockedIssues: [
        create(RelatedIssueSchema, {
          id: "cm-dependent",
          title: "Agent waiting on this decision",
        }),
      ],
    });

    expect(approvalPresentation(checkpoint)).toEqual({
      description,
      issueHref: "/board/private-board/issue/cm-check",
      issueID: "cm-check",
      readiness: "Ready",
      reasonID: "approval-reason-cm-check",
      title: "Release production",
    });
  });
});

describe("Approvals RPC boundary", () => {
  it("reports the durable decision outcome, reason, time, and lifecycle", () => {
    const result = create(ResolveCheckpointResponseSchema, {
      decision: create(CheckpointDecisionSchema, {
        outcome: CheckpointOutcome.APPROVED,
        reason: create(MarkdownContentSchema, { source: "Staging passed." }),
        decidedAt: create(TimestampSchema, { seconds: 200n }),
        revision: 8n,
      }),
      checkpoint: create(IssueSummarySchema, {
        title: "Release production",
        lifecycle: IssueLifecycle.CLOSED,
      }),
    });

    expect(checkpointOutcome(result)).toContain("Release production approved.");
    expect(checkpointOutcome(result)).toContain("Reason: Staging passed.");
    expect(checkpointOutcome(result)).toContain("Decided ");
    expect(checkpointOutcome(result)).toContain("Lifecycle: Closed.");
  });

  it("attributes a decision to the browser actor and trims an optional reason", () => {
    expect(resolveActionableCheckpoint(
      "cm-check",
      "browser-actor",
      CheckpointOutcome.APPROVED,
      "  verified in staging  ",
    )).toEqual({
      issueId: "cm-check",
      context: { actor: "browser-actor" },
      outcome: CheckpointOutcome.APPROVED,
      reason: "verified in staging",
    });
  });

  it("omits an empty decision reason", () => {
    expect(resolveActionableCheckpoint(
      "cm-check",
      "browser-actor",
      CheckpointOutcome.DENIED,
      "   ",
    )).toEqual({
      issueId: "cm-check",
      context: { actor: "browser-actor" },
      outcome: CheckpointOutcome.DENIED,
    });
  });
});

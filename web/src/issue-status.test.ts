import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { IssueStatus } from "./gen/cardamom/private/v1/issue_pb.ts";
import {
  IssueStatusDot,
  issueStatusPresentation,
} from "./issue-status.tsx";

describe("issue status presentation", () => {
  it("assigns one visible label and semantic state to every issue status", () => {
    expect([
      issueStatusPresentation(IssueStatus.READY),
      issueStatusPresentation(IssueStatus.BLOCKED),
      issueStatusPresentation(IssueStatus.IN_PROGRESS),
      issueStatusPresentation(IssueStatus.WAITING),
      issueStatusPresentation(IssueStatus.CLOSED),
      issueStatusPresentation(IssueStatus.CANCELLED),
      issueStatusPresentation(IssueStatus.UNSPECIFIED),
    ]).toEqual([
      { label: "Ready", state: "ready" },
      { label: "Blocked", state: "blocked" },
      { label: "In progress", state: "in-progress" },
      { label: "Waiting", state: "waiting" },
      { label: "Closed", state: "closed" },
      { label: "Cancelled", state: "cancelled" },
      { label: "Unknown", state: "unknown" },
    ]);
  });

  it("renders Ready as a ring and Cancelled as a labelled X", () => {
    const ready = renderToStaticMarkup(
      createElement(IssueStatusDot, { status: IssueStatus.READY }),
    );
    const cancelled = renderToStaticMarkup(
      createElement(IssueStatusDot, { status: IssueStatus.CANCELLED }),
    );

    expect(ready).not.toContain("<svg");
    expect(cancelled).toContain('class="lucide lucide-x"');
    expect(cancelled).toContain('aria-label="Cancelled"');
    expect(cancelled).toContain('title="Cancelled"');
  });
});

import { create } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  IssueStatus,
  IssueSummarySchema,
  IssueType,
  WaitingStateSchema,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import {
  IssueReferencePill,
  pointerCanPreview,
} from "./issue-reference-pill.tsx";

describe("issue reference pill", () => {
  it("presents compact issue context without changing the pill contents", () => {
    const markup = renderToStaticMarkup(
      <IssueReferencePill
        issue={create(IssueSummarySchema, {
          title: "Design aggregate dashboard routing",
          status: IssueStatus.WAITING,
          type: IssueType.WORKSTREAM,
          priority: 1,
          waiting: create(WaitingStateSchema, {
            reason: "architecture review",
            since: create(TimestampSchema, { seconds: 1_704_165_840n }),
          }),
        })}
        issueID="cm-ja7as"
      >
        <a href="/board/board-1/issue/cm-ja7as">%cm-ja7as</a>
      </IssueReferencePill>,
    );

    expect(markup).toContain('<a href="/board/board-1/issue/cm-ja7as"');
    expect(markup).toContain("%cm-ja7as</a>");
    expect(markup).toContain('role="tooltip"');
    expect(markup).toContain("Design aggregate dashboard routing");
    expect(markup).toContain("Waiting");
    expect(markup).toContain("Waiting: architecture review");
    expect(markup).toContain("Workstream");
    expect(markup).toContain("P1");
    expect(markup).toContain("hidden");
  });

  it("does not add waiting text to a ready issue preview", () => {
    const markup = renderToStaticMarkup(
      <IssueReferencePill
        issue={create(IssueSummarySchema, {
          title: "Ready implementation",
          status: IssueStatus.READY,
          type: IssueType.TASK,
        })}
        issueID="cm-ready"
      >
        <a href="/board/board-1/issue/cm-ready">%cm-ready</a>
      </IssueReferencePill>,
    );

    expect(markup).not.toContain("Waiting:");
  });

  it("defers loading context until the user expresses preview intent", () => {
    const loadIssue = vi.fn();

    const markup = renderToStaticMarkup(
      <IssueReferencePill issueID="cm-ja7as" loadIssue={loadIssue}>
        <a href="/board/board-1/issue/cm-ja7as">%cm-ja7as</a>
      </IssueReferencePill>,
    );

    expect(loadIssue).not.toHaveBeenCalled();
    expect(markup).not.toContain('role="tooltip"');
  });

  it("limits pointer previews to hover-capable non-touch pointers", () => {
    expect(pointerCanPreview("mouse", true)).toBe(true);
    expect(pointerCanPreview("pen", true)).toBe(true);
    expect(pointerCanPreview("touch", true)).toBe(false);
    expect(pointerCanPreview("mouse", false)).toBe(false);
  });
});

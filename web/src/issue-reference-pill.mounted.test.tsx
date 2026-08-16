// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { IssueStatus, IssueType } from "./gen/cardamom/private/v1/issue_pb.ts";
import { IssueReferencePill } from "./issue-reference-pill.tsx";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("issue reference interactions", () => {
  it("loads issue details on hover and keeps the reference copyable", async () => {
    vi.stubGlobal("matchMedia", vi.fn(() => ({ matches: true })));
    const loadIssue = vi.fn(async () => ({
      title: "Investigate queue latency",
      status: IssueStatus.IN_PROGRESS,
      type: IssueType.TASK,
      priority: 1,
    }));
    const writeText = vi.fn(async () => undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    const { container } = render(
      <IssueReferencePill issueID="cm-latency" loadIssue={loadIssue}>
        <a href="/board/board-1/issue/cm-latency">%cm-latency</a>
      </IssueReferencePill>,
    );
    const reference = container.querySelector(".issue-reference-pill");

    expect(reference).not.toBeNull();
    expect(loadIssue).not.toHaveBeenCalled();
    fireEvent.pointerEnter(reference!, { pointerType: "mouse" });

    await waitFor(() => expect(loadIssue).toHaveBeenCalledExactlyOnceWith("cm-latency"));
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Investigate queue latencyIn progressTaskP1",
    );

    fireEvent.click(screen.getByRole("button", {
      name: "Copy issue ID %cm-latency",
    }));
    await waitFor(() => expect(writeText).toHaveBeenCalledExactlyOnceWith("%cm-latency"));
  });
});

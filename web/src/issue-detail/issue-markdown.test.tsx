import { useState } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("lucide-react", () => ({ Copy: () => null, Pencil: () => null }));

const {
  linkMarkdownImages,
  markdownObjectReferenceTargets,
  markdownIssueReferenceTargets,
  scrollToLogEntryFragment,
  useMarkdownEnhancementProps,
} = await import("./issue-detail.tsx");

describe("issue Markdown presentation", () => {
  it("keeps enhancement host props stable through a React rerender", () => {
    const lifecycles: ReturnType<typeof useMarkdownEnhancementProps>[] = [];
    const loadIssue = vi.fn();

    function MarkdownLifecycle() {
      const [revision, setRevision] = useState(0);
      const props = useMarkdownEnhancementProps(
        '<span data-issue-reference="cm-task">%cm-task</span>',
        loadIssue,
      );
      lifecycles.push(props);
      if (revision === 0) {
        setRevision(1);
      }
      return <div {...props} />;
    }

    renderToStaticMarkup(<MarkdownLifecycle />);

    expect(lifecycles).toHaveLength(2);
    expect(lifecycles[1]?.dangerouslySetInnerHTML).toBe(
      lifecycles[0]?.dangerouslySetInnerHTML,
    );
    expect(lifecycles[1]?.ref).toBe(lifecycles[0]?.ref);
  });

  it("opens an image through a new-tab link to the original", () => {
    const link = {
      append: vi.fn(),
      href: "",
      rel: "",
      target: "",
    };
    const image = {
      closest: vi.fn(() => null),
      replaceWith: vi.fn(),
      src: "https://cardamom.test/attachments/mobile.png",
    };
    const root = {
      ownerDocument: {
        createElement: vi.fn(() => link),
      },
      querySelectorAll: vi.fn(() => [image]),
    };

    linkMarkdownImages(root as unknown as HTMLDivElement);

    expect(root.ownerDocument.createElement).toHaveBeenCalledWith("a");
    expect(image.replaceWith).toHaveBeenCalledWith(link);
    expect(link.append).toHaveBeenCalledWith(image);
    expect(link).toMatchObject({
      href: "https://cardamom.test/attachments/mobile.png",
      target: "_blank",
      rel: "noopener noreferrer",
    });
  });

  it("discovers the same issue marker after repeated enhancement", () => {
    const marker = {
      dataset: { issueReference: "cm-task" },
      getAttribute: vi.fn((name: string) =>
        name === "data-issue-reference-href" ? "/issues/cm-task" : null,
      ),
      replaceChildren: vi.fn(),
    };
    const root = {
      querySelectorAll: vi.fn(() => [marker]),
    } as unknown as HTMLDivElement;

    const first = markdownIssueReferenceTargets(root);
    const second = markdownIssueReferenceTargets(root);

    expect(first).toEqual([{
      element: marker,
      href: "/issues/cm-task",
      id: "cm-task",
    }]);
    expect(second).toEqual(first);
    expect(marker.replaceChildren).toHaveBeenCalledTimes(2);
  });

  it("prepares resolved log and attachment markers for clipboard pills", () => {
    const log = {
      dataset: {
        cardamomReference: "log",
        cardamomReferenceId: "log_0123456789abcdef0123456789abcdef",
        cardamomReferenceLabel: "%log_0123456789abcdef0123456789abcdef",
      },
      getAttribute: vi.fn(() =>
        "/issues/cm-owner#log_0123456789abcdef0123456789abcdef"
      ),
      replaceChildren: vi.fn(),
    };
    const attachment = {
      dataset: {
        cardamomReference: "attachment",
        cardamomReferenceId: "att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
        cardamomReferenceLabel: "diagnostic report.pdf",
      },
      getAttribute: vi.fn(() =>
        "/attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/content?board_id=board-1"
      ),
      replaceChildren: vi.fn(),
    };
    const root = {
      querySelectorAll: vi.fn(() => [log, attachment]),
    } as unknown as HTMLDivElement;

    expect(markdownObjectReferenceTargets(root)).toEqual([
      {
        element: log,
        href: "/issues/cm-owner#log_0123456789abcdef0123456789abcdef",
        id: "log_0123456789abcdef0123456789abcdef",
        kind: "log",
        label: "%log_0123456789abcdef0123456789abcdef",
      },
      {
        element: attachment,
        href: "/attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/content?board_id=board-1",
        id: "att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
        kind: "attachment",
        label: "diagnostic report.pdf",
      },
    ]);
    expect(log.replaceChildren).toHaveBeenCalledOnce();
    expect(attachment.replaceChildren).toHaveBeenCalledOnce();
  });

  it("scrolls to a log fragment after asynchronous entries mount", () => {
    const target = { scrollIntoView: vi.fn() };
    const document = {
      getElementById: vi.fn(() => target),
    };

    scrollToLogEntryFragment(
      [{ id: "log_0123456789abcdef0123456789abcdef" }],
      "#log_0123456789abcdef0123456789abcdef",
      document as unknown as Document,
    );

    expect(document.getElementById).toHaveBeenCalledWith(
      "log_0123456789abcdef0123456789abcdef",
    );
    expect(target.scrollIntoView).toHaveBeenCalledWith({ block: "start" });
  });
});

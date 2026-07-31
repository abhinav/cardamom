import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AttachmentIdentity } from "./attachment-identity.tsx";

const attachment = {
  filename: "incident-log.txt",
  id: "att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
};

describe("attachment identity", () => {
  it("presents an attachment ID copy pill without a Markdown reference", () => {
    const markup = renderToStaticMarkup(
      createElement(AttachmentIdentity, { attachment }),
    );

    expect(markup).toContain("incident-log.txt");
    expect(markup).toContain("att_aaaaaaaaaaaaaaaaaaaaaaaaaa");
    expect(markup).toContain(
      'aria-label="Copy attachment ID att_aaaaaaaaaaaaaaaaaaaaaaaaaa"',
    );
    expect(markup).not.toContain("attachment:");
    expect(markup).not.toContain("Markdown reference");
  });

  it("retains the full filename when the visible label truncates", () => {
    const filename = `2026-06-13-${"merge-roadmap-".repeat(8)}plan.md`;
    const markup = renderToStaticMarkup(
      createElement(AttachmentIdentity, {
        attachment: { ...attachment, filename },
      }),
    );

    expect(markup).toContain(`<strong title="${filename}">${filename}</strong>`);
  });
});

import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const issueDetailStyles = readFileSync(
  new URL("./issue-detail.css", import.meta.url),
  "utf8",
);

describe("issue Markdown styles", () => {
  it("constrains images to the record width without distortion", () => {
    const imageRule = issueDetailStyles.match(
      /\.issue-markdown img\s*\{(?<declarations>[^}]*)\}/,
    )?.groups?.declarations;

    expect(imageRule).toMatch(/max-width:\s*100%;/);
    expect(imageRule).toMatch(/height:\s*auto;/);
  });

  it("positions and indicates a targeted log entry", () => {
    const logItemRule = issueDetailStyles.match(
      /\.issue-log-list > li\s*\{(?<declarations>[^}]*)\}/,
    )?.groups?.declarations;
    const targetRule = issueDetailStyles.match(
      /\.issue-log-list > li:target\s*\{(?<declarations>[^}]*)\}/,
    )?.groups?.declarations;

    expect(logItemRule).toMatch(/scroll-margin-block-start:/);
    expect(targetRule).toMatch(/border-inline-start-color:/);
    expect(targetRule).toMatch(/background:/);
  });

  it("does not divide the final log entry from empty space", () => {
    const lastLogItemRule = issueDetailStyles.match(
      /\.issue-log-list > li:last-child\s*\{(?<declarations>[^}]*)\}/,
    )?.groups?.declarations;

    expect(lastLogItemRule).toMatch(/border-bottom:\s*0;/);
  });
});

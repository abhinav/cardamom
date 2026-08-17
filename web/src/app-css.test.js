import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const appStyles = readFileSync(new URL("./app.css", import.meta.url), "utf8");

describe("application shell styles", () => {
  it("reserves the mobile navigation footprint once", () => {
    const mobileFrameRule = [...appStyles.matchAll(
      /\.app-frame\s*\{(?<declarations>[^}]*)\}/g,
    )].find(({ groups }) =>
      groups?.declarations.includes("grid-template-rows: auto 1fr auto")
    )?.groups?.declarations;
    const mobileMainRules = [...appStyles.matchAll(
      /\.main-content\s*\{(?<declarations>[^}]*)\}/g,
    )]
      .map(({ groups }) => groups?.declarations ?? "")
      .filter((declarations) => declarations.includes("padding:"));

    expect(mobileFrameRule).toMatch(/padding-bottom:\s*3\.5rem;/);
    expect(mobileMainRules.join("\n")).not.toMatch(/(?:^|\s)5rem(?:\s|;)/);
  });
});

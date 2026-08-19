import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const appStyles = readFileSync(new URL("./app.css", import.meta.url), "utf8");

describe("application shell styles", () => {
  it("keeps header overlays above collection content", () => {
    const headerRule = appStyles.match(
      /\.app-header\s*\{(?<declarations>[^}]*)\}/,
    )?.groups?.declarations;

    expect(headerRule).toMatch(/position:\s*relative;/);
    expect(headerRule).toMatch(/z-index:\s*11;/);
  });

  it("does not force the document wider than a narrow mobile viewport", () => {
    const bodyRule = appStyles.match(
      /^body\s*\{(?<declarations>[^}]*)\}/m,
    )?.groups?.declarations;

    expect(bodyRule).toBeDefined();
    expect(bodyRule).not.toMatch(/min-width:/);
  });

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

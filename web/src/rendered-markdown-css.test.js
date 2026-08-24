import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const markdownStyles = readFileSync(
  new URL("./rendered-markdown.css", import.meta.url),
  "utf8",
);

function declarations(selector) {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return markdownStyles.match(
    new RegExp(`${escapedSelector}\\s*\\{(?<declarations>[^}]*)\\}`),
  )?.groups?.declarations;
}

describe("rendered Markdown styles", () => {
  it("separates prose blocks and gives headings a visible hierarchy", () => {
    expect(declarations(".rendered-markdown > * + *"))
      .toMatch(/margin-block-start:/);
    expect(declarations(".rendered-markdown h1"))
      .toMatch(/font-size:\s*1\.5rem;/);
    expect(declarations(".rendered-markdown h2"))
      .toMatch(/font-size:\s*1\.3rem;/);
    expect(declarations(".rendered-markdown h3"))
      .toMatch(/font-size:\s*1\.125rem;/);
  });

  it("restores list markers and nested indentation", () => {
    expect(declarations(".rendered-markdown ul"))
      .toMatch(/list-style:\s*disc;/);
    expect(declarations(".rendered-markdown ol"))
      .toMatch(/list-style:\s*decimal;/);
    expect(declarations(".rendered-markdown ul,\n.rendered-markdown ol"))
      .toMatch(/padding-inline-start:/);
  });

  it("distinguishes tables and inline code from surrounding prose", () => {
    expect(declarations(".rendered-markdown table"))
      .toMatch(/overflow-x:\s*auto;/);
    expect(declarations(
      ".rendered-markdown th,\n.rendered-markdown td",
    )).toMatch(/padding:/);
    expect(declarations(".rendered-markdown :not(pre) > code"))
      .toMatch(/background:/);
  });
});

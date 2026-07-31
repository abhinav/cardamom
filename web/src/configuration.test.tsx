import { create } from "@bufbuild/protobuf";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  ConfigurationContent,
  configurationPreview,
  configurationUpdateInput,
  validateConfigurationDraft,
} from "./configuration.tsx";
import {
  ConfigurationIssueIDStrategy,
  ConfigurationScope,
  ConfigurationViewSchema,
} from "./gen/cardamom/private/v1/configuration_pb.ts";

describe("Configuration workflow", () => {
  it("builds one-field typed updates and resets", () => {
    expect(
      configurationUpdateInput(
        "board-1",
        "  browser-actor  ",
        ConfigurationScope.PROJECT,
        "issue.id.prefix",
        "work-",
      ),
    ).toEqual({
      boardId: "board-1",
      scope: ConfigurationScope.PROJECT,
      overrides: { issue: { id: { prefix: "work-" } } },
      updateMask: { paths: ["issue.id.prefix"] },
      context: { actor: "browser-actor" },
    });

    expect(
      configurationUpdateInput(
        "board-1",
        "browser-actor",
        ConfigurationScope.BOARD,
        "issue.id.strategy",
        undefined,
      ),
    ).toEqual({
      boardId: "board-1",
      scope: ConfigurationScope.BOARD,
      overrides: {},
      updateMask: { paths: ["issue.id.strategy"] },
      context: { actor: "browser-actor" },
    });
  });

  it("validates editable values before submission", () => {
    expect(validateConfigurationDraft("issue.id.prefix", "Work"))
      .toBe("Use lowercase letters, digits, or dashes, end with a dash, and limit the prefix to 16 characters.");
    expect(validateConfigurationDraft("issue.id.prefix", "work-")).toBeUndefined();
    expect(validateConfigurationDraft("issue.summary.max_bytes", "0"))
      .toBe("Enter a whole number of bytes between 1 and 9223372036854775807.");
    expect(validateConfigurationDraft("attachment.max_bytes", "104857600"))
      .toBeUndefined();
  });

  it("previews the effective value after edit and reset", () => {
    const view = configurationView();

    expect(
      configurationPreview(
        view,
        ConfigurationScope.STORE,
        "attachment.max_bytes",
        "83886080",
      ),
    ).toEqual({ value: "80 MiB", source: "Store" });
    expect(
      configurationPreview(
        view,
        ConfigurationScope.BOARD,
        "issue.id.strategy",
        undefined,
      ),
    ).toEqual({ value: "random", source: "Built-in" });
    expect(
      configurationPreview(
        view,
        ConfigurationScope.PROJECT,
        "issue.summary.max_bytes",
        "not-a-number",
      ),
    ).toEqual({ value: "4 KiB", source: "Project" });
    expect(
      configurationPreview(
        view,
        ConfigurationScope.BOARD,
        "issue.id.prefix",
        "Work",
      ),
    ).toEqual({ value: "Work", source: "Board" });
  });

  it("renders effective values before the four-layer cascade", () => {
    const markup = renderToStaticMarkup(
      <ConfigurationContent
        boardName="Default"
        canMutateServer
        onBeginEdit={vi.fn()}
        view={configurationView()}
      />,
    );

    expect(markup.indexOf("Effective for Default")).toBeLessThan(
      markup.indexOf("Resolution layers"),
    );
    expect(markup).toContain("work-");
    expect(markup).toContain("sequential");
    expect(markup).toContain("Local to this store");
    expect(markup).toContain("Cardamom defaults");
    expect(markup).toContain("/tmp/test-store/.cardamom");
    expect(markup.match(/class="configuration-layer"/g)).toHaveLength(4);
    expect(markup).not.toContain("Save all");
  });

  it("keeps resolved layers readable without configuration controls", () => {
    const markup = renderToStaticMarkup(
      <ConfigurationContent
        boardName="Default"
        canMutateServer={false}
        onBeginEdit={vi.fn()}
        view={configurationView()}
      />,
    );

    expect(markup).toContain("Effective for Default");
    expect(markup).toContain("Resolution layers");
    expect(markup).not.toContain("configuration-field-actions");
    expect(markup).not.toContain(">Edit</button>");
    expect(markup).not.toContain(">Reset</button>");
  });
});

function configurationView() {
  return create(ConfigurationViewSchema, {
    layers: [
      {
        source: { scope: ConfigurationScope.BUILT_IN, identity: "built-in" },
        overrides: {
          issue: {
            id: {
              prefix: "cm-",
              strategy:
                ConfigurationIssueIDStrategy.CONFIGURATION_ISSUE_ID_STRATEGY_RANDOM,
            },
            summary: { maxBytes: 2048n },
          },
          attachment: { maxBytes: 104857600n },
        },
      },
      {
        source: {
          scope: ConfigurationScope.STORE,
          identity: "/tmp/test-store/.cardamom",
        },
        overrides: { attachment: { maxBytes: 78643200n } },
      },
      {
        source: {
          scope: ConfigurationScope.PROJECT,
          identity: "project-1",
        },
        overrides: {
          issue: {
            id: { prefix: "work-" },
            summary: { maxBytes: 4096n },
          },
        },
      },
      {
        source: { scope: ConfigurationScope.BOARD, identity: "board-1" },
        overrides: {
          issue: {
            id: {
              strategy:
                ConfigurationIssueIDStrategy.CONFIGURATION_ISSUE_ID_STRATEGY_SEQUENTIAL,
            },
          },
        },
      },
    ],
    effective: {
      issue: {
        id: {
          prefix: "work-",
          strategy:
            ConfigurationIssueIDStrategy.CONFIGURATION_ISSUE_ID_STRATEGY_SEQUENTIAL,
        },
        summary: { maxBytes: 4096n },
      },
      attachment: { maxBytes: 78643200n },
    },
    origins: {
      issue: {
        id: {
          prefix: {
            scope: ConfigurationScope.PROJECT,
            identity: "project-1",
          },
          strategy: {
            scope: ConfigurationScope.BOARD,
            identity: "board-1",
          },
        },
        summary: {
          maxBytes: {
            scope: ConfigurationScope.PROJECT,
            identity: "project-1",
          },
        },
      },
      attachment: {
        maxBytes: {
          scope: ConfigurationScope.STORE,
          identity: "/tmp/test-store/.cardamom",
        },
      },
    },
  });
}

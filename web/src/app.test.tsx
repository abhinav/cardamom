import { createRouterTransport } from "@connectrpc/connect";
import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { createWebClient } from "./api.ts";
import { App } from "./app.tsx";
import { bootstrapQueryOptions } from "./query-runtime.ts";

describe("application shell", () => {
  it("names Cardamom while startup metadata is loading", () => {
    const markup = renderApp(new QueryClient());

    expect(markup).toContain("Loading Cardamom");
    expect(markup).toContain(">Cardamom</div>");
  });

  it("shows the selected board and project in the header selector", () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
      serverDefaultBoardId: "board-1",
    });

    const markup = renderApp(queryClient, transport);

    expect(markup).toContain('aria-label="Select board scope: Primary"');
    expect(markup).toContain(
      '<span class="board-selector-trigger-primary">Primary</span>',
    );
    expect(markup).toContain(
      '<span class="board-selector-trigger-secondary">Cardamom</span>',
    );
  });

  it("directs an unresolved board scope without naming a control location", () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
        { id: "board-2", projectId: "project-1", name: "Secondary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });

    const markup = renderApp(queryClient, transport);

    expect(markup).toContain("Select a board to load issues.");
    expect(markup).not.toContain("Select a board in Settings");
  });
});

function renderApp(
  queryClient: QueryClient,
  transport = createRouterTransport(() => {}),
): string {
  return renderToStaticMarkup(
    createElement(
      TransportProvider,
      { transport },
      createElement(
        QueryClientProvider,
        { client: queryClient },
        createElement(
          MemoryRouter,
          null,
          createElement(App, {
            client: createWebClient(transport),
            storage: {
              getItem: () => null,
              setItem: () => {},
            },
          }),
        ),
      ),
    ),
  );
}

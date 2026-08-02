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

  it("renders the root as a board picker even with a server default", () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
      serverDefaultBoardId: "board-1",
    });

    const markup = renderApp(queryClient, transport, "/");

    expect(markup).toContain(">Boards</h1>");
    expect(markup).toContain('href="/board/board-1"');
    expect(markup).toContain('href="/all"');
    expect(markup).not.toContain('aria-label="Issue board"');
  });

  it("uses the board route instead of persisted scope for shell navigation", () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
        { id: "board-2", projectId: "project-1", name: "Secondary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });

    const markup = renderApp(
      queryClient,
      transport,
      "/board/board-1/list",
      JSON.stringify({
        version: 4,
        actor: "captain",
        theme: "dark",
        boardScope: { kind: "board", boardId: "board-2" },
      }),
    );

    expect(markup).toContain('aria-label="Select board scope: Primary"');
    expect(markup).toContain('href="/board/board-1"');
    expect(markup).toContain('href="/board/board-1/approvals"');
    expect(markup).toContain('href="/board/board-1/list"');
    expect(markup).toContain('href="/board/board-1/routines"');
    expect(markup).not.toContain('href="/list"');
  });

  it("renders canonical all-board navigation", () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });

    const markup = renderApp(queryClient, transport, "/all/approvals");

    expect(markup).toContain('aria-label="Select board scope: All boards"');
    expect(markup).toContain('href="/all"');
    expect(markup).toContain('href="/all/approvals"');
    expect(markup).toContain('href="/all/list"');
    expect(markup).toContain('href="/all/routines"');
  });

  it("loads board configuration from the canonical settings route", () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });

    const markup = renderApp(
      queryClient,
      transport,
      "/board/board-1/settings",
    );

    expect(markup).toContain("Loading configuration");
    expect(markup).toContain('aria-label="Select board scope: Primary"');
    expect(markup).not.toContain("Page not found");
  });
});

function renderApp(
  queryClient: QueryClient,
  transport = createRouterTransport(() => {}),
  path = "/",
  persistedPreferences: string | null = null,
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
          { initialEntries: [path] },
          createElement(App, {
            client: createWebClient(transport),
            storage: {
              getItem: () => persistedPreferences,
              setItem: () => {},
            },
          }),
        ),
      ),
    ),
  );
}

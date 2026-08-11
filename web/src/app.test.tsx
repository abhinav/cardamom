import { create } from "@bufbuild/protobuf";
import { createRouterTransport } from "@connectrpc/connect";
import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { renderToReadableStream } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { createWebClient } from "./api.ts";
import { App } from "./app.tsx";
import {
  IssueDetailSchema,
  IssueService,
  IssueSummarySchema,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import { AccessMode } from "./gen/cardamom/private/v1/project_pb.ts";
import {
  bootstrapQueryOptions,
  unaryRouteQueryOptions,
} from "./query-runtime.ts";

describe("application shell", () => {
  it("names Cardamom while startup metadata is loading", async () => {
    const markup = await renderApp(new QueryClient());

    expect(markup).toContain("Loading Cardamom");
    expect(markup).toContain(">Cardamom</div>");
  });

  it("renders the root as a board picker even with a server default", async () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
      serverDefaultBoardId: "board-1",
    });

    const markup = await renderApp(queryClient, transport, "/");

    expect(markup).toContain(">Boards</h1>");
    expect(markup).toContain('href="/board/board-1"');
    expect(markup).toContain('href="/all"');
    expect(markup).not.toContain('aria-label="Issue board"');
  });

  it("uses the board route instead of persisted scope for shell navigation", async () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
        { id: "board-2", projectId: "project-1", name: "Secondary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });

    const markup = await renderApp(
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
    expect(markup).toContain('href="/board/board-1?lifecycle=all"');
    expect(markup).toContain('href="/board/board-1/approvals"');
    expect(markup).toContain('href="/board/board-1/list"');
    expect(markup).toContain('href="/board/board-1/routines"');
    expect(markup).not.toContain('href="/list"');
  });

  it("preserves effective filters between Board and List links", async () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });

    const markup = await renderApp(
      queryClient,
      transport,
      "/board/board-1?status=in-progress&title=route+filters",
    );

    expect(markup).toContain(
      'href="/board/board-1?status=in-progress&amp;title=route+filters"',
    );
    expect(markup).toContain(
      'href="/board/board-1/list?lifecycle=current&amp;status=in-progress&amp;' +
        'title=route+filters"',
    );
  });

  it("renders canonical all-board navigation", async () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });

    const markup = await renderApp(queryClient, transport, "/all/approvals");

    expect(markup).toContain('aria-label="Select board scope: All boards"');
    expect(markup).toContain('href="/all"');
    expect(markup).toContain('href="/all/approvals"');
    expect(markup).toContain('href="/all/list"');
    expect(markup).toContain('href="/all/routines"');
  });

  it("loads board configuration from the canonical settings route", async () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });

    const markup = await renderApp(
      queryClient,
      transport,
      "/board/board-1/settings",
    );

    expect(markup).toContain("Loading configuration");
    expect(markup).toContain('aria-label="Select board scope: Primary"');
    expect(markup).not.toContain("Page not found");
  });

  it("loads a canonical issue route in its board scope", async () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });

    const markup = await renderApp(
      queryClient,
      transport,
      "/board/board-1/issue/cm-direct",
    );

    expect(markup).toContain("Loading cm-direct");
    expect(markup).toContain('aria-label="Select board scope: Primary"');
    expect(markup).toContain('href="/board/board-1"');
    expect(markup).not.toContain("Page not found");
  });

  it("rejects an issue returned outside the route board scope", async () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      accessMode: AccessMode.READ_WRITE,
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
        { id: "board-2", projectId: "project-1", name: "Secondary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });
    queryClient.setQueryData(
      unaryRouteQueryOptions(
        IssueService.method.getIssue,
        { issueId: "cm-wrong-board" },
        transport,
      ).queryKey,
      {
        issue: create(IssueDetailSchema, {
          issue: create(IssueSummarySchema, {
            id: "cm-wrong-board",
            boardId: "board-2",
            title: "Wrong board issue",
          }),
        }),
      },
    );

    const markup = await renderApp(
      queryClient,
      transport,
      "/board/board-1/issue/cm-wrong-board",
    );

    expect(markup).toContain("Issue could not be loaded");
    expect(markup).not.toContain("Wrong board issue");
    expect(markup).not.toContain("Issue actions");
    expect(markup).not.toContain("Edit issue");
  });

  it("does not register the transitional issue route", async () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });

    const markup = await renderApp(queryClient, transport, "/issues/cm-legacy");

    expect(markup).toContain("Page not found");
  });
});

function renderApp(
  queryClient: QueryClient,
  transport = createRouterTransport(() => {}),
  path = "/",
  persistedPreferences: string | null = null,
): Promise<string> {
  return renderAppToString(
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

async function renderAppToString(element: ReactNode): Promise<string> {
  const stream = await renderToReadableStream(element);
  await stream.allReady;
  return new Response(stream).text();
}

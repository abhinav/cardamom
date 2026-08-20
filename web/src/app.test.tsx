import { create } from "@bufbuild/protobuf";
import { createRouterTransport } from "@connectrpc/connect";
import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { renderToReadableStream } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { createWebClient } from "./api.ts";
import { App, boardConfigurationActor } from "./app.tsx";
import {
  IssueDetailSchema,
  IssueService,
  ListBoardPinsResponseSchema,
  RelatedIssueSchema,
  IssueSummarySchema,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import {
  AccessMode,
  BoardArchiveSchema,
} from "./gen/cardamom/private/v1/project_pb.ts";
import { SourceRefSchema } from "./gen/cardamom/private/v1/source_pb.ts";
import {
  bootstrapQueryOptions,
  unaryRouteQueryOptions,
  unaryScopeQueryOptions,
} from "./query-runtime.ts";

describe("application shell", () => {
  it("enables board configuration edits only on a writable local server", () => {
    expect(boardConfigurationActor(false, true, "captain")).toBe("captain");
    expect(boardConfigurationActor(true, true, "captain")).toBeUndefined();
    expect(boardConfigurationActor(false, false, "captain")).toBeUndefined();
  });

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

  it("keeps copied board IDs distinct in aggregate source routes", async () => {
    const first = create(SourceRefSchema, {
      sourceId: "first",
      storeLineageId: "lineage-first",
    });
    const second = create(SourceRefSchema, {
      sourceId: "second",
      storeLineageId: "lineage-second",
    });
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      aggregateStatus: { complete: true },
      sources: [{ source: first }, { source: second }],
      boards: [
        {
          id: "board-copied",
          projectId: "project-first",
          name: "Original",
          source: first,
        },
        {
          id: "board-copied",
          projectId: "project-second",
          name: "Restored",
          source: second,
        },
      ],
      projects: [
        { id: "project-first", name: "First", source: first },
        { id: "project-second", name: "Second", source: second },
      ],
    });
    queryClient.setQueryData(
      unaryScopeQueryOptions(
        IssueService.method.listBoardPins,
        { boardId: "board-copied", source: second },
        transport,
      ).queryKey,
      create(ListBoardPinsResponseSchema, {
        issues: [
          create(RelatedIssueSchema, {
            id: "card-restored",
            boardId: "board-copied",
            title: "Restored board issue",
            source: second,
          }),
        ],
      }),
    );

    const markup = await renderApp(
      queryClient,
      transport,
      "/source/second/board/board-copied",
    );

    expect(markup).toContain("Select board scope: Restored");
    expect(markup).toContain('href="/source/second/board/board-copied"');
    expect(markup).toContain(
      'href="/source/second/board/board-copied/issue/card-restored"',
    );
    expect(markup).toContain("Restored board issue");
    expect(markup).not.toContain("Board ID is ambiguous");
  });

  it("shows board pins while the issue columns are still loading", async () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });
    queryClient.setQueryData(
      unaryScopeQueryOptions(
        IssueService.method.listBoardPins,
        { boardId: "board-1" },
        transport,
      ).queryKey,
      create(ListBoardPinsResponseSchema, {
        issues: [
          create(RelatedIssueSchema, {
            id: "cm-pin",
            boardId: "board-1",
            title: "Pinned while loading",
          }),
        ],
      }),
    );

    const markup = await renderApp(
      queryClient,
      transport,
      "/board/board-1",
    );

    expect(markup).toContain("Pinned while loading");
    expect(markup).toContain("Loading issues");
  });

  it("keeps cached board pins visible after a refresh failure", async () => {
    const transport = createRouterTransport(({ service }) => {
      service(IssueService, {
        listBoardPins: () => {
          throw new Error("refresh failed");
        },
      });
    });
    const queryClient = new QueryClient();
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [
        { id: "board-1", projectId: "project-1", name: "Primary" },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });
    const pinOptions = unaryScopeQueryOptions(
      IssueService.method.listBoardPins,
      { boardId: "board-1" },
      transport,
    );
    queryClient.setQueryData(
      pinOptions.queryKey,
      create(ListBoardPinsResponseSchema, {
        issues: [
          create(RelatedIssueSchema, {
            id: "cm-cached",
            boardId: "board-1",
            title: "Cached pin",
          }),
        ],
      }),
    );
    await expect(queryClient.fetchQuery(pinOptions)).rejects.toThrow();

    const markup = await renderApp(
      queryClient,
      transport,
      "/board/board-1",
    );

    expect(markup).toContain("Cached pin");
    expect(markup).toContain("Pinned issues could not be refreshed.");
    expect(markup).toContain(">Retry</button>");
  });

  it("does not offer issue mutations on an archived board", async () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      accessMode: AccessMode.READ_WRITE,
      boards: [
        {
          id: "board-1",
          projectId: "project-1",
          name: "Archived",
          archived: create(BoardArchiveSchema),
        },
      ],
      projects: [{ id: "project-1", name: "Cardamom" }],
    });
    queryClient.setQueryData(
      unaryRouteQueryOptions(
        IssueService.method.getIssue,
        { issueId: "cm-archived" },
        transport,
      ).queryKey,
      {
        issue: create(IssueDetailSchema, {
          issue: create(IssueSummarySchema, {
            id: "cm-archived",
            boardId: "board-1",
            title: "Archived board issue",
          }),
        }),
      },
    );

    const markup = await renderApp(
      queryClient,
      transport,
      "/board/board-1/issue/cm-archived",
    );

    expect(markup).toContain("Archived board issue");
    expect(markup).not.toContain("Issue actions");
    expect(markup).not.toContain("Pin to board");
    expect(markup).not.toContain("Unpin from board");
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

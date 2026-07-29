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

describe("application identity", () => {
  it("names Cardamom while startup metadata is loading", () => {
    const markup = renderApp(new QueryClient());

    expect(markup).toContain("Loading Cardamom");
    expect(markup).toContain(">Cardamom</div>");
  });

  it("names Cardamom in the loaded application shell", () => {
    const queryClient = new QueryClient();
    const transport = createRouterTransport(() => {});
    queryClient.setQueryData(bootstrapQueryOptions(transport).queryKey, {
      boards: [{ id: "board-1", name: "Primary" }],
      serverDefaultBoardId: "board-1",
    });

    const markup = renderApp(queryClient, transport);

    expect(markup).toContain('class="brand"');
    expect(markup).toContain(">Cardamom</a>");
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

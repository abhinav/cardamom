import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";

import { createWebClient, createWebTransport } from "./api.ts";
import { App } from "./app.tsx";
import "./app.css";

const root = document.getElementById("app");
if (root === null) {
  throw new Error("Cardamom application root is missing");
}

const transport = createWebTransport();
const queryClient = new QueryClient();
const client = createWebClient(transport);

createRoot(root).render(
  <TransportProvider transport={transport}>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App client={client} storage={window.localStorage} />
      </BrowserRouter>
    </QueryClientProvider>
  </TransportProvider>,
);

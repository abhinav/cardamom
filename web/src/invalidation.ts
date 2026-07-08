import type { QueryClient } from "@tanstack/react-query";

import type { ChangeClient } from "./api.ts";
import { WatchResource } from "./gen/cardamom/private/v1/change_pb.ts";
import type { BoardScope } from "./gen/cardamom/private/v1/scope_pb.ts";
import {
  invalidateActiveQueryResources,
  invalidateQueryResources,
} from "./query-runtime.ts";

/** StreamStatus reports connection attempts, active delivery, and retry waits. */
export type StreamStatus = "connecting" | "live" | "offline";

interface WatchChange {
  resources: readonly WatchResource[];
}

/** WatchLoopOptions supplies stream effects so reconnect behavior is deterministic in tests. */
export interface WatchLoopOptions {
  /** signal ends the active stream and any retry wait. */
  signal: AbortSignal;

  /** connect starts one stream attempt. */
  connect: (signal: AbortSignal) => Promise<AsyncIterable<WatchChange>>;

  /** invalidateActive refreshes reads that may have changed while disconnected. */
  invalidateActive: () => Promise<void>;

  /** invalidate refreshes reads named by one stream notification. */
  invalidate: (resources: readonly WatchResource[]) => Promise<void>;

  /** report receives externally visible state transitions in delivery order. */
  report: (status: StreamStatus) => void;

  /** wait delays the next attempt and must resolve when signal is aborted. */
  wait: (signal: AbortSignal) => Promise<void>;
}

/** runWatchLoop reconnects until cancellation and refreshes active reads before Live. */
export async function runWatchLoop(options: WatchLoopOptions): Promise<void> {
  const { signal } = options;
  while (!signal.aborted) {
    options.report("connecting");
    try {
      const changes = await options.connect(signal);
      if (signal.aborted) {
        return;
      }
      await options.invalidateActive();
      options.report("live");
      for await (const change of changes) {
        if (signal.aborted) {
          return;
        }
        await options.invalidate(change.resources);
      }
    } catch {
      if (signal.aborted) {
        return;
      }
    }
    options.report("offline");
    await options.wait(signal);
  }
}

/** watchContinuously binds the generated Connect stream to the retry state machine. */
export function watchContinuously(
  client: ChangeClient,
  scope: BoardScope,
  queryClient: QueryClient,
  report: (status: StreamStatus) => void,
  signal: AbortSignal,
): Promise<void> {
  return runWatchLoop({
    signal,
    connect: async (connectSignal) =>
      client.watchChanges({ scope }, { signal: connectSignal }),
    invalidateActive: () => invalidateActiveQueryResources(queryClient),
    invalidate: (resources) => invalidateQueryResources(queryClient, resources),
    report,
    wait: waitForReconnect,
  });
}

function waitForReconnect(signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const finish = () => {
      window.clearTimeout(timeout);
      signal.removeEventListener("abort", finish);
      resolve();
    };
    const timeout = window.setTimeout(finish, 1_000);
    signal.addEventListener("abort", finish, { once: true });
  });
}

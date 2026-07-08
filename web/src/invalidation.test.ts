import { describe, expect, it } from "vitest";

import { WatchResource } from "./gen/cardamom/private/v1/change_pb.ts";
import { runWatchLoop, type StreamStatus } from "./invalidation.ts";

describe("runWatchLoop", () => {
  it("invalidates active reads before reporting a connection live", async () => {
    const events: string[] = [];
    const controller = new AbortController();
    let connections = 0;
    let waits = 0;

    await runWatchLoop({
      signal: controller.signal,
      connect: async () => {
        connections++;
        return streamOf({
          resources: [
            connections === 1 ? WatchResource.LOG : WatchResource.BOARD,
          ],
        });
      },
      async invalidateActive() {
        events.push("invalidate:active");
      },
      async invalidate(resources) {
        events.push(`invalidate:${resources.join(",")}`);
      },
      report(status: StreamStatus) {
        events.push(`status:${status}`);
      },
      wait: async () => {
        waits++;
        if (waits === 2) {
          controller.abort();
        }
      },
    });

    expect(events).toEqual([
      "status:connecting",
      "invalidate:active",
      "status:live",
      "invalidate:4",
      "status:offline",
      "status:connecting",
      "invalidate:active",
      "status:live",
      "invalidate:2",
      "status:offline",
    ]);
  });

  it("retries after a failed connection and remains abortable", async () => {
    const statuses: StreamStatus[] = [];
    const controller = new AbortController();
    let attempts = 0;

    await runWatchLoop({
      signal: controller.signal,
      async connect() {
        attempts++;
        throw new Error("offline");
      },
      async invalidateActive() {},
      async invalidate() {},
      report(status) {
        statuses.push(status);
      },
      wait: async () => controller.abort(),
    });

    expect(attempts).toBe(1);
    expect(statuses).toEqual(["connecting", "offline"]);
  });
});

async function* streamOf(...changes: Array<{ resources: WatchResource[] }>) {
  for (const change of changes) {
    yield change;
  }
}

import { create } from "@bufbuild/protobuf";

import type { IssueDetail, IssueSummary } from "../gen/cardamom/private/v1/issue_pb.ts";
import {
  IssueLifecycle,
  IssueType,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import {
  MutationContextSchema,
  type MutationContext,
} from "../gen/cardamom/private/v1/mutation_pb.ts";
import type {
  LogEntry,
  StateRecord,
} from "../gen/cardamom/private/v1/record_pb.ts";

/**
 * CheckpointDecisionPresentation is the readiness visible from an issue
 * summary. ResolveCheckpoint remains authoritative when a decision is sent.
 */
export type CheckpointDecisionPresentation =
  | { state: "hidden" }
  | { state: "waiting"; message: string }
  | { state: "ready" };

/** checkpointDecisionPresentation projects checkpoint controls from summary state. */
export function checkpointDecisionPresentation(
  issue: IssueSummary,
): CheckpointDecisionPresentation {
  if (
    issue.type !== IssueType.CHECKPOINT ||
    issue.lifecycle !== IssueLifecycle.OPEN
  ) {
    return { state: "hidden" };
  }
  if (issue.activeClaim !== undefined) {
    return {
      state: "waiting",
      message: `Claimed by ${issue.activeClaim.actor}; checkpoint decisions are unavailable.`,
    };
  }
  if (issue.blocked) {
    return {
      state: "waiting",
      message: "Waiting for dependencies to be completed.",
    };
  }
  if (issue.waiting !== undefined) {
    return {
      state: "waiting",
      message: `Waiting for ${issue.waiting.reason}.`,
    };
  }
  return { state: "ready" };
}

/** mutationContext omits an empty browser-selected actor from generated requests. */
export function mutationContext(actor: string): MutationContext | undefined {
  const normalized = actor.trim();
  return normalized === ""
    ? undefined
    : create(MutationContextSchema, { actor: normalized });
}

/** describeDependencyMutation reports current relationship counts from the server. */
export function describeDependencyMutation(
  issue: Pick<IssueDetail, "prerequisites" | "dependents">,
): string {
  const prerequisites = counted(
    issue.prerequisites.length,
    "prerequisite",
  );
  const dependents = counted(issue.dependents.length, "dependent");
  return `Dependency updated: ${prerequisites} and ${dependents}.`;
}

/** currentIssueState omits mutable State from terminal issue presentation. */
export function currentIssueState(
  issue: Pick<IssueSummary, "lifecycle">,
  state: StateRecord | undefined,
): StateRecord | undefined {
  return issue.lifecycle === IssueLifecycle.OPEN ? state : undefined;
}

/** visibleIssueLogEntries omits State already pinned above the Log. */
export function visibleIssueLogEntries(
  entries: readonly LogEntry[] | undefined,
  state: StateRecord | undefined,
): readonly LogEntry[] | undefined {
  if (entries === undefined || state?.snapshotLogEntryId === undefined) {
    return entries;
  }
  return entries.filter(({ id }) => id !== state.snapshotLogEntryId);
}

/** logEntryPresentation projects one typed Log payload for browser rendering. */
export function logEntryPresentation(entry: LogEntry) {
  switch (entry.payload.case) {
    case "post":
      return {
        actor: entry.payload.value.actor,
        body: entry.payload.value.body,
        createdAt: entry.payload.value.createdAt,
        kind: undefined,
        nextAction: undefined,
      };
    case "stateSnapshot":
      return {
        actor:
          entry.payload.value.author ??
          entry.payload.value.committer ??
          "State",
        body: entry.payload.value.body,
        createdAt: entry.payload.value.createdAt,
        kind: "State snapshot",
        nextAction: entry.payload.value.nextAction,
      };
  }
  throw new Error(`Log entry ${entry.id} has no payload`);
}

function counted(count: number, noun: string): string {
  return `${count} ${count === 1 ? noun : `${noun}s`}`;
}

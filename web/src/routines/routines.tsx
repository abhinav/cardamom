import type {
  DescMessage,
  MessageInitShape,
  MessageShape,
} from "@bufbuild/protobuf";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { useMutation, useTransport } from "@connectrpc/connect-query";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Label } from "@/components/ui/label";

import { issuePath } from "../board-scope.ts";
import { WatchResource } from "../gen/cardamom/private/v1/change_pb.ts";
import { ExecutionService } from "../gen/cardamom/private/v1/execution_pb.ts";
import type {
  ActiveClaim,
  IssueSummary,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import {
  IssueService,
  IssueLifecycle,
  IssueSort,
  IssueStatus,
  IssueType,
  SortDirection,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import type { BoardScope } from "../gen/cardamom/private/v1/scope_pb.ts";
import {
  LifecycleAction,
  lifecycleActionLabel,
  type LifecycleAction as LifecycleActionValue,
} from "../issue-lifecycle.ts";
import { IssueLabel, type SelectLabel } from "../issue-label.tsx";
import {
  runInvalidatingMutation,
  unaryRouteQueryOptions,
} from "../query-runtime.ts";

import "./routines.css";

interface RoutinesRouteProps {
  actor: string;
  canMutateRoutines: boolean;
  requestKey: string;
  scope: BoardScope | undefined;
  selectLabel: SelectLabel;
  showRetired: boolean;
  showScopeMutationNotice: boolean;
  updateShowRetired: (showRetired: boolean) => void;
}

interface RoutineStateSource {
  activeClaim?: Pick<ActiveClaim, "actor">;
  lifecycle: IssueLifecycle;
  status: IssueStatus;
}

interface RoutinePresentation {
  actions: LifecycleActionValue[];
  state:
    | "Available"
    | "Blocked"
    | "Closed"
    | "Cancelled"
    | `Claimed by ${string}`;
}

/** routineListInput makes routine type and retired lifecycle selection explicit. */
export function routineListInput(
  scope: BoardScope,
  showRetired: boolean,
) {
  return {
    scope,
    types: [IssueType.ROUTINE],
    lifecycles: showRetired
      ? [
          IssueLifecycle.OPEN,
          IssueLifecycle.CLOSED,
          IssueLifecycle.CANCELLED,
        ]
      : [IssueLifecycle.OPEN],
    sort: IssueSort.UPDATED_AT,
    direction: SortDirection.DESCENDING,
  };
}

type Mutation<I extends DescMessage, O extends DescMessage> = (
  input: MessageInitShape<I>,
) => Promise<MessageShape<O>>;

/** RoutineMutations provides descriptor-backed routine lifecycle operations. */
interface RoutineMutations {
  cancel: Mutation<
    typeof ExecutionService.method.cancelIssues.input,
    typeof ExecutionService.method.cancelIssues.output
  >;
  claim: Mutation<
    typeof ExecutionService.method.claimIssue.input,
    typeof ExecutionService.method.claimIssue.output
  >;
  close: Mutation<
    typeof ExecutionService.method.closeIssues.input,
    typeof ExecutionService.method.closeIssues.output
  >;
  release: Mutation<
    typeof ExecutionService.method.releaseIssue.input,
    typeof ExecutionService.method.releaseIssue.output
  >;
  reopen: Mutation<
    typeof ExecutionService.method.reopenIssues.input,
    typeof ExecutionService.method.reopenIssues.output
  >;
}

/** changeRoutineLifecycle dispatches one routine control to its product operation. */
export async function changeRoutineLifecycle(
  mutations: RoutineMutations,
  issueId: string,
  actor: string,
  action: LifecycleActionValue,
): Promise<IssueSummary | undefined> {
  const context = { actor: actor.trim() };
  switch (action) {
    case LifecycleAction.CLAIM:
      return (await mutations.claim({ issueId, context })).issue?.issue;
    case LifecycleAction.RELEASE:
      return (await mutations.release({ issueId, context })).issue?.issue;
    case LifecycleAction.CLOSE:
      return (await mutations.close({ issueIds: [issueId], context })).issues[0];
    case LifecycleAction.REOPEN:
      return (await mutations.reopen({ issueIds: [issueId], context }))
        .issues[0]?.issue;
    case LifecycleAction.CANCEL:
      return (await mutations.cancel({ rootIssueIds: [issueId], context }))
        .issues.find(({ id }) => id === issueId);
  }
}

/** routinePresentation derives the operator-facing state and locally knowable actions. */
export function routinePresentation(
  routine: RoutineStateSource,
  actor: string,
  canMutate: boolean,
): RoutinePresentation {
  let state: RoutinePresentation["state"];
  if (routine.lifecycle === IssueLifecycle.CLOSED) {
    state = "Closed";
  } else if (routine.lifecycle === IssueLifecycle.CANCELLED) {
    state = "Cancelled";
  } else if (routine.activeClaim !== undefined) {
    state = `Claimed by ${routine.activeClaim.actor}`;
  } else if (routine.status === IssueStatus.BLOCKED) {
    state = "Blocked";
  } else {
    state = "Available";
  }

  if (!canMutate || actor.trim() === "") {
    return { state, actions: [] };
  }
  if (
    routine.lifecycle === IssueLifecycle.CLOSED ||
    routine.lifecycle === IssueLifecycle.CANCELLED
  ) {
    return { state, actions: [LifecycleAction.REOPEN] };
  }

  const actions: LifecycleActionValue[] = [];
  if (
    routine.activeClaim === undefined &&
    routine.status !== IssueStatus.BLOCKED
  ) {
    actions.push(LifecycleAction.CLAIM);
  } else if (routine.activeClaim?.actor === actor.trim()) {
    actions.push(LifecycleAction.RELEASE);
  }
  actions.push(LifecycleAction.CLOSE, LifecycleAction.CANCEL);
  return { state, actions };
}

export function RoutinesRoute({
  actor,
  canMutateRoutines,
  requestKey,
  scope,
  selectLabel,
  showRetired,
  showScopeMutationNotice,
  updateShowRetired,
}: RoutinesRouteProps) {
  const transport = useTransport();
  const queryClient = useQueryClient();
  const mutations: RoutineMutations = {
    cancel: useMutation(ExecutionService.method.cancelIssues).mutateAsync,
    claim: useMutation(ExecutionService.method.claimIssue).mutateAsync,
    close: useMutation(ExecutionService.method.closeIssues).mutateAsync,
    release: useMutation(ExecutionService.method.releaseIssue).mutateAsync,
    reopen: useMutation(ExecutionService.method.reopenIssues).mutateAsync,
  };
  const [pendingIDs, setPendingIDs] = useState<Set<string>>(() => new Set());
  const [mutationErrors, setMutationErrors] = useState<Record<string, string>>(
    {},
  );
  const [updatedIssues, setUpdatedIssues] = useState<Record<string, IssueSummary>>(
    {},
  );
  const [outcome, setOutcome] = useState("");
  const routines = useQuery({
    ...unaryRouteQueryOptions(
      IssueService.method.listIssues,
      scope === undefined ? undefined : routineListInput(scope, showRetired),
      transport,
    ),
    enabled: scope !== undefined,
  });
  const successfulRoutines = routines.isSuccess ? routines.data : undefined;

  useEffect(() => {
    setUpdatedIssues({});
    setMutationErrors({});
  }, [requestKey, showRetired]);

  useEffect(() => {
    if (successfulRoutines !== undefined) {
      setUpdatedIssues({});
    }
  }, [successfulRoutines]);

  if (scope === undefined) {
    return (
      <RouteState title="Routines">
        <p>Select a board to load routines.</p>
      </RouteState>
    );
  }

  const data = routines.data;
  if (data === undefined && !routines.isError) {
    return <RouteState title="Routines" status="Loading routines" />;
  }
  if (data === undefined && routines.isError) {
    return (
      <RouteState title="Routines" error={routines.error.message}>
        <Button type="button" onClick={() => void routines.refetch()}>
          Retry
        </Button>
      </RouteState>
    );
  }

  const visibleIssues = (data?.issues ?? [])
    .map((issue) => updatedIssues[issue.id] ?? issue)
    .filter(
      (issue) => showRetired || issue.lifecycle === IssueLifecycle.OPEN,
    );
  const actorMissing = actor.trim() === "";

  const mutate = async (
    issue: IssueSummary,
    action: LifecycleActionValue,
  ) => {
    setPendingIDs((current) => new Set(current).add(issue.id));
    setMutationErrors((current) => {
      const next = { ...current };
      delete next[issue.id];
      return next;
    });
    setOutcome("");
    try {
      const updatedIssue = await runInvalidatingMutation(
        queryClient,
        [WatchResource.ISSUES, WatchResource.APPROVALS],
        () => changeRoutineLifecycle(mutations, issue.id, actor, action),
      );
      if (updatedIssue !== undefined) {
        setUpdatedIssues((current) => ({
          ...current,
          [issue.id]: updatedIssue,
        }));
      }
      setOutcome(`${issue.title}: ${actionOutcome(action)}.`);
    } catch (failure) {
      const message = failure instanceof Error ? failure.message : String(failure);
      setMutationErrors((current) => ({ ...current, [issue.id]: message }));
    } finally {
      setPendingIDs((current) => {
        const next = new Set(current);
        next.delete(issue.id);
        return next;
      });
    }
  };

  return (
    <section className="routines-route" aria-labelledby="routines-title">
      <header className="route-heading routines-heading">
        <div>
          <h1 id="routines-title">Routines</h1>
          <p>
            {visibleIssues.length}{" "}
            {visibleIssues.length === 1 ? "routine" : "routines"} shown
          </p>
        </div>
        <div className="retired-toggle">
          <Checkbox
            id="show-retired-routines"
            checked={showRetired}
            onCheckedChange={updateShowRetired}
          />
          <Label htmlFor="show-retired-routines">Show retired</Label>
        </div>
      </header>

      {showScopeMutationNotice && (
        <Notice>
          All boards is read-only. Select one board to change a routine.
        </Notice>
      )}
      {canMutateRoutines && actorMissing && (
        <Notice>Set an actor in Settings before changing a routine.</Notice>
      )}
      {routines.isError && data !== undefined && (
        <div className="routines-error" role="alert">
          <span>Could not refresh routines: {routines.error.message}</span>
          <Button type="button" variant="outline" onClick={() => void routines.refetch()}>
            Retry
          </Button>
        </div>
      )}
      {outcome !== "" && (
        <p className="routines-outcome" role="status" aria-live="polite">
          {outcome}
        </p>
      )}
      {data?.truncated === true && (
        <Notice>
          The server truncated this routine list. Narrow the board scope to see
          a complete result.
        </Notice>
      )}

      {visibleIssues.length === 0 ? (
        <div className="routines-empty">
          <h2>No routines to show</h2>
          <p>
            {showRetired
              ? "No routines exist in this scope."
              : "There are no active routines. Show retired to include closed " +
                "and cancelled routines."}
          </p>
        </div>
      ) : (
        <div className="routines-grid">
          {visibleIssues.map((issue) => (
            <RoutineCard
              key={issue.id}
              actor={actor}
              canMutate={canMutateRoutines}
              error={mutationErrors[issue.id]}
              issue={issue}
              mutate={mutate}
              pending={pendingIDs.has(issue.id)}
              selectLabel={selectLabel}
            />
          ))}
        </div>
      )}
    </section>
  );
}

function RoutineCard({
  actor,
  canMutate,
  error,
  issue,
  mutate,
  pending,
  selectLabel,
}: {
  actor: string;
  canMutate: boolean;
  error: string | undefined;
  issue: IssueSummary;
  mutate: (issue: IssueSummary, action: LifecycleActionValue) => Promise<void>;
  pending: boolean;
  selectLabel: SelectLabel;
}) {
  const presentation = routinePresentation(issue, actor, canMutate);
  const stateKind = routineStateKind(issue);
  const updatedDate = timestampDate(issue.updatedAt);

  return (
    <article className="routine-card" aria-labelledby={`routine-${issue.id}`}>
      <header className="routine-card-heading">
        <div className="routine-card-title">
          <h2>
            <a id={`routine-${issue.id}`} href={issuePath(issue.boardId, issue.id)}>
              {issue.title}
            </a>
          </h2>
          <p>{issue.id}</p>
        </div>
        <div className="routine-observation">
          <span className="state-badge" data-routine-state={stateKind}>
            {presentation.state}
          </span>
          <time
            className="routine-updated"
            dateTime={updatedDate?.toISOString()}
            title={formatTimestamp(issue.updatedAt)}
          >
            Updated {formatRecency(issue.updatedAt)}
          </time>
        </div>
      </header>
      {issue.labels.length > 0 && (
        <ul className="routine-labels" aria-label="Labels">
          {issue.labels.map((label) => (
            <li key={label}>
              <IssueLabel label={label} select={selectLabel} />
            </li>
          ))}
        </ul>
      )}
      {error !== undefined && (
        <p className="routine-mutation-error" role="alert">
          Change failed: {error}
        </p>
      )}
      {presentation.actions.length > 0 && (
        <Collapsible className="routine-actions">
          <CollapsibleTrigger render={<Button variant="outline" size="sm" />}>
            Actions
          </CollapsibleTrigger>
          <CollapsibleContent className="routine-action-menu">
            {presentation.actions.map((action) => (
              <Button
                key={action}
                variant={
                  action === LifecycleAction.CANCEL ? "destructive" : "default"
                }
                type="button"
                disabled={pending}
                onClick={() => void mutate(issue, action)}
              >
                {pending ? "Working..." : lifecycleActionLabel(action)}
              </Button>
            ))}
          </CollapsibleContent>
        </Collapsible>
      )}
    </article>
  );
}

function Notice({ children }: { children: ReactNode }) {
  return (
    <p className="routines-notice" role="note">
      {children}
    </p>
  );
}

function RouteState({
  children,
  error,
  status,
  title,
}: {
  children?: ReactNode;
  error?: string;
  status?: string;
  title: string;
}) {
  return (
    <section className="routines-route" aria-labelledby="routines-title">
      <h1 id="routines-title">{title}</h1>
      {status !== undefined && <p role="status">{status}</p>}
      {error !== undefined && <p role="alert">Could not load routines: {error}</p>}
      {children}
    </section>
  );
}

function routineStateKind(issue: IssueSummary): string {
  if (issue.lifecycle === IssueLifecycle.CLOSED) {
    return "closed";
  }
  if (issue.lifecycle === IssueLifecycle.CANCELLED) {
    return "cancelled";
  }
  if (issue.activeClaim !== undefined) {
    return "claimed";
  }
  return issue.status === IssueStatus.BLOCKED ? "blocked" : "available";
}

function actionOutcome(action: LifecycleActionValue): string {
  switch (action) {
    case LifecycleAction.CLAIM:
      return "claimed";
    case LifecycleAction.RELEASE:
      return "released";
    case LifecycleAction.CLOSE:
      return "closed";
    case LifecycleAction.REOPEN:
      return "reopened";
    case LifecycleAction.CANCEL:
      return "cancelled";
  }
}

function formatTimestamp(timestamp: Timestamp | undefined): string {
  const date = timestampDate(timestamp);
  if (date === undefined) {
    return "Unknown";
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

export function formatRecency(
  timestamp: Timestamp | undefined,
  now = Date.now(),
): string {
  const date = timestampDate(timestamp);
  if (date === undefined) {
    return "unknown";
  }

  const elapsed = date.getTime() - now;
  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ["year", 365 * 24 * 60 * 60 * 1_000],
    ["month", 30 * 24 * 60 * 60 * 1_000],
    ["week", 7 * 24 * 60 * 60 * 1_000],
    ["day", 24 * 60 * 60 * 1_000],
    ["hour", 60 * 60 * 1_000],
    ["minute", 60 * 1_000],
  ];
  const [unit, milliseconds] =
    units.find(([, size]) => Math.abs(elapsed) >= size) ?? ["second", 1_000];

  return new Intl.RelativeTimeFormat(undefined, { numeric: "always" }).format(
    Math.round(elapsed / milliseconds),
    unit,
  );
}

function timestampDate(timestamp: Timestamp | undefined): Date | undefined {
  if (timestamp === undefined) {
    return undefined;
  }
  const milliseconds = Number(timestamp.seconds) * 1_000 + timestamp.nanos / 1e6;
  return new Date(milliseconds);
}

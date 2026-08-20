import { useMutation, useTransport } from "@connectrpc/connect-query";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { Link } from "react-router";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

import { issuePath } from "../board-scope.ts";
import type {
  ActionableCheckpoint,
  ResolveCheckpointResponse,
} from "../gen/cardamom/private/v1/checkpoint_pb.ts";
import { CheckpointService } from "../gen/cardamom/private/v1/checkpoint_pb.ts";
import { WatchResource } from "../gen/cardamom/private/v1/change_pb.ts";
import type { MarkdownContent } from "../gen/cardamom/private/v1/content_pb.ts";
import {
  CheckpointOutcome,
  IssueLifecycle,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import type { BoardScope } from "../gen/cardamom/private/v1/scope_pb.ts";
import {
  runInvalidatingMutation,
  unaryRouteQueryOptions,
} from "../query-runtime.ts";

import "./approvals.css";

interface ApprovalsRouteProps {
  actor: string;
  canResolveCheckpoints: boolean;
  requestKey: string;
  scope: BoardScope | undefined;
  showDecisionControls: boolean;
  showScopeMutationNotice: boolean;
}

/** resolveActionableCheckpoint normalizes browser attribution at the RPC boundary. */
export function resolveActionableCheckpoint(
  issueId: string,
  actor: string,
  outcome: CheckpointOutcome,
  reason: string,
) {
  const normalizedReason = reason.trim();
  return {
    issueId,
    context: { actor: actor.trim() },
    outcome,
    ...(normalizedReason === "" ? {} : { reason: normalizedReason }),
  };
}

export function ApprovalsRoute({
  actor,
  canResolveCheckpoints,
  requestKey,
  scope,
  showDecisionControls,
  showScopeMutationNotice,
}: ApprovalsRouteProps) {
  const transport = useTransport();
  const queryClient = useQueryClient();
  const resolveCheckpoint = useMutation(
    CheckpointService.method.resolveCheckpoint,
  );
  const [pendingIDs, setPendingIDs] = useState<Set<string>>(() => new Set());
  const [mutationErrors, setMutationErrors] = useState<Record<string, string>>(
    {},
  );
  const [resolvedIDs, setResolvedIDs] = useState<Set<string>>(() => new Set());
  const [outcome, setOutcome] = useState("");
  const approvals = useQuery({
    ...unaryRouteQueryOptions(
      CheckpointService.method.listActionableCheckpoints,
      { scope },
      transport,
    ),
    enabled: scope !== undefined,
  });
  const successfulApprovals = approvals.isSuccess ? approvals.data : undefined;

  useEffect(() => {
    setResolvedIDs(new Set());
    setMutationErrors({});
  }, [requestKey]);

  useEffect(() => {
    if (successfulApprovals === undefined) {
      return;
    }
    const returnedIDs = new Set(
      successfulApprovals.checkpoints.map(
        ({ checkpoint }) => checkpoint?.id ?? "",
      ),
    );
    setResolvedIDs((current) => {
      const next = new Set([...current].filter((id) => returnedIDs.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [successfulApprovals]);

  if (scope === undefined) {
    return (
      <RouteState title="Approvals">
        <p>Select a board to load actionable checkpoints.</p>
      </RouteState>
    );
  }

  const data = approvals.data;
  if (data === undefined && !approvals.isError) {
    return <RouteState title="Approvals" status="Loading approvals" />;
  }
  if (data === undefined && approvals.isError) {
    return (
      <RouteState title="Approvals" error={approvals.error.message}>
        <Button type="button" onClick={() => void approvals.refetch()}>
          Retry
        </Button>
      </RouteState>
    );
  }

  const checkpoints = (data?.checkpoints ?? []).filter(
    ({ checkpoint }) => !resolvedIDs.has(checkpoint?.id ?? ""),
  );
  const actorMissing = actor.trim() === "";

  const resolve = async (
    checkpoint: ActionableCheckpoint,
    outcome: CheckpointOutcome,
    reason: string,
  ) => {
    const issueID = checkpoint.checkpoint?.id;
    if (issueID === undefined || issueID === "") {
      return;
    }
    setPendingIDs((current) => new Set(current).add(issueID));
    setMutationErrors((current) => {
      const next = { ...current };
      delete next[issueID];
      return next;
    });
    setOutcome("");
    try {
      const result = await runInvalidatingMutation(
        queryClient,
        [WatchResource.ISSUES, WatchResource.APPROVALS],
        () =>
          resolveCheckpoint.mutateAsync(
            resolveActionableCheckpoint(issueID, actor, outcome, reason),
          ),
      );
      setResolvedIDs((current) => new Set(current).add(issueID));
      setOutcome(checkpointOutcome(result));
    } catch (failure) {
      const message = failure instanceof Error ? failure.message : String(failure);
      setMutationErrors((current) => ({ ...current, [issueID]: message }));
    } finally {
      setPendingIDs((current) => {
        const next = new Set(current);
        next.delete(issueID);
        return next;
      });
    }
  };

  return (
    <section className="approvals-route" aria-label="Approvals">
      <p className="approvals-summary">
        {checkpoints.length}{" "}
        {checkpoints.length === 1 ? "checkpoint" : "checkpoints"}{" "}
        awaiting decision
      </p>

      {showScopeMutationNotice && (
        <Notice>
          All boards is read-only. Select one board to approve or deny a checkpoint.
        </Notice>
      )}
      {canResolveCheckpoints && actorMissing && (
        <Notice>Set an actor in Settings before recording a decision.</Notice>
      )}
      {approvals.isError && data !== undefined && (
        <div className="approvals-error" role="alert">
          <span>Could not refresh approvals: {approvals.error.message}</span>
          <Button type="button" variant="outline" onClick={() => void approvals.refetch()}>
            Retry
          </Button>
        </div>
      )}
      {outcome !== "" && (
        <p className="approvals-outcome" role="status" aria-live="polite">
          {outcome}
        </p>
      )}

      {checkpoints.length === 0 ? (
        <div className="approvals-empty">
          <h2>No decisions waiting</h2>
          <p>There are no ready checkpoints waiting for a decision.</p>
        </div>
      ) : (
        <div className="approvals-list">
          {checkpoints.map((checkpoint) => {
            const issueID = checkpoint.checkpoint?.id ?? "";
            return (
              <ApprovalCard
                key={issueID}
                checkpoint={checkpoint}
                disabled={!canResolveCheckpoints || actorMissing}
                error={mutationErrors[issueID]}
                pending={pendingIDs.has(issueID)}
                resolve={resolve}
                showDecisionControls={showDecisionControls}
              />
            );
          })}
        </div>
      )}
    </section>
  );
}

interface ApprovalCardProps {
  checkpoint: ActionableCheckpoint;
  disabled: boolean;
  error: string | undefined;
  pending: boolean;
  resolve: (
    checkpoint: ActionableCheckpoint,
    outcome: CheckpointOutcome,
    reason: string,
  ) => Promise<void>;
  showDecisionControls: boolean;
}

interface ApprovalPresentation {
  description: MarkdownContent | undefined;
  issueHref: string;
  issueID: string;
  readiness: "Ready";
  reasonID: string;
  title: string;
}

/** approvalPresentation selects the checkpoint data shown to human approvers. */
export function approvalPresentation(
  checkpoint: ActionableCheckpoint,
): ApprovalPresentation | undefined {
  const summary = checkpoint.checkpoint;
  if (summary === undefined) {
    return undefined;
  }
  return {
    description: checkpoint.summary,
    issueHref: issuePath(summary.boardId, summary.id, summary.source),
    issueID: summary.id,
    readiness: "Ready",
    reasonID: `approval-reason-${summary.id}`,
    title: summary.title,
  };
}

function ApprovalCard({
  checkpoint,
  disabled,
  error,
  pending,
  resolve,
  showDecisionControls,
}: ApprovalCardProps) {
  const [reason, setReason] = useState("");
  const presentation = approvalPresentation(checkpoint);
  if (presentation === undefined) {
    return null;
  }

  return (
    <article
      className="approval-card"
      aria-labelledby={`approval-${presentation.issueID}`}
    >
      <header className="approval-card-heading">
        <div>
          <p className="approval-issue-id">{presentation.issueID}</p>
          <h2 id={`approval-${presentation.issueID}`}>
            <Link to={presentation.issueHref}>
              {presentation.title}
            </Link>
          </h2>
        </div>
        <span className="state-badge" data-approval-state="ready">
          {presentation.readiness}
        </span>
      </header>

      <Markdown content={presentation.description} />

      {showDecisionControls && (
        <div className="approval-decision">
          <Label htmlFor={presentation.reasonID}>Reason (optional)</Label>
          <Textarea
            id={presentation.reasonID}
            value={reason}
            rows={2}
            disabled={disabled || pending}
            onChange={(event) => setReason(event.currentTarget.value)}
          />
          {error !== undefined && (
            <p className="approval-mutation-error" role="alert">
              Decision failed: {error}
            </p>
          )}
          <div className="approval-actions">
            <Button
              type="button"
              disabled={disabled || pending}
              onClick={() => void resolve(checkpoint, CheckpointOutcome.APPROVED, reason)}
            >
              {pending ? "Recording..." : "Approve"}
            </Button>
            <Button
              variant="destructive"
              type="button"
              disabled={disabled || pending}
              onClick={() => void resolve(checkpoint, CheckpointOutcome.DENIED, reason)}
            >
              Deny
            </Button>
          </div>
        </div>
      )}
    </article>
  );
}

/** Markdown renders only the sanitized HTML supplied by the generated protocol. */
function Markdown({ content }: { content: MarkdownContent | undefined }) {
  if (content === undefined || content.renderedHtml === "") {
    return null;
  }
  return (
    <div
      className="approval-markdown"
      dangerouslySetInnerHTML={{ __html: content.renderedHtml }}
    />
  );
}

function Notice({ children }: { children: ReactNode }) {
  return (
    <p className="approvals-notice" role="note">
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
    <section className="approvals-route" aria-label={title}>
      {status !== undefined && <p role="status">{status}</p>}
      {error !== undefined && <p role="alert">Could not load approvals: {error}</p>}
      {children}
    </section>
  );
}

export function checkpointOutcome(result: ResolveCheckpointResponse): string {
  const title = result.checkpoint?.title ?? "Checkpoint";
  const decision = result.decision;
  if (decision === undefined) {
    return `${title} decision recorded.`;
  }
  const outcome = decision.outcome === CheckpointOutcome.APPROVED
    ? "approved"
    : "denied";
  const reason = decision.reason?.source.trim();
  const decidedAt = formatTimestamp(decision.decidedAt);
  const lifecycle = enumLabel(IssueLifecycle[result.checkpoint?.lifecycle ?? 0]);
  const details = [
    `${title} ${outcome}.`,
    ...(reason === undefined || reason === "" ? [] : [`Reason: ${reason}`]),
    `Decided ${decidedAt}.`,
    `Lifecycle: ${lifecycle}.`,
  ];
  const count = result.cancelledDependents.length;
  if (decision.outcome === CheckpointOutcome.DENIED && count > 0) {
    details.push(
      `${count} dependent ${count === 1 ? "issue was" : "issues were"} cancelled.`,
    );
  }
  return details.join(" ");
}

function formatTimestamp(
  timestamp: Timestamp | undefined,
): string {
  if (timestamp === undefined) {
    return "at an unrecorded time";
  }
  const date = new Date(Number(timestamp.seconds) * 1_000 + timestamp.nanos / 1e6);
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function enumLabel(value: string | undefined): string {
  if (value === undefined || value === "UNSPECIFIED") {
    return "Unknown";
  }
  return value
    .toLowerCase()
    .split("_")
    .map((part) => part[0]?.toUpperCase() + part.slice(1))
    .join(" ");
}

import {
  create,
  type DescMessage,
  type MessageInitShape,
  type MessageShape,
} from "@bufbuild/protobuf";
import { useMutation, useTransport } from "@connectrpc/connect-query";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Pencil, Pin, PinOff } from "lucide-react";
import type { FormEvent } from "react";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { createRoot } from "react-dom/client";
import { Link } from "react-router";

import type {
  AttachmentClient,
} from "../api.ts";
import { issuePath } from "../board-scope.ts";
import {
  AttachmentRecords,
  AttachmentUploadPanel,
} from "../attachments.tsx";
import { ClipboardPill } from "../clipboard-pill.tsx";
import { WatchResource } from "../gen/cardamom/private/v1/change_pb.ts";
import { CheckpointService } from "../gen/cardamom/private/v1/checkpoint_pb.ts";
import type { MarkdownContent } from "../gen/cardamom/private/v1/content_pb.ts";
import { ExecutionService } from "../gen/cardamom/private/v1/execution_pb.ts";
import type {
  AncestorContext,
  IssueDetail,
  RelatedIssue,
  IssueSummary,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import type { BoardSummary, Project } from "../gen/cardamom/private/v1/project_pb.ts";
import {
  CheckpointOutcome,
  IssueSort,
  IssueLifecycle,
  IssueService,
  SortDirection,
  ListIssuesRequestSchema,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import { PlanningService } from "../gen/cardamom/private/v1/planning_pb.ts";
import {
  RecordService,
  type LogEntry,
  type StateRecord,
} from "../gen/cardamom/private/v1/record_pb.ts";
import { BoardScopeSchema } from "../gen/cardamom/private/v1/scope_pb.ts";
import type { SourceRef } from "../gen/cardamom/private/v1/source_pb.ts";
import { issueTypeLabel } from "../issue-collection.ts";
import { IssueStatusBadge } from "../issue-status.tsx";
import { IssueWaitingReason } from "../issue-waiting.tsx";
import { IssueLabel, type SelectLabel } from "../issue-label.tsx";
import {
  IssueReferencePill,
  type LoadIssueReferencePreview,
} from "../issue-reference-pill.tsx";
import {
  IssueMetadataFields,
  metadataLabels,
  type IssueMetadataDraft,
} from "../issue-metadata-form.tsx";
import { IssueFormDialog } from "../issue-form-dialog.tsx";
import {
  availableLifecycleActions,
  describeLifecycleMutation,
  LifecycleAction,
  lifecycleActionLabel,
  type LifecycleAction as LifecycleActionValue,
} from "../issue-lifecycle.ts";
import {
  runInvalidatingMutation,
  unaryRouteQueryOptions,
} from "../query-runtime.ts";
import { useServerAccess } from "../server-access.tsx";
import { issueProvenance } from "../provenance.ts";
import { IssueHierarchy } from "./hierarchy.tsx";
import { IssueReferenceLink } from "./issue-reference.tsx";
import {
  checkpointDecisionPresentation,
  currentIssueState,
  describeDependencyMutation,
  logEntryPresentation,
  mutationContext,
  visibleIssueLogEntries,
} from "./model.ts";

import "./issue-detail.css";

const IssueReferencePreviewLoaderContext =
  createContext<LoadIssueReferencePreview | undefined>(undefined);

interface IssueDetailPageProps {
  actor: string;
  attachmentClient: AttachmentClient;
  source?: SourceRef;
  boards?: readonly BoardSummary[];
  collapsedDetailsBoardIds: readonly string[];
  expectedBoardId: string;
  issueId: string;
  projects?: readonly Project[];
  readOnly?: boolean;
  relationsOpen: boolean;
  selectLabel: SelectLabel;
  setDetailsCollapsed: (boardId: string, collapsed: boolean) => void;
  setRelationsOpen: (open: boolean) => void;
}

/** dependencySearchInput selects title matches from one board. */
export function dependencySearchInput(
  boardId: string,
  query: string,
  source?: SourceRef,
) {
  return create(ListIssuesRequestSchema, {
      scope: create(BoardScopeSchema, {
        ...(source === undefined ? {} : { source }),
        selection: { case: "boardId", value: boardId },
      }),
      titleQuery: query.trim(),
      sort: IssueSort.TITLE,
      direction: SortDirection.ASCENDING,
      limit: 8,
    });
}

/** dependencyCandidates removes the current issue and existing relationships. */
export function dependencyCandidates(
  issues: readonly IssueSummary[],
  excludedIssueIds: ReadonlySet<string>,
): IssueSummary[] {
  return issues.filter(({ id }) => !excludedIssueIds.has(id));
}

/** resolveIssueCheckpoint normalizes browser attribution at the domain RPC. */
export function resolveIssueCheckpointInput(
  issueId: string,
  actor: string,
  outcome: CheckpointOutcome,
  reason = "",
) {
  const normalizedReason = reason.trim();
  return {
    issueId,
    context: { actor: actor.trim() },
    outcome,
    ...(normalizedReason === "" ? {} : { reason: normalizedReason }),
  };
}

type Mutation<I extends DescMessage, O extends DescMessage> = (
  input: MessageInitShape<I>,
) => Promise<MessageShape<O>>;

/** LifecycleMutations provides descriptor-backed issue lifecycle operations. */
interface LifecycleMutations {
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

interface PinMutations {
  pin: Mutation<
    typeof IssueService.method.pinBoardIssue.input,
    typeof IssueService.method.pinBoardIssue.output
  >;
  unpin: Mutation<
    typeof IssueService.method.unpinBoardIssue.input,
    typeof IssueService.method.unpinBoardIssue.output
  >;
}

/** changeIssueLifecycle dispatches one browser control to its product operation. */
export async function changeIssueLifecycle(
  mutations: LifecycleMutations,
  issueId: string,
  actor: string,
  action: LifecycleActionValue,
): Promise<string> {
  const context = mutationContext(actor);
  switch (action) {
    case LifecycleAction.CLAIM: {
      const response = await mutations.claim({ issueId, context });
      return describeLifecycleMutation(
        action,
        response.issue?.issue?.id ?? issueId,
        0,
      );
    }
    case LifecycleAction.RELEASE: {
      const response = await mutations.release({ issueId, context });
      return describeLifecycleMutation(
        action,
        response.issue?.issue?.id ?? issueId,
        0,
      );
    }
    case LifecycleAction.CLOSE: {
      const response = await mutations.close({ issueIds: [issueId], context });
      return describeLifecycleMutation(
        action,
        response.issues[0]?.id ?? issueId,
        0,
      );
    }
    case LifecycleAction.REOPEN: {
      const response = await mutations.reopen({ issueIds: [issueId], context });
      return describeLifecycleMutation(
        action,
        response.issues[0]?.issue?.id ?? issueId,
        0,
      );
    }
    case LifecycleAction.CANCEL: {
      const response = await mutations.cancel({
        rootIssueIds: [issueId],
        context,
      });
      const primary = response.issues.find(({ id }) => id === issueId);
      return describeLifecycleMutation(
        action,
        primary?.id ?? issueId,
        Number(response.dependents),
      );
    }
  }
}

/** changeIssuePin applies one actor-attributed board pin mutation. */
export async function changeIssuePin(
  mutations: PinMutations,
  issueId: string,
  actor: string,
  pinned: boolean,
): Promise<string> {
  const context = mutationContext(actor);
  if (pinned) {
    const response = await mutations.unpin({ issueId, context });
    const id = response.issue?.id ?? issueId;
    return response.changed ? `Unpinned ${id}.` : `${id} was already unpinned.`;
  }
  const response = await mutations.pin({ issueId, context });
  const id = response.issue?.id ?? issueId;
  return response.changed ? `Pinned ${id}.` : `${id} was already pinned.`;
}

/** DependencyEdit selects one atomic planning relationship edit. */
export type DependencyEdit = "add" | "remove";

/** editIssueDependencyInput selects one prerequisite change. */
export function editIssueDependencyInput(
  issueId: string,
  prerequisiteId: string,
  actor: string,
  edit: DependencyEdit,
) {
  return {
    issueId,
    context: mutationContext(actor),
    ...(edit === "add"
      ? { addPrerequisiteIds: [prerequisiteId] }
      : { removePrerequisiteIds: [prerequisiteId] }),
  };
}

/** issueMetadataDraft prefills the shared metadata form from an issue detail. */
export function issueMetadataDraft(detail: IssueDetail): IssueMetadataDraft {
  const issue = detail.issue;
  if (issue === undefined) {
    throw new Error("Issue detail does not include its primary record");
  }
  const selectedNode = detail.containment?.nodes.find(
    (node) => node.issue?.id === issue.id,
  );
  return {
    title: issue.title,
    type: issue.type,
    priority: issue.priority,
    summary: detail.summary?.source ?? "",
    details: detail.details?.source ?? "",
    labels: issue.labels.join(", "),
    parent: selectedNode?.parentId ?? "",
  };
}

/** editIssueMetadataInput replaces the complete metadata form in one request. */
export function editIssueMetadataInput(
  issueId: string,
  actor: string,
  draft: IssueMetadataDraft,
) {
  return {
    issueId,
    title: draft.title.trim(),
    type: draft.type,
    priority: draft.priority,
    summarySource: draft.summary,
    detailsSource: draft.details,
    labels: { values: metadataLabels(draft.labels) },
    parentId: draft.parent.trim(),
    context: mutationContext(actor),
  };
}

/** addIssueLogEntryInput attributes one immutable record at the RPC boundary. */
export function addIssueLogEntryInput(
  issueId: string,
  actor: string,
  bodySource: string,
) {
  return {
    issueId,
    bodySource,
    context: mutationContext(actor),
  };
}

/** IssueDetailPage owns the independently refreshed issue and log records. */
export function IssueDetailPage({
  actor,
  attachmentClient,
  source,
  boards = [],
  collapsedDetailsBoardIds,
  expectedBoardId,
  issueId,
  projects = [],
  readOnly = false,
  relationsOpen,
  selectLabel,
  setDetailsCollapsed,
  setRelationsOpen,
}: IssueDetailPageProps) {
  const { canMutateServer: serverCanMutate } = useServerAccess();
  const canMutateServer = serverCanMutate && !readOnly;
  const transport = useTransport();
  const queryClient = useQueryClient();
  const lifecycleMutations: LifecycleMutations = {
    cancel: useMutation(ExecutionService.method.cancelIssues).mutateAsync,
    claim: useMutation(ExecutionService.method.claimIssue).mutateAsync,
    close: useMutation(ExecutionService.method.closeIssues).mutateAsync,
    release: useMutation(ExecutionService.method.releaseIssue).mutateAsync,
    reopen: useMutation(ExecutionService.method.reopenIssues).mutateAsync,
  };
  const pinMutations: PinMutations = {
    pin: useMutation(IssueService.method.pinBoardIssue).mutateAsync,
    unpin: useMutation(IssueService.method.unpinBoardIssue).mutateAsync,
  };
  const resolveCheckpoint = useMutation(
    CheckpointService.method.resolveCheckpoint,
  );
  const editIssue = useMutation(PlanningService.method.editIssue);
  const addLogEntryMutation = useMutation(RecordService.method.addLogEntry);
  const [mutation, setMutation] = useState<MutationState>({ status: "idle" });
  const [logBody, setLogBody] = useState("");
  const [dependencyQuery, setDependencyQuery] = useState("");
  const [checkpointReason, setCheckpointReason] = useState("");
  const [metadataEditor, setMetadataEditor] = useState<IssueMetadataDraft>();

  const issueRequest = useQuery({
    ...unaryRouteQueryOptions(
      IssueService.method.getIssue,
      {
        issueId,
        ...(source === undefined
          ? {}
          : { source, boardId: expectedBoardId }),
      },
      transport,
    ),
    select(response) {
      if (response.issue === undefined) {
        throw new Error(`Issue ${issueId} was not returned by the server.`);
      }
      if (
        response.issue.issue !== undefined &&
        response.issue.issue.boardId !== expectedBoardId
      ) {
        throw new Error(
          `Issue ${issueId} was not found in board ${expectedBoardId}.`,
        );
      }
      return response.issue;
    },
  });
  const logRequest = useQuery({
    ...unaryRouteQueryOptions(
      RecordService.method.listLogEntries,
      {
        issueId,
        direction: SortDirection.ASCENDING,
        ...(source === undefined
          ? {}
          : { source, boardId: expectedBoardId }),
      },
      transport,
    ),
    select: (response) => response.logEntries,
  });
  const stateRequest = useQuery({
    ...unaryRouteQueryOptions(
      RecordService.method.getState,
      {
        issueId,
        ...(source === undefined
          ? {}
          : { source, boardId: expectedBoardId }),
      },
      transport,
    ),
    select: (response) => response.state,
  });
  const loadIssueReferencePreview = useCallback<LoadIssueReferencePreview>(
    async (referencedIssueID) => {
      const response = await queryClient.fetchQuery(
        unaryRouteQueryOptions(
          IssueService.method.getIssue,
          {
            issueId: referencedIssueID,
            ...(source === undefined
              ? {}
              : {
                  source,
                  boardId: expectedBoardId,
                }),
          },
          transport,
        ),
      );
      const referencedIssue = response.issue?.issue;
      if (referencedIssue === undefined) {
        throw new Error(
          `Issue ${referencedIssueID} was not returned by the server.`,
        );
      }
      return referencedIssue;
    },
    [expectedBoardId, queryClient, source, transport],
  );
  const detail = requestData(issueRequest);

  if (detail === undefined) {
    return (
      <InitialRequestState
        request={issueRequest}
        loadingMessage={`Loading ${issueId}`}
        retry={() => void issueRequest.refetch()}
      />
    );
  }
  const issue = detail.issue;
  if (issue === undefined) {
    return (
      <section className="issue-detail-state" role="alert">
        <h1>Issue data is incomplete</h1>
        <p>The server did not return the primary record for {issueId}.</p>
      </section>
    );
  }

  const runMutation = async (
    resources: readonly WatchResource[],
    operation: () => Promise<string>,
  ) => {
    setMutation({ status: "pending" });
    try {
      const message = await runInvalidatingMutation(
        queryClient,
        resources,
        operation,
      );
      setMutation({ status: "success", message });
      return true;
    } catch (failure) {
      setMutation({ status: "error", message: failureMessage(failure) });
      return false;
    }
  };
  const changeLifecycle = (action: LifecycleActionValue) =>
    runMutation([WatchResource.ISSUES, WatchResource.APPROVALS], () =>
      changeIssueLifecycle(lifecycleMutations, issueId, actor, action),
    );
  const pinned = detail.context?.pins.some((pin) => pin.id === issueId) ?? false;
  const changePin = () =>
    runMutation([WatchResource.ISSUES], () =>
      changeIssuePin(pinMutations, issueId, actor, pinned),
    );
  const changeDependency = (
    prerequisiteId: string,
    edit: DependencyEdit,
  ) =>
    runMutation([WatchResource.ISSUES, WatchResource.APPROVALS], async () => {
      const response = await editIssue.mutateAsync(
        editIssueDependencyInput(
          issueId,
          prerequisiteId,
          actor,
          edit,
        ),
      );
      if (response.issue === undefined) {
        throw new Error("EditIssue response did not include the edited issue");
      }
      if (edit === "add") {
        setDependencyQuery("");
      }
      return describeDependencyMutation(response.issue);
    });
  const decideCheckpoint = (outcome: CheckpointOutcome) =>
    runMutation([WatchResource.ISSUES, WatchResource.APPROVALS], async () => {
      const response = await resolveCheckpoint.mutateAsync(
        resolveIssueCheckpointInput(
          issueId,
          actor,
          outcome,
          checkpointReason,
        ),
      );
      setCheckpointReason("");
      if (response.decision === undefined) {
        throw new Error("ResolveCheckpoint response did not include the decision");
      }
      return describeCheckpointDecision(
        response.decision.outcome,
        response.cancelledDependents.length,
      );
    });
  const addLogEntry = () =>
    runMutation([WatchResource.ISSUES, WatchResource.LOG], async () => {
      await addLogEntryMutation.mutateAsync(
        addIssueLogEntryInput(issueId, actor, logBody),
      );
      setLogBody("");
      return "Log entry added.";
    });
  const saveMetadata = async () => {
    if (metadataEditor === undefined || actor.trim() === "") {
      return;
    }
    const saved = await runMutation(
      [WatchResource.ISSUES, WatchResource.APPROVALS],
      async () => {
        const response = await editIssue.mutateAsync(
          editIssueMetadataInput(issueId, actor, metadataEditor),
        );
        const editedIssue = response.issue?.issue;
        if (editedIssue === undefined) {
          throw new Error("EditIssue response did not include the edited issue");
        }
        return `Updated ${editedIssue.id}.`;
      },
    );
    if (saved) {
      setMetadataEditor(undefined);
    }
  };
  const openMetadataEditor = () => {
    setMutation({ status: "idle" });
    setMetadataEditor(issueMetadataDraft(detail));
  };
  const state = currentIssueState(
    issue,
    requestData(stateRequest),
  );
  const logEntries = visibleIssueLogEntries(requestData(logRequest), state);

  return (
    <IssueReferencePreviewLoaderContext.Provider
      value={loadIssueReferencePreview}
    >
      <article className="issue-detail-page" aria-labelledby="issue-title">
        <RequestRefreshError request={issueRequest} recordName="issue" />
        <RequestRefreshError request={stateRequest} recordName="state" />
        <IssueHeader
          ancestors={detail.context?.ancestors ?? []}
          summary={issue}
          externalKeys={detail.externalKeys}
        />
        {canMutateServer && (
          <IssueActions
            actor={actor}
            changePin={() => void changePin()}
            pending={mutation.status === "pending"}
            pinned={pinned}
            summary={issue}
            changeLifecycle={(action) => void changeLifecycle(action)}
            edit={openMetadataEditor}
          />
        )}
        {canMutateServer && metadataEditor === undefined && (
          <MutationResult mutation={mutation} />
        )}
        {canMutateServer && metadataEditor !== undefined && (
          <EditIssueDialog
            actor={actor}
            draft={metadataEditor}
            error={mutation.status === "error" ? mutation.message : undefined}
            issueId={issueId}
            pending={mutation.status === "pending"}
            onChange={(draft) => {
              setMetadataEditor(draft);
              if (mutation.status === "error") {
                setMutation({ status: "idle" });
              }
            }}
            onDismiss={() => setMetadataEditor(undefined)}
            onSubmit={() => void saveMetadata()}
          />
        )}
        <PrimaryRecord
          boards={boards}
          detail={detail}
          detailsOpen={!collapsedDetailsBoardIds.includes(issue.boardId)}
          projects={projects}
          selectLabel={selectLabel}
          setDetailsOpen={(open) =>
            setDetailsCollapsed(issue.boardId, !open)
          }
        />
        <AttachmentRecords
          boardId={issue.boardId}
          issueId={issue.id}
          source={source}
        />
        <RelationshipBand
          source={source}
          canMutate={canMutateServer}
          dependencyQuery={dependencyQuery}
          detail={detail}
          pending={mutation.status === "pending"}
          relationsOpen={relationsOpen}
          removeDependency={(id) => void changeDependency(id, "remove")}
          setDependencyQuery={setDependencyQuery}
          setRelationsOpen={setRelationsOpen}
          addDependency={(id) => void changeDependency(id, "add")}
        />
        <IssueLog
          entries={logEntries}
          state={state}
          request={logRequest}
          retry={() => void logRequest.refetch()}
        />
        <CurrentIssueState state={state} />
        {canMutateServer && (
          <IssueLogComposer
            body={logBody}
            pending={mutation.status === "pending"}
            setBody={setLogBody}
            submit={(event) => {
              event.preventDefault();
              if (logBody.trim() !== "") {
                void addLogEntry();
              }
            }}
          />
        )}
        <CheckpointRecord detail={detail} />
        {canMutateServer && (
          <CheckpointControls
            actor={actor}
            pending={mutation.status === "pending"}
            reason={checkpointReason}
            setReason={setCheckpointReason}
            summary={issue}
            decide={(decision) => void decideCheckpoint(decision)}
          />
        )}
        {canMutateServer && (
          <AttachmentUploadPanel
            actor={actor}
            boardId={issue.boardId}
            client={attachmentClient}
            issueId={issue.id}
          />
        )}
      </article>
    </IssueReferencePreviewLoaderContext.Provider>
  );
}

type MutationState =
  | { status: "idle" }
  | { status: "pending" }
  | { status: "success"; message: string }
  | { status: "error"; message: string };

export function IssueHeader({
  ancestors = [],
  summary,
  externalKeys,
}: {
  ancestors?: readonly AncestorContext[];
  summary: IssueSummary;
  externalKeys: string[];
}) {
  return (
    <header className="issue-detail-header">
      <div className="issue-detail-id">
        <IssueBreadcrumbs ancestors={ancestors} current={summary} />
        {externalKeys.map((externalKey) => (
          <span
            className="metadata-chip issue-external-key"
            key={externalKey}
          >
            {externalKey}
          </span>
        ))}
      </div>
      <h1 id="issue-title">{summary.title}</h1>
      <div className="issue-detail-badges" aria-label="Issue classification">
        <IssueStatusBadge status={summary.status} />
        <span className="metadata-chip">{issueTypeLabel(summary.type)}</span>
        <span className="metadata-chip">P{summary.priority}</span>
      </div>
      <IssueWaitingReason issue={summary} mode="detail" />
    </header>
  );
}

/** IssueBreadcrumbs presents the containment path in inherited context order. */
export function IssueBreadcrumbs({
  ancestors,
  current,
}: {
  ancestors: readonly AncestorContext[];
  current: IssueSummary;
}) {
  const ancestorIssues = ancestors.flatMap(({ issue }) =>
    issue === undefined ? [] : [issue],
  );
  const currentReference = `%${current.id}`;
  return (
    <nav
      className="issue-containment-breadcrumb"
      aria-label="Issue containment"
    >
      <ol>
        {ancestorIssues.map((issue) => {
          const reference = `%${issue.id}`;
          return (
            <li key={issue.id}>
              <IssueReferencePill issue={issue} issueID={issue.id}>
                <Link
                  className="issue-containment-link"
                  to={issuePath(issue.boardId, issue.id)}
                >
                  {reference}
                </Link>
              </IssueReferencePill>
              <ChevronRight aria-hidden="true" />
            </li>
          );
        })}
        <li>
          <IssueReferencePill issue={current} issueID={current.id}>
            <span aria-current="page">{currentReference}</span>
          </IssueReferencePill>
        </li>
      </ol>
    </nav>
  );
}

/** PrimaryRecord renders stable issue content and metadata. */
export function PrimaryRecord({
  boards = [],
  detail,
  detailsOpen = true,
  projects = [],
  selectLabel,
  setDetailsOpen = () => {},
}: {
  boards?: readonly BoardSummary[];
  detail: IssueDetail;
  detailsOpen?: boolean;
  projects?: readonly Project[];
  selectLabel: SelectLabel;
  setDetailsOpen?: (open: boolean) => void;
}) {
  const issue = detail.issue;
  if (issue === undefined) {
    return null;
  }
  const provenance = issueProvenance(issue, { boards, projects });
  return (
    <>
      <section
        className="issue-detail-section issue-record-section"
        aria-labelledby="record-title"
      >
        <h2 id="record-title">Record</h2>
        <dl className="issue-record">
          {provenance.source !== undefined && (
            <div>
              <dt>Source</dt>
              <dd>{provenance.source}</dd>
            </div>
          )}
          {provenance.project !== undefined && (
            <div>
              <dt>Project</dt>
              <dd>{provenance.project}</dd>
            </div>
          )}
          <div>
            <dt>Board</dt>
            <dd>{provenance.board ?? issue.boardId}</dd>
          </div>
          <div>
            <dt>Created</dt>
            <dd>{formatTimestamp(issue.createdAt)}</dd>
          </div>
          <div>
            <dt>Updated</dt>
            <dd>{formatTimestamp(issue.updatedAt)}</dd>
          </div>
          <div>
            <dt>Custody</dt>
            <dd>{issue.activeClaim?.actor ?? "Unclaimed"}</dd>
          </div>
        </dl>
        {issue.labels.length > 0 && (
          <ul className="issue-labels" aria-label="Labels">
            {issue.labels.map((label) => (
              <li key={label}>
                <IssueLabel label={label} select={selectLabel} />
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="issue-detail-section" aria-labelledby="summary-title">
        <h2 id="summary-title">Summary</h2>
        <Markdown content={detail.summary} empty="No summary." />
      </section>

      {hasMarkdown(detail.details) && (
        <details
          className="issue-detail-section issue-details"
          open={detailsOpen}
          onToggle={(event) => setDetailsOpen(event.currentTarget.open)}
        >
          <summary>
            <h2 id="details-title">Details</h2>
          </summary>
          <Markdown content={detail.details} />
        </details>
      )}

      {hasMarkdown(detail.result) && (
        <section
          className="issue-detail-section"
          aria-labelledby="result-title"
        >
          <h2 id="result-title">Result</h2>
          <Markdown content={detail.result} />
        </section>
      )}
    </>
  );
}

/** IssueActions keeps mutations inside one collapsed header disclosure. */
export function IssueActions({
  actor,
  changePin,
  changeLifecycle,
  edit,
  pending,
  pinned,
  summary,
}: {
  actor: string;
  changePin: () => void;
  changeLifecycle: (action: LifecycleActionValue) => void;
  edit: () => void;
  pending: boolean;
  pinned: boolean;
  summary: IssueSummary;
}) {
  const actions = availableLifecycleActions(summary, actor);
  const actorMissing = actor.trim() === "";
  return (
    <details className="issue-actions">
      <summary>Issue actions</summary>
      <div className="issue-action-row" aria-label="Issue changes">
        <button
          type="button"
          className="issue-action-quiet"
          disabled={pending}
          onClick={edit}
        >
          <Pencil aria-hidden="true" size={14} strokeWidth={2} />
          Edit issue
        </button>
        <button
          type="button"
          className="issue-action-quiet"
          disabled={pending || actorMissing}
          onClick={changePin}
        >
          {pinned ? (
            <PinOff aria-hidden="true" size={14} strokeWidth={2} />
          ) : (
            <Pin aria-hidden="true" size={14} strokeWidth={2} />
          )}
          {pinned ? "Unpin from board" : "Pin to board"}
        </button>
        {actions.map((action) => (
          <button
            key={action}
            type="button"
            className={
              action === LifecycleAction.CANCEL
                ? "danger-button"
                : undefined
            }
            disabled={pending}
            onClick={() => changeLifecycle(action)}
          >
            {lifecycleActionLabel(action)}
          </button>
        ))}
      </div>
    </details>
  );
}

function EditIssueDialog({
  actor,
  draft,
  error,
  issueId,
  onChange,
  onDismiss,
  onSubmit,
  pending,
}: {
  actor: string;
  draft: IssueMetadataDraft;
  error: string | undefined;
  issueId: string;
  onChange: (draft: IssueMetadataDraft) => void;
  onDismiss: () => void;
  onSubmit: () => void;
  pending: boolean;
}) {
  const actorMissing = actor.trim() === "";
  return (
    <IssueFormDialog
      actions={
        <>
          <button
            type="button"
            className="secondary-button"
            disabled={pending}
            onClick={onDismiss}
          >
            Cancel
          </button>
          <button type="submit" disabled={pending || actorMissing}>
            {pending ? "Saving" : "Save issue"}
          </button>
        </>
      }
      description={issueId}
      title="Edit issue"
      titleId="edit-issue-title"
      onSubmit={(event) => {
        event.preventDefault();
        if (!pending && !actorMissing) {
          onSubmit();
        }
      }}
    >
      <IssueMetadataFields
        autoFocusTitle
        disabled={pending}
        draft={draft}
        onChange={(field, value) =>
          onChange({ ...draft, [field]: value })
        }
      />
      {error !== undefined && (
        <p className="form-error form-field-wide" role="alert">
          {error}
        </p>
      )}
      {actorMissing && (
        <p className="issue-control-note form-field-wide">
          Set an actor in Settings to edit this issue.
        </p>
      )}
    </IssueFormDialog>
  );
}

function CheckpointRecord({ detail }: { detail: IssueDetail }) {
  const decision = detail.checkpointDecision;
  const issue = detail.issue;
  if (decision === undefined || issue === undefined) {
    return null;
  }
  return (
    <section
      className="issue-detail-section"
      aria-labelledby="checkpoint-record-title"
    >
      <h2 id="checkpoint-record-title">Checkpoint decision</h2>
      <dl className="issue-record">
        <div>
          <dt>Outcome</dt>
          <dd>{checkpointOutcomeLabel(decision.outcome)}</dd>
        </div>
        <div>
          <dt>Decided</dt>
          <dd>{formatTimestamp(decision.decidedAt)}</dd>
        </div>
        <div>
          <dt>Lifecycle</dt>
          <dd>{enumLabel(IssueLifecycle[issue.lifecycle])}</dd>
        </div>
        <div>
          <dt>Revision</dt>
          <dd>Revision {decision.revision.toString()}</dd>
        </div>
      </dl>
      <Markdown content={decision.reason} />
    </section>
  );
}

function CheckpointControls({
  actor,
  decide,
  pending,
  reason,
  setReason,
  summary,
}: {
  actor: string;
  decide: (outcome: CheckpointOutcome) => void;
  pending: boolean;
  reason: string;
  setReason: (reason: string) => void;
  summary: IssueSummary;
}) {
  const presentation = checkpointDecisionPresentation(summary);
  if (presentation.state === "hidden") {
    return null;
  }
  if (presentation.state === "waiting") {
    return (
      <section
        className="issue-detail-section issue-checkpoint-waiting"
        aria-labelledby="checkpoint-title"
      >
        <h2 id="checkpoint-title">Checkpoint decision</h2>
        <p>{presentation.message}</p>
      </section>
    );
  }
  const actorMissing = actor.trim() === "";
  return (
    <section className="issue-detail-section issue-checkpoint" aria-labelledby="checkpoint-title">
      <div>
        <h2 id="checkpoint-title">Checkpoint decision</h2>
        <p>Record the human decision for this checkpoint.</p>
      </div>
      <label htmlFor="checkpoint-reason">
        Reason <span>(optional)</span>
        <textarea
          id="checkpoint-reason"
          rows={2}
          value={reason}
          disabled={pending}
          onInput={(event) => setReason(event.currentTarget.value)}
        />
      </label>
      {actorMissing && (
        <p className="issue-control-note">Set an actor in Settings to record a decision.</p>
      )}
      <div className="issue-action-row">
        <button
          type="button"
          disabled={pending || actorMissing}
          onClick={() => decide(CheckpointOutcome.APPROVED)}
        >
          Approve
        </button>
        <button
          type="button"
          className="danger-button"
          disabled={pending || actorMissing}
          onClick={() => decide(CheckpointOutcome.DENIED)}
        >
          Deny
        </button>
      </div>
    </section>
  );
}

function MutationResult({ mutation }: { mutation: MutationState }) {
  if (mutation.status === "idle") {
    return null;
  }
  return (
    <p
      className="issue-mutation-result"
      data-status={mutation.status}
      role={mutation.status === "error" ? "alert" : "status"}
    >
      {mutation.status === "pending" ? "Updating issue..." : mutation.message}
    </p>
  );
}

export function RelationshipBand({
  addDependency,
  source,
  canMutate = true,
  dependencyQuery,
  detail,
  pending,
  relationsOpen,
  removeDependency,
  setDependencyQuery,
  setRelationsOpen,
}: {
  addDependency: (id: string) => void;
  source?: SourceRef;
  canMutate?: boolean;
  dependencyQuery: string;
  detail: IssueDetail;
  pending: boolean;
  relationsOpen: boolean;
  removeDependency: (id: string) => void;
  setDependencyQuery: (query: string) => void;
  setRelationsOpen: (open: boolean) => void;
}) {
  const issue = detail.issue;
  if (issue === undefined) {
    return null;
  }
  const hasRelations =
    detail.prerequisites.length > 0 ||
    detail.dependents.length > 0 ||
    detail.containment?.nodes.some(
      (node) => node.issue !== undefined && node.issue.id !== issue.id,
    ) === true;
  if (!hasRelations) {
    return null;
  }
  return (
    <details
      className="issue-detail-section issue-relations"
      open={relationsOpen}
      onToggle={(event) => setRelationsOpen(event.currentTarget.open)}
    >
      <summary>
        <h2 id="relations-title">Relations</h2>
      </summary>
      <div className="issue-relationship-band" aria-labelledby="relations-title">
        <DependencyPanel
          add={canMutate ? addDependency : undefined}
          boardId={issue.boardId}
          source={source}
          currentIssueId={issue.id}
          dependencies={detail.prerequisites}
          pending={pending}
          query={dependencyQuery}
          remove={canMutate ? removeDependency : undefined}
          setQuery={setDependencyQuery}
        />
        <HierarchyPanel detail={detail} />
        <section className="issue-relationship-panel" aria-labelledby="dependents-title">
          <h2 id="dependents-title">Dependents</h2>
          <RelationshipList
            empty="No dependents."
            issues={detail.dependents}
            pending={pending}
          />
        </section>
      </div>
    </details>
  );
}

function DependencyPanel({
  add,
  boardId,
  source,
  currentIssueId,
  dependencies,
  pending,
  query,
  remove,
  setQuery,
}: {
  add?: (id: string) => void;
  boardId: string;
  source?: SourceRef;
  currentIssueId: string;
  dependencies: readonly RelatedIssue[];
  pending: boolean;
  query: string;
  remove?: (id: string) => void;
  setQuery: (query: string) => void;
}) {
  const transport = useTransport();
  const [searchOpen, setSearchOpen] = useState(false);
  const normalizedQuery = query.trim();
  const excludedIssueIds = new Set([
    currentIssueId,
    ...dependencies.map(({ id }) => id),
  ]);
  const candidates = useQuery({
    ...unaryRouteQueryOptions(
      IssueService.method.listIssues,
      dependencySearchInput(boardId, normalizedQuery, source),
      transport,
    ),
    enabled: add !== undefined && searchOpen && normalizedQuery !== "",
    select: (response) =>
      dependencyCandidates(response.issues, excludedIssueIds),
  });
  const results = candidates.data ?? [];

  return (
    <section className="issue-relationship-panel" aria-labelledby="dependencies-title">
      <div className="issue-panel-heading">
        <h2 id="dependencies-title">Dependencies</h2>
        {add !== undefined && (
          <button
            type="button"
            className="issue-add-dependency"
            aria-expanded={searchOpen}
            aria-controls="dependency-search"
            aria-label="Add dependency"
            title="Add dependency"
            onClick={() => setSearchOpen((open) => !open)}
          >
            +
          </button>
        )}
      </div>
      <RelationshipList
        empty="No dependencies."
        issues={dependencies}
        remove={remove}
        pending={pending}
      />
      {add !== undefined && searchOpen && (
        <form
          id="dependency-search"
          className="issue-dependency-search"
          onSubmit={(event) => {
            event.preventDefault();
            const id = query.trim();
            if (id !== "") {
              add(id);
            }
          }}
        >
          <label htmlFor="dependency-query">Search by title or enter an issue ID</label>
          <input
            id="dependency-query"
            type="search"
            value={query}
            placeholder="Search issues"
            onInput={(event) => setQuery(event.currentTarget.value)}
          />
          <button type="submit" disabled={pending || query.trim() === ""}>
            Add issue ID
          </button>
          {candidates.isFetching && query.trim() !== "" && (
            <p className="issue-detail-empty" role="status">Searching...</p>
          )}
          {candidates.isError && (
            <p className="issue-control-note" role="alert">
              Search failed: {candidates.error.message}
            </p>
          )}
          {query.trim() !== "" && candidates.isSuccess && results.length === 0 && (
            <p className="issue-detail-empty">No matching issues.</p>
          )}
          {results.length > 0 && (
            <ul className="issue-dependency-results">
              {results.map((candidate) => (
                <li key={candidate.id}>
                  <button
                    type="button"
                    disabled={pending}
                    onClick={() => add(candidate.id)}
                  >
                    <span>{candidate.title}</span>
                    <small>{candidate.id}</small>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </form>
      )}
    </section>
  );
}

export function RelationshipList({
  empty,
  issues,
  pending,
  remove,
}: {
  empty: string;
  issues: readonly RelatedIssue[];
  pending: boolean;
  remove?: (id: string) => void;
}) {
  if (issues.length === 0) {
    return <p className="issue-detail-empty">{empty}</p>;
  }
  return (
    <ul className="issue-relationship-list">
      {issues.map((issue) => (
        <li key={issue.id}>
          <IssueReferenceLink issue={issue} />
          {remove !== undefined && (
            <button
              type="button"
              className="issue-action-quiet"
              disabled={pending}
              onClick={() => remove(issue.id)}
            >
              Remove
            </button>
          )}
        </li>
      ))}
    </ul>
  );
}

function HierarchyPanel({ detail }: { detail: IssueDetail }) {
  const issue = detail.issue;
  if (issue === undefined) {
    return null;
  }
  const nodes = detail.containment?.nodes ?? [];
  return (
    <section className="issue-relationship-panel" aria-labelledby="hierarchy-title">
      <h2 id="hierarchy-title">Hierarchy</h2>
      <IssueHierarchy
        nodes={nodes}
        selectedIssueId={issue.id}
      />
    </section>
  );
}

function IssueLog({
  entries,
  state,
  request,
  retry,
}: {
  entries: readonly LogEntry[] | undefined;
  state: StateRecord | undefined;
  request: QueryState<LogEntry[]>;
  retry: () => void;
}) {
  useEffect(() => {
    if (entries !== undefined) {
      scrollToLogEntryFragment(
        state?.snapshotLogEntryId === undefined
          ? entries
          : [...entries, { id: state.snapshotLogEntryId }],
        globalThis.location.hash,
        document,
      );
    }
  }, [entries, state?.snapshotLogEntryId]);

  return (
    <section className="issue-detail-section issue-log" aria-labelledby="log-title">
      <div className="issue-section-heading">
        <h2 id="log-title">Log</h2>
        <span>{entries?.length ?? 0}</span>
      </div>
      <RequestRefreshError request={request} recordName="log" />
      {request.isError && request.data === undefined && (
        <button type="button" className="issue-action-quiet" onClick={retry}>
          Retry log
        </button>
      )}
      {entries === undefined ? (
        !request.isError && <p role="status">Loading log...</p>
      ) : entries.length === 0 ? (
        <p className="issue-detail-empty">No log entries.</p>
      ) : (
        <LogEntryList entries={entries} />
      )}
    </section>
  );
}

function IssueLogComposer({
  body,
  pending,
  setBody,
  submit,
}: {
  body: string;
  pending: boolean;
  setBody: (body: string) => void;
  submit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  return (
    <section className="issue-log-composer" aria-label="Log composer">
      <form className="issue-log-form" onSubmit={submit}>
        <label htmlFor="log-body">Add log entry</label>
        <textarea
          id="log-body"
          rows={5}
          value={body}
          onInput={(event) => setBody(event.currentTarget.value)}
        />
        <button type="submit" disabled={pending || body.trim() === ""}>
          Add log entry
        </button>
      </form>
    </section>
  );
}

/** CurrentIssueState renders retained mutable State after immutable Log history. */
export function CurrentIssueState({ state }: { state: StateRecord | undefined }) {
  if (
    state === undefined ||
    (!hasMarkdown(state.body) && !hasMarkdown(state.nextAction))
  ) {
    return null;
  }
  return (
    <section
      className="issue-detail-section issue-current-state"
      id={state.snapshotLogEntryId}
      aria-labelledby="state-title"
    >
      <header className="issue-current-state-heading">
        <h2 id="state-title">State</h2>
        <time>{formatTimestamp(state.updatedAt)}</time>
      </header>
      <Markdown content={state.body} />
      <NextAction content={state.nextAction} label="Next action" />
    </section>
  );
}

/** LogEntryList renders typed immutable records without card chrome. */
export function LogEntryList({ entries }: { entries: readonly LogEntry[] }) {
  return (
    <ol className="issue-log-list">
      {entries.map((entry) => {
        const payload = logEntryPresentation(entry);
        return (
          <li id={entry.id} key={entry.id}>
            <article>
              <header>
                <div className="issue-log-origin">
                  <strong>{payload.actor}</strong>
                  {payload.kind !== undefined && (
                    <span className="issue-log-kind">{payload.kind}</span>
                  )}
                </div>
                <time>{formatTimestamp(payload.createdAt)}</time>
              </header>
              <Markdown content={payload.body} />
              <NextAction
                content={payload.nextAction}
                label="Planned next action"
              />
            </article>
          </li>
        );
      })}
    </ol>
  );
}

/** NextAction renders structured action Markdown without interpreting State prose. */
function NextAction({
  content,
  label,
}: {
  content: MarkdownContent | undefined;
  label: string;
}) {
  if (content === undefined || content.source.trim() === "") {
    return null;
  }
  return (
    <div className="issue-next-action">
      <h3>{label}</h3>
      <Markdown content={content} />
    </div>
  );
}

/** scrollToLogEntryFragment restores fragment navigation after log data loads. */
export function scrollToLogEntryFragment(
  entries: readonly Pick<LogEntry, "id">[],
  hash: string,
  root: Document,
) {
  if (!hash.startsWith("#")) {
    return;
  }
  const id = hash.slice(1);
  if (!entries.some((entry) => entry.id === id)) {
    return;
  }
  root.getElementById(id)?.scrollIntoView({ block: "start" });
}

export function Markdown({
  content,
  empty,
}: {
  content: MarkdownContent | undefined;
  empty?: string;
}) {
  const loadIssue = useContext(IssueReferencePreviewLoaderContext);
  const enhancementProps = useMarkdownEnhancementProps(
    content?.renderedHtml ?? "",
    loadIssue,
  );

  if (content === undefined || content.source.trim() === "") {
    return empty === undefined ? null : (
      <p className="issue-detail-empty">{empty}</p>
    );
  }
  return <div className="issue-markdown" {...enhancementProps} />;
}

/**
 * useMarkdownEnhancementProps keeps unchanged server HTML from replacing
 * mounted Markdown enhancements during unrelated React renders.
 */
export function useMarkdownEnhancementProps(
  renderedHtml: string,
  loadIssue: LoadIssueReferencePreview | undefined,
) {
  const dangerouslySetInnerHTML = useMemo(
    () => ({ __html: renderedHtml }),
    [renderedHtml],
  );
  const enhanceMarkdown = useCallback((element: HTMLDivElement | null) => {
    if (element === null) {
      return;
    }
    linkMarkdownImages(element);
    const unmountIssues = loadIssue === undefined
      ? () => undefined
      : mountMarkdownIssueReferences(element, loadIssue);
    const unmountObjects = mountMarkdownObjectReferences(element);
    return () => {
      unmountIssues();
      unmountObjects();
    };
  }, [renderedHtml, loadIssue]);

  return { dangerouslySetInnerHTML, ref: enhanceMarkdown };
}

function hasMarkdown(content: MarkdownContent | undefined): boolean {
  return content !== undefined && content.source.trim() !== "";
}

/** MarkdownIssueReferenceTarget binds one safe server marker to its issue route. */
interface MarkdownIssueReferenceTarget {
  /** element receives the React clipboard-pill child root. */
  element: HTMLElement;

  /** href is the server-resolved same-board issue route. */
  href: string;

  /** id is the grammar-certified issue ID without its percent prefix. */
  id: string;
}

/** markdownIssueReferenceTargets prepares durable server markers for child roots. */
export function markdownIssueReferenceTargets(
  root: HTMLDivElement,
): MarkdownIssueReferenceTarget[] {
  const targets: MarkdownIssueReferenceTarget[] = [];
  for (const element of root.querySelectorAll<HTMLElement>(
    "[data-issue-reference]",
  )) {
    const id = element.dataset.issueReference;
    const href = element.getAttribute("data-issue-reference-href");
    if (id === undefined || href === null) {
      continue;
    }
    element.replaceChildren();
    targets.push({ element, href, id });
  }
  return targets;
}

function mountMarkdownIssueReferences(
  root: HTMLDivElement,
  loadIssue: LoadIssueReferencePreview,
): () => void {
  const roots = markdownIssueReferenceTargets(root).map(
    ({ element, href, id }) => {
      const issueReference = `%${id}`;
      const childRoot = createRoot(element);
      childRoot.render(
        <IssueReferencePill issueID={id} loadIssue={loadIssue}>
          <a href={href}>{issueReference}</a>
        </IssueReferencePill>,
      );
      return childRoot;
    },
  );
  return () => {
    for (const childRoot of roots) {
      childRoot.unmount();
    }
  };
}

type MarkdownObjectReferenceKind = "attachment" | "log";

/** MarkdownObjectReferenceTarget binds a resolved server marker to its route. */
interface MarkdownObjectReferenceTarget {
  /** element receives the React clipboard-pill child root. */
  element: HTMLElement;

  /** href is the server-resolved same-board object route. */
  href: string;

  /** id is the grammar-certified object ID without its percent prefix. */
  id: string;

  /** kind identifies the referenced Cardamom object. */
  kind: MarkdownObjectReferenceKind;

  /** label is the server-selected visible link text. */
  label: string;
}

/** markdownObjectReferenceTargets prepares resolved object markers for child roots. */
export function markdownObjectReferenceTargets(
  root: HTMLDivElement,
): MarkdownObjectReferenceTarget[] {
  const targets: MarkdownObjectReferenceTarget[] = [];
  for (const element of root.querySelectorAll<HTMLElement>(
    "[data-cardamom-reference]",
  )) {
    const kind = element.dataset.cardamomReference;
    const id = element.dataset.cardamomReferenceId;
    const label = element.dataset.cardamomReferenceLabel;
    const href = element.getAttribute("data-cardamom-reference-href");
    if (
      (kind !== "attachment" && kind !== "log") ||
      id === undefined ||
      label === undefined ||
      href === null
    ) {
      continue;
    }
    element.replaceChildren();
    targets.push({ element, href, id, kind, label });
  }
  return targets;
}

function mountMarkdownObjectReferences(root: HTMLDivElement): () => void {
  const roots = markdownObjectReferenceTargets(root).map(
    ({ element, href, id, kind, label }) => {
      const reference = `%${id}`;
      const childRoot = createRoot(element);
      childRoot.render(
        <ClipboardPill
          copyLabel={`Copy ${kind} reference ${reference}`}
          copyText={reference}
        >
          <a href={href}>{label}</a>
        </ClipboardPill>,
      );
      return childRoot;
    },
  );
  return () => {
    for (const childRoot of roots) {
      childRoot.unmount();
    }
  };
}

/** linkMarkdownImages makes each rendered image open its original content. */
export function linkMarkdownImages(root: HTMLDivElement) {
  for (const image of root.querySelectorAll("img")) {
    let link = image.closest("a");
    if (link === null) {
      link = root.ownerDocument.createElement("a");
      image.replaceWith(link);
      link.append(image);
    }
    link.href = image.src;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
  }
}

function InitialRequestState<T>({
  loadingMessage,
  request,
  retry,
}: {
  loadingMessage: string;
  request: QueryState<T>;
  retry: () => void;
}) {
  if (!request.isError) {
    return (
      <section className="issue-detail-state" role="status">
        <h1>{loadingMessage}</h1>
      </section>
    );
  }
  return (
    <section className="issue-detail-state" role="alert">
      <h1>Issue could not be loaded</h1>
      <p>{request.error?.message}</p>
      <button type="button" onClick={retry}>
        Retry
      </button>
    </section>
  );
}

function RequestRefreshError<T>({
  recordName,
  request,
}: {
  recordName: string;
  request: QueryState<T>;
}) {
  if (request.isError) {
    return (
      <p className="issue-refresh-state" role="alert">
        Could not refresh {recordName}: {request.error?.message}
      </p>
    );
  }
  return null;
}

/** QueryState is the route-visible subset shared by detail query results. */
interface QueryState<T> {
  data: T | undefined;
  error: Error | null;
  isError: boolean;
  isFetching: boolean;
}

function requestData<T>(request: QueryState<T>): T | undefined {
  return request.data;
}

function formatTimestamp(
  timestamp: { seconds: bigint; nanos: number } | undefined,
): string {
  if (timestamp === undefined) {
    return "Not recorded";
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

function failureMessage(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure);
}

function describeCheckpointDecision(
  outcome: CheckpointOutcome,
  cancelledDependents: number,
): string {
  if (outcome === CheckpointOutcome.APPROVED) {
    return "Checkpoint approved.";
  }
  if (cancelledDependents === 0) {
    return "Checkpoint denied.";
  }
  const noun = cancelledDependents === 1 ? "dependent" : "dependents";
  return `Checkpoint denied; ${cancelledDependents} ${noun} cancelled.`;
}

function checkpointOutcomeLabel(outcome: CheckpointOutcome): string {
  if (outcome === CheckpointOutcome.APPROVED) {
    return "Approved";
  }
  if (outcome === CheckpointOutcome.DENIED) {
    return "Denied";
  }
  return "Unknown";
}

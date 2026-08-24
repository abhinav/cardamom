import { useMutation, useTransport } from "@connectrpc/connect-query";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Pencil } from "lucide-react";
import type { FormEvent } from "react";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

import type { BoardScopeSelection } from "./board-scope.ts";
import { WatchResource } from "./gen/cardamom/private/v1/change_pb.ts";
import { ProjectService, type Board } from "./gen/cardamom/private/v1/project_pb.ts";
import type { SourceRef } from "./gen/cardamom/private/v1/source_pb.ts";
import {
  runInvalidatingMutation,
  unaryRouteQueryOptions,
} from "./query-runtime.ts";

/** boardSettingsBoardId returns the writable board selected for settings. */
export function boardSettingsBoardId(
  selection: BoardScopeSelection,
): string | undefined {
  return selection.kind === "board" ? selection.boardId : undefined;
}

/** boardSettingsUpdateInput normalizes editable settings at the RPC boundary. */
export function boardSettingsUpdateInput(
  boardId: string,
  actor: string,
  name: string,
  descriptionSource: string,
) {
  return {
    boardId,
    name: name.trim(),
    descriptionSource,
    context: { actor: actor.trim() },
  };
}

/** boardConfigurationRequest identifies a board on a local or aggregate server. */
export function boardConfigurationRequest(boardId: string, source?: SourceRef) {
  return { boardId, source };
}

/** boardSettingsLoaded initializes editor state only for the first board result. */
export function boardSettingsLoaded(
  state: BoardSettingsEditorState,
  board: Board,
): BoardSettingsEditorState {
  if (state.initialized) {
    return state;
  }
  return {
    initialized: true,
    draft: {
      name: board.name,
      descriptionSource: board.description?.source ?? "",
    },
    mode: "read",
    submission: { kind: "idle" },
  };
}

interface BoardConfigurationDialogProps {
  actor?: string;
  boardId: string;
  source?: SourceRef;
  onDismiss: () => void;
  onSaved: () => void;
}

/**
 * BoardConfigurationDialog presents the selected board and enables mutations
 * only when the shell supplies a local actor.
 */
export function BoardConfigurationDialog({
  actor,
  boardId,
  source,
  onDismiss,
  onSaved,
}: BoardConfigurationDialogProps) {
  if (actor === undefined) {
    return (
      <ReadonlyBoardConfigurationDialog
        boardId={boardId}
        source={source}
        onDismiss={onDismiss}
      />
    );
  }
  return (
    <EditableBoardConfigurationDialog
      actor={actor}
      boardId={boardId}
      onDismiss={onDismiss}
      onSaved={onSaved}
    />
  );
}

interface EditableBoardConfigurationDialogProps {
  actor: string;
  boardId: string;
  onDismiss: () => void;
  onSaved: () => void;
}

interface ReadonlyBoardConfigurationDialogProps {
  boardId: string;
  source?: SourceRef;
  onDismiss: () => void;
}

/** ReadonlyBoardConfigurationDialog presents a remote board without mutations. */
function ReadonlyBoardConfigurationDialog({
  boardId,
  source,
  onDismiss,
}: ReadonlyBoardConfigurationDialogProps) {
  const transport = useTransport();
  const load = useQuery({
    ...unaryRouteQueryOptions(
      ProjectService.method.getBoard,
      boardConfigurationRequest(boardId, source),
      transport,
    ),
    select(response) {
      if (response.board === undefined) {
        throw new Error("GetBoard response did not include the selected board");
      }
      return response.board;
    },
  });
  const boardLabel = source?.sourceId === undefined
    ? boardId
    : `${source.sourceId} / ${boardId}`;

  return (
    <Dialog open onOpenChange={(open) => !open && onDismiss()}>
      <DialogContent
        className="modal-panel modal-panel-compact max-w-lg"
      >
        <DialogHeader className="pr-8">
          <DialogTitle id="board-configuration-title">Board configuration</DialogTitle>
          <DialogDescription>{boardLabel}</DialogDescription>
        </DialogHeader>
        {load.data === undefined && !load.isError && (
          <p role="status">Loading board configuration</p>
        )}
        {load.data === undefined && load.isError && (
          <div className="modal-load-error" role="alert">
            <p>{load.error.message}</p>
            <Button type="button" onClick={() => void load.refetch()}>
              Retry
            </Button>
          </div>
        )}
        {load.data !== undefined && <BoardContextContent board={load.data} />}
      </DialogContent>
    </Dialog>
  );
}

/**
 * EditableBoardConfigurationDialog owns settings and lifecycle mutations for
 * one explicit board. Archived boards remain readable here and expose only
 * unarchive.
 */
function EditableBoardConfigurationDialog({
  actor,
  boardId,
  onDismiss,
  onSaved,
}: EditableBoardConfigurationDialogProps) {
  const transport = useTransport();
  const queryClient = useQueryClient();
  const updateBoard = useMutation(ProjectService.method.updateBoard);
  const archiveBoard = useMutation(ProjectService.method.archiveBoard);
  const unarchiveBoard = useMutation(ProjectService.method.unarchiveBoard);
  const load = useQuery({
    ...unaryRouteQueryOptions(
      ProjectService.method.getBoard,
      { boardId },
      transport,
    ),
    select(response) {
      if (response.board === undefined) {
        throw new Error("GetBoard response did not include the selected board");
      }
      return response.board;
    },
  });
  const [editor, setEditor] = useState<BoardSettingsEditorState>(() => ({
    initialized: false,
    draft: { name: "", descriptionSource: "" },
    mode: "read",
    submission: { kind: "idle" },
  }));
  const { draft, mode, submission } = editor;
  const [archiveReason, setArchiveReason] = useState("");
  const [archiveExpanded, setArchiveExpanded] = useState(false);
  const [lifecycleError, setLifecycleError] = useState<string>();

  useEffect(() => {
    if (load.data === undefined) {
      return;
    }
    setEditor((current) => boardSettingsLoaded(current, load.data));
  }, [load.data]);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (load.data === undefined || submission.kind === "submitting") {
      return;
    }
    setEditor((current) => ({
      ...current,
      submission: { kind: "submitting" },
    }));
    try {
      const response = await runInvalidatingMutation(
        queryClient,
        [WatchResource.BOARD, WatchResource.BOARD_CATALOG],
        () =>
          updateBoard.mutateAsync(
            boardSettingsUpdateInput(
              boardId,
              actor,
              draft.name,
              draft.descriptionSource,
            ),
          ),
      );
      if (response.board === undefined) {
        throw new Error("UpdateBoard response did not include the updated board");
      }
      onSaved();
    } catch (failure) {
      setEditor((current) => ({
        ...current,
        submission: { kind: "error", message: failureMessage(failure) },
      }));
    }
  };

  type LifecycleAction = "archive" | "unarchive";

  // Lifecycle changes invalidate both detail and catalog membership because
  // archival changes the board's writable state and its default discoverability.
  const changeLifecycle = async (action: LifecycleAction) => {
    setLifecycleError(undefined);
    try {
      const resources = [WatchResource.BOARD, WatchResource.BOARD_CATALOG];
      if (action === "archive") {
        await runInvalidatingMutation(
          queryClient,
          resources,
          () => archiveBoard.mutateAsync({
              boardId,
              reason: archiveReason.trim() || undefined,
              context: { actor: actor.trim() },
            }),
        );
      } else {
        await runInvalidatingMutation(
          queryClient,
          resources,
          () => unarchiveBoard.mutateAsync({ boardId }),
        );
      }
      await load.refetch();
      onSaved();
    } catch (failure) {
      setLifecycleError(failureMessage(failure));
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onDismiss()}>
      <DialogContent
        className="modal-panel modal-panel-compact max-w-lg"
      >
        <DialogHeader className="pr-8">
          <DialogTitle id="board-settings-title">Board configuration</DialogTitle>
          <DialogDescription>{boardId}</DialogDescription>
        </DialogHeader>
        {load.data === undefined && !load.isError && (
          <p role="status">Loading board settings</p>
        )}
        {load.data === undefined && load.isError && (
          <div className="modal-load-error" role="alert">
            <p>{load.error.message}</p>
            <Button type="button" onClick={() => void load.refetch()}>
              Retry
            </Button>
          </div>
        )}
        {load.data !== undefined && (
          <>
            <BoardSettingsContent
              board={load.data}
              draft={draft}
              mode={mode}
              submission={submission}
              onBeginEdit={(nextMode) => {
                setEditor((current) => ({
                  ...current,
                  draft: {
                    name: load.data.name,
                    descriptionSource: load.data.description?.source ?? "",
                  },
                  mode: nextMode,
                  submission: { kind: "idle" },
                }));
              }}
              onCancelEdit={() => {
                setEditor((current) => ({
                  ...current,
                  draft: {
                    name: load.data.name,
                    descriptionSource: load.data.description?.source ?? "",
                  },
                  mode: "read",
                  submission: { kind: "idle" },
                }));
              }}
              onChangeDraft={(nextDraft) =>
                setEditor((current) => ({
                  ...current,
                  draft: nextDraft,
                }))
              }
              onSubmit={submit}
            />
            <BoardLifecycleControls
              archived={load.data.archived !== undefined}
              expanded={archiveExpanded}
              reason={archiveReason}
              error={lifecycleError}
              submitting={archiveBoard.isPending || unarchiveBoard.isPending}
              onBeginArchive={() => setArchiveExpanded(true)}
              onCancelArchive={() => {
                setArchiveExpanded(false);
                setArchiveReason("");
                setLifecycleError(undefined);
              }}
              onReasonChange={setArchiveReason}
              onArchive={() => void changeLifecycle("archive")}
              onUnarchive={() => void changeLifecycle("unarchive")}
            />
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

interface BoardSettingsContentProps {
  board: Board;
  draft: BoardSettingsDraft;
  mode: BoardSettingsMode;
  submission: BoardSettingsSubmission;
  onBeginEdit: (mode: Exclude<BoardSettingsMode, "read">) => void;
  onCancelEdit: () => void;
  onChangeDraft: (draft: BoardSettingsDraft) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}

/**
 * BoardSettingsContent renders one loaded board in read or edit mode. Archived
 * boards remain readable and never expose the settings editor.
 */
export function BoardSettingsContent({
  board,
  draft,
  mode,
  submission,
  onBeginEdit,
  onCancelEdit,
  onChangeDraft,
  onSubmit,
}: BoardSettingsContentProps) {
  const fieldId = mode === "name"
    ? "board-settings-name"
    : "board-settings-description";
  if (mode === "read") {
    return <BoardContextContent board={board} onBeginEdit={onBeginEdit} />;
  }

  return (
    <form className="workflow-form" onSubmit={onSubmit}>
      {mode === "name" ? (
        <div className="form-field form-field-wide">
          <Label htmlFor={fieldId}>Name</Label>
          <Input
            id={fieldId}
            autoFocus
            required
            type="text"
            value={draft.name}
            onChange={(event) =>
              onChangeDraft({ ...draft, name: event.currentTarget.value })
            }
          />
        </div>
      ) : (
        <div className="form-field form-field-wide">
          <Label htmlFor={fieldId}>Description (Markdown)</Label>
          <Textarea
            id={fieldId}
            autoFocus
            rows={9}
            value={draft.descriptionSource}
            onChange={(event) =>
              onChangeDraft({
                ...draft,
                descriptionSource: event.currentTarget.value,
              })
            }
          />
        </div>
      )}
      {submission.kind === "error" && (
        <p className="form-error form-field-wide" role="alert">
          {submission.message}
        </p>
      )}
      <div className="modal-actions form-field-wide">
        <Button
          type="button"
          variant="outline"
          disabled={submission.kind === "submitting"}
          onClick={onCancelEdit}
        >
          Cancel
        </Button>
        <Button type="submit" disabled={submission.kind === "submitting"}>
          {submission.kind === "submitting"
            ? "Saving"
            : mode === "name" ? "Save name" : "Save description"}
        </Button>
      </div>
    </form>
  );
}

interface BoardContextContentProps {
  board: Board;
  onBeginEdit?: (mode: "name" | "description") => void;
}

/** BoardContextContent renders board settings with optional edit entry points. */
export function BoardContextContent({
  board,
  onBeginEdit,
}: BoardContextContentProps) {
  const renderedDescription = board.description?.renderedHtml ?? "";
  const editable = board.archived === undefined && onBeginEdit !== undefined;
  return (
    <div className="board-settings-content">
      <div className="board-settings-field-row">
        <h3>{board.name}</h3>
        {editable && (
          <Button
            type="button"
            className="board-settings-icon-button"
            variant="ghost"
            size="icon-sm"
            aria-label="Edit board name"
            title="Edit name"
            onClick={() => onBeginEdit?.("name")}
          >
            <Pencil aria-hidden="true" />
          </Button>
        )}
      </div>
      <div className="board-settings-field-row board-settings-description-row">
        {renderedDescription === "" ? (
          <p className="empty-copy">No description.</p>
        ) : (
          <div
            className="rendered-markdown board-settings-description"
            dangerouslySetInnerHTML={{ __html: renderedDescription }}
          />
        )}
        {editable && (
          <Button
            type="button"
            className="board-settings-icon-button"
            variant="ghost"
            size="icon-sm"
            aria-label="Edit board description"
            title="Edit description"
            onClick={() => onBeginEdit?.("description")}
          >
            <Pencil aria-hidden="true" />
          </Button>
        )}
      </div>
    </div>
  );
}

interface BoardSettingsDraft {
  name: string;
  descriptionSource: string;
}

/** BoardSettingsEditorState owns user input across background board refreshes. */
interface BoardSettingsEditorState {
  /** initialized distinguishes the first load from later query refreshes. */
  initialized: boolean;
  draft: BoardSettingsDraft;
  mode: BoardSettingsMode;
  submission: BoardSettingsSubmission;
}

type BoardSettingsMode = "read" | "name" | "description";

interface BoardLifecycleControlsProps {
  archived: boolean;
  expanded: boolean;
  reason: string;
  error?: string;
  submitting?: boolean;
  onBeginArchive: () => void;
  onCancelArchive: () => void;
  onReasonChange: (reason: string) => void;
  onArchive: () => void;
  onUnarchive: () => void;
}

/**
 * BoardLifecycleControls keeps the infrequent archive form collapsed until the
 * user explicitly requests it. The final Archive button remains a distinct
 * action so opening the form cannot mutate the board.
 */
export function BoardLifecycleControls({
  archived,
  expanded,
  reason,
  error,
  submitting = false,
  onBeginArchive,
  onCancelArchive,
  onReasonChange,
  onArchive,
  onUnarchive,
}: BoardLifecycleControlsProps) {
  const reasonId = "board-settings-archive-reason";
  return (
    <div className="board-settings-lifecycle">
      {archived ? (
        <div className="board-settings-lifecycle-summary">
          <p>This board is archived.</p>
          <Button
            type="button"
            variant="outline"
            disabled={submitting}
            onClick={onUnarchive}
          >
            {submitting ? "Unarchiving" : "Unarchive"}
          </Button>
        </div>
      ) : expanded ? (
        <form
          className="board-settings-archive-form"
          onSubmit={(event) => {
            event.preventDefault();
            onArchive();
          }}
        >
          <div className="form-field form-field-wide">
            <Label htmlFor={reasonId}>Archive reason (optional)</Label>
            <Input
              id={reasonId}
              autoFocus
              type="text"
              value={reason}
              onChange={(event) => onReasonChange(event.currentTarget.value)}
            />
          </div>
          <div className="board-settings-archive-actions">
            <Button
              type="button"
              variant="outline"
              disabled={submitting}
              onClick={onCancelArchive}
            >
              Cancel
            </Button>
            <Button type="submit" variant="destructive" disabled={submitting}>
              {submitting ? "Archiving" : "Archive"}
            </Button>
          </div>
        </form>
      ) : (
        <div className="board-settings-lifecycle-summary">
          <p>Archive this board</p>
          <Button
            type="button"
            className="board-settings-icon-button"
            variant="destructive"
            size="icon-sm"
            aria-label="Archive board"
            title="Archive board"
            onClick={onBeginArchive}
          >
            <Archive aria-hidden="true" />
          </Button>
        </div>
      )}
      {error !== undefined && <p role="alert">{error}</p>}
    </div>
  );
}

type BoardSettingsSubmission =
  | { kind: "idle" }
  | { kind: "submitting" }
  | { kind: "error"; message: string };

function failureMessage(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure);
}

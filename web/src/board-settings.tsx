import { useMutation, useTransport } from "@connectrpc/connect-query";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { FormEvent } from "react";
import { useEffect, useState } from "react";

import type { BoardScopeSelection } from "./board-scope.ts";
import { WatchResource } from "./gen/cardamom/private/v1/change_pb.ts";
import { ProjectService, type Board } from "./gen/cardamom/private/v1/project_pb.ts";
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

interface BoardSettingsDialogProps {
  actor: string;
  boardId: string;
  onDismiss: () => void;
  onSaved: () => void;
}

export function BoardSettingsDialog({
  actor,
  boardId,
  onDismiss,
  onSaved,
}: BoardSettingsDialogProps) {
  const transport = useTransport();
  const queryClient = useQueryClient();
  const updateBoard = useMutation(ProjectService.method.updateBoard);
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

  return (
    <div className="modal-backdrop">
      <section
        className="modal-panel modal-panel-compact"
        role="dialog"
        aria-modal="true"
        aria-labelledby="board-settings-title"
      >
        <header className="modal-header">
          <div>
            <h2 id="board-settings-title">Board settings</h2>
            <p>{boardId}</p>
          </div>
        </header>
        {load.data === undefined && !load.isError && (
          <p role="status">Loading board settings</p>
        )}
        {load.data === undefined && load.isError && (
          <div className="modal-load-error" role="alert">
            <p>{load.error.message}</p>
            <button type="button" onClick={() => void load.refetch()}>
              Retry
            </button>
          </div>
        )}
        {load.data !== undefined && (
          <BoardSettingsContent
            board={load.data}
            draft={draft}
            mode={mode}
            submission={submission}
            onBeginEdit={() => {
              setEditor((current) => ({
                ...current,
                draft: {
                  name: load.data.name,
                  descriptionSource: load.data.description?.source ?? "",
                },
                mode: "edit",
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
            onDismiss={onDismiss}
            onSubmit={submit}
          />
        )}
        {load.data === undefined && (
          <div className="modal-actions">
            <button type="button" className="secondary-button" onClick={onDismiss}>
              Close
            </button>
          </div>
        )}
      </section>
    </div>
  );
}

interface BoardSettingsContentProps {
  board: Board;
  draft: BoardSettingsDraft;
  mode: BoardSettingsMode;
  submission: BoardSettingsSubmission;
  onBeginEdit: () => void;
  onCancelEdit: () => void;
  onChangeDraft: (draft: BoardSettingsDraft) => void;
  onDismiss: () => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}

/** BoardSettingsContent renders one loaded board in read or edit mode. */
export function BoardSettingsContent({
  board,
  draft,
  mode,
  submission,
  onBeginEdit,
  onCancelEdit,
  onChangeDraft,
  onDismiss,
  onSubmit,
}: BoardSettingsContentProps) {
  if (mode === "read") {
    const renderedDescription = board.description?.renderedHtml ?? "";
    return (
      <div className="board-settings-content">
        <div className="board-settings-read">
          <h3>{board.name}</h3>
          {renderedDescription === "" ? (
            <p className="empty-copy">No description.</p>
          ) : (
            <div
              className="markdown-content board-settings-description"
              dangerouslySetInnerHTML={{ __html: renderedDescription }}
            />
          )}
        </div>
        <div className="modal-actions">
          <button type="button" className="secondary-button" onClick={onDismiss}>
            Close
          </button>
          <button type="button" onClick={onBeginEdit}>
            Edit
          </button>
        </div>
      </div>
    );
  }

  return (
    <form className="workflow-form" onSubmit={onSubmit}>
      <label className="form-field form-field-wide">
        <span>Name</span>
        <input
          autoFocus
          required
          type="text"
          value={draft.name}
          onInput={(event) =>
            onChangeDraft({ ...draft, name: event.currentTarget.value })
          }
        />
      </label>
      <label className="form-field form-field-wide">
        <span>Description (Markdown)</span>
        <textarea
          rows={9}
          value={draft.descriptionSource}
          onInput={(event) =>
            onChangeDraft({
              ...draft,
              descriptionSource: event.currentTarget.value,
            })
          }
        />
      </label>
      {submission.kind === "error" && (
        <p className="form-error form-field-wide" role="alert">
          {submission.message}
        </p>
      )}
      <div className="modal-actions form-field-wide">
        <button
          type="button"
          className="secondary-button"
          disabled={submission.kind === "submitting"}
          onClick={onCancelEdit}
        >
          Cancel
        </button>
        <button type="submit" disabled={submission.kind === "submitting"}>
          {submission.kind === "submitting" ? "Saving" : "Save settings"}
        </button>
      </div>
    </form>
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

type BoardSettingsMode = "read" | "edit";

type BoardSettingsSubmission =
  | { kind: "idle" }
  | { kind: "submitting" }
  | { kind: "error"; message: string };

function failureMessage(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure);
}

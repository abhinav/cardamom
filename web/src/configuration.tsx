import type { MessageInitShape } from "@bufbuild/protobuf";
import { useMutation, useTransport } from "@connectrpc/connect-query";
import { useQuery } from "@tanstack/react-query";
import { Pencil, RotateCcw } from "lucide-react";
import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import { Link } from "react-router";

import {
  ConfigurationIssueIDStrategy,
  ConfigurationScope,
  ConfigurationService,
  type Configuration,
  type ConfigurationLayer,
  type ConfigurationOrigins,
  type ConfigurationOverrides,
  type ConfigurationSource,
  type ConfigurationView,
  UpdateConfigurationRequestSchema,
} from "./gen/cardamom/private/v1/configuration_pb.ts";
import { IssueFormDialog } from "./issue-form-dialog.tsx";
import { unaryRouteQueryOptions } from "./query-runtime.ts";

import "./configuration.css";

const maxByteLimit = 9_223_372_036_854_775_807n;

const configurationFields = [
  {
    path: "issue.id.prefix",
    label: "Issue ID prefix",
    input: "prefix",
  },
  {
    path: "issue.id.strategy",
    label: "Issue ID strategy",
    input: "strategy",
  },
  {
    path: "issue.summary.max_bytes",
    label: "Summary limit",
    input: "bytes",
  },
  {
    path: "attachment.max_bytes",
    label: "Attachment limit",
    input: "bytes",
  },
] as const;

export type ConfigurationFieldPath =
  (typeof configurationFields)[number]["path"];

type ConfigurationField = (typeof configurationFields)[number];

interface ConfigurationRouteProps {
  actor: string;
  boardId: string | undefined;
  boardName: string | undefined;
}

/** ConfigurationRoute selects the concrete-board configuration workflow. */
export function ConfigurationRoute({
  actor,
  boardId,
  boardName,
}: ConfigurationRouteProps) {
  if (boardId === undefined || boardName === undefined) {
    return (
      <section className="route-placeholder" aria-labelledby="configuration-title">
        <p className="route-kicker">Settings</p>
        <h1 id="configuration-title">Configuration</h1>
        <p>Select one board before viewing its resolved configuration.</p>
        <Link to="/">Return to Board</Link>
      </section>
    );
  }
  return (
    <BoardConfigurationRoute
      key={boardId}
      actor={actor}
      boardId={boardId}
      boardName={boardName}
    />
  );
}

interface BoardConfigurationRouteProps {
  actor: string;
  boardId: string;
  boardName: string;
}

function BoardConfigurationRoute({
  actor,
  boardId,
  boardName,
}: BoardConfigurationRouteProps) {
  const transport = useTransport();
  const updateConfiguration = useMutation(
    ConfigurationService.method.updateConfiguration,
  );
  const load = useQuery({
    ...unaryRouteQueryOptions(
      ConfigurationService.method.getConfiguration,
      { boardId },
      transport,
    ),
    select(response) {
      if (response.view === undefined) {
        throw new Error("GetConfiguration response did not include a view");
      }
      return response.view;
    },
  });
  const [view, setView] = useState<ConfigurationView>();
  const [editor, setEditor] = useState<ConfigurationEditor>();
  const [submission, setSubmission] = useState<ConfigurationSubmission>({
    kind: "idle",
  });

  useEffect(() => {
    if (load.data !== undefined) {
      setView(load.data);
    }
  }, [load.data]);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (editor === undefined || submission.kind === "submitting") {
      return;
    }
    const draft = editor.mode === "edit" ? editor.draft : undefined;
    const validation = editor.mode === "edit"
      ? validateConfigurationDraft(editor.field, editor.draft)
      : undefined;
    if (validation !== undefined) {
      return;
    }

    setSubmission({ kind: "submitting" });
    try {
      const response = await updateConfiguration.mutateAsync(
        configurationUpdateInput(
          boardId,
          actor,
          editor.scope,
          editor.field,
          draft,
        ),
      );
      if (response.view === undefined) {
        throw new Error("UpdateConfiguration response did not include a view");
      }
      setView(response.view);
      setEditor(undefined);
      setSubmission({ kind: "idle" });
    } catch (failure) {
      setSubmission({ kind: "error", message: failureMessage(failure) });
    }
  };

  if (view === undefined) {
    if (load.isError) {
      return (
        <ConfigurationLoadState
          message={load.error.message}
          retry={() => void load.refetch()}
        />
      );
    }
    return <ConfigurationLoadState message="Loading configuration" />;
  }

  const selectedLayer = editor === undefined
    ? undefined
    : layerForScope(view, editor.scope);
  return (
    <>
      <ConfigurationContent
        boardName={boardName}
        view={view}
        onBeginEdit={(mode, layer, field) => {
          const scope = layer.source?.scope ?? ConfigurationScope.UNSPECIFIED;
          const inherited = configurationInherited(view, scope, field.path);
          setEditor({
            mode,
            scope,
            field: field.path,
            draft: fieldOverride(layer.overrides, field.path) ?? inherited.raw,
          });
          setSubmission({ kind: "idle" });
        }}
      />
      {editor !== undefined && selectedLayer !== undefined && (
        <ConfigurationEditorDialog
          editor={editor}
          layer={selectedLayer}
          submission={submission}
          view={view}
          onCancel={() => {
            setEditor(undefined);
            setSubmission({ kind: "idle" });
          }}
          onChange={(draft) => {
            setEditor((current) =>
              current === undefined ? current : { ...current, draft },
            );
            setSubmission({ kind: "idle" });
          }}
          onSubmit={submit}
        />
      )}
    </>
  );
}

function ConfigurationLoadState({
  message,
  retry,
}: {
  message: string;
  retry?: () => void;
}) {
  return (
    <section className="configuration-load-state" role={retry ? "alert" : "status"}>
      <p>{message}</p>
      {retry !== undefined && (
        <button type="button" onClick={retry}>
          Retry
        </button>
      )}
    </section>
  );
}

interface ConfigurationContentProps {
  boardName: string;
  onBeginEdit: (
    mode: ConfigurationEditorMode,
    layer: ConfigurationLayer,
    field: ConfigurationField,
  ) => void;
  view: ConfigurationView;
}

/** ConfigurationContent renders effective values before their source layers. */
export function ConfigurationContent({
  boardName,
  onBeginEdit,
  view,
}: ConfigurationContentProps) {
  const store = layerForScope(view, ConfigurationScope.STORE);
  const board = layerForScope(view, ConfigurationScope.BOARD);
  return (
    <section className="configuration-page" aria-labelledby="configuration-title">
      <header className="configuration-heading">
        <div>
          <p className="route-kicker">Settings</p>
          <h1 id="configuration-title">Configuration</h1>
          <p className="configuration-intro">
            Effective values for the selected board, followed by every layer
            that contributes to them.
          </p>
        </div>
        <p className="configuration-context">
          <span>{store.source?.identity}</span>
          <span>{boardName} / {board.source?.identity}</span>
        </p>
      </header>

      <section className="configuration-effective" aria-labelledby="effective-title">
        <div className="configuration-section-heading">
          <h2 id="effective-title">Effective for {boardName}</h2>
          <p>Most specific value wins for each field.</p>
        </div>
        <div className="configuration-effective-grid">
          {configurationFields.map((field) => {
            const origin = fieldOrigin(view.origins, field.path);
            return (
              <div className="configuration-effective-field" key={field.path}>
                <span className="configuration-field-label">{field.label}</span>
                <strong>{fieldDisplayValue(field.path, fieldValue(view.effective, field.path))}</strong>
                <span className="configuration-origin">
                  <span aria-hidden="true" />
                  From {scopeLabel(origin?.scope)}
                </span>
              </div>
            );
          })}
        </div>
      </section>

      <section className="configuration-layers" aria-labelledby="layers-title">
        <div className="configuration-section-heading">
          <h2 id="layers-title">Resolution layers</h2>
          <p>Built-in → Store → Project → Board</p>
        </div>
        <div className="configuration-layer-list">
          {view.layers.map((layer, index) => (
            <ConfigurationLayerView
              key={`${layer.source?.scope ?? 0}-${layer.source?.identity ?? index}`}
              index={index}
              layer={layer}
              view={view}
              onBeginEdit={onBeginEdit}
            />
          ))}
        </div>
      </section>
    </section>
  );
}

function ConfigurationLayerView({
  index,
  layer,
  onBeginEdit,
  view,
}: {
  index: number;
  layer: ConfigurationLayer;
  onBeginEdit: ConfigurationContentProps["onBeginEdit"];
  view: ConfigurationView;
}) {
  const scope = layer.source?.scope ?? ConfigurationScope.UNSPECIFIED;
  const readOnly = scope === ConfigurationScope.BUILT_IN;
  return (
    <article className="configuration-layer">
      <header className="configuration-layer-heading">
        <span className="configuration-layer-number">{index + 1}</span>
        <div>
          <h3>{scopeLabel(scope)}</h3>
          <p>{layerIdentity(layer)}</p>
        </div>
        <span className="metadata-chip">{scopeImpact(scope)}</span>
      </header>
      <div className="configuration-layer-fields">
        {configurationFields.map((field) => {
          const override = fieldOverride(layer.overrides, field.path);
          const effective = fieldValue(view.effective, field.path);
          const origin = fieldOrigin(view.origins, field.path);
          return (
            <section className="configuration-layer-field" key={field.path}>
              <span className="configuration-field-label">{field.label}</span>
              <code>{field.path}</code>
              <strong>
                {override === undefined
                  ? "Inherited"
                  : fieldDisplayValue(field.path, override)}
              </strong>
              <span className="configuration-layer-effective">
                Effective for board: {fieldDisplayValue(field.path, effective)} from {scopeLabel(origin?.scope)}
              </span>
              {!readOnly && (
                <div className="configuration-field-actions">
                  <button
                    type="button"
                    className="secondary-button"
                    onClick={() => onBeginEdit("edit", layer, field)}
                  >
                    <Pencil aria-hidden="true" />
                    Edit
                  </button>
                  {override !== undefined && (
                    <button
                      type="button"
                      className="danger-button"
                      onClick={() => onBeginEdit("reset", layer, field)}
                    >
                      <RotateCcw aria-hidden="true" />
                      Reset
                    </button>
                  )}
                </div>
              )}
            </section>
          );
        })}
      </div>
    </article>
  );
}

function ConfigurationEditorDialog({
  editor,
  layer,
  onCancel,
  onChange,
  onSubmit,
  submission,
  view,
}: {
  editor: ConfigurationEditor;
  layer: ConfigurationLayer;
  onCancel: () => void;
  onChange: (draft: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  submission: ConfigurationSubmission;
  view: ConfigurationView;
}) {
  const field = fieldForPath(editor.field);
  const resetting = editor.mode === "reset";
  const validation = resetting
    ? undefined
    : validateConfigurationDraft(editor.field, editor.draft);
  const inherited = configurationInherited(view, editor.scope, editor.field);
  const preview = configurationPreview(
    view,
    editor.scope,
    editor.field,
    resetting ? undefined : editor.draft,
  );
  const override = fieldOverride(layer.overrides, editor.field);
  const titleId = "configuration-editor-title";
  return (
    <IssueFormDialog
      title={resetting ? `Reset ${field.label}` : `Edit ${field.label}`}
      titleId={titleId}
      description={`${scopeLabel(editor.scope)} · ${layer.source?.identity ?? "Unknown source"}`}
      onSubmit={onSubmit}
      actions={(
        <>
          <button
            type="button"
            className="secondary-button"
            disabled={submission.kind === "submitting"}
            onClick={onCancel}
          >
            Cancel
          </button>
          <button
            type="submit"
            className={resetting ? "danger-button" : undefined}
            disabled={submission.kind === "submitting" || validation !== undefined}
          >
            {submission.kind === "submitting"
              ? (resetting ? "Resetting" : "Saving")
              : (resetting ? "Reset override" : "Save")}
          </button>
        </>
      )}
    >
      {resetting ? (
        <div className="configuration-reset-summary form-field-wide">
          <p>
            Remove <strong>{fieldDisplayValue(editor.field, override)}</strong> from {scopeLabel(editor.scope)}.
          </p>
          <p>
            This layer will inherit <strong>{inherited.value}</strong> from {inherited.source}.
          </p>
        </div>
      ) : (
        <ConfigurationFieldInput
          draft={editor.draft}
          field={field}
          validation={validation}
          onChange={onChange}
        />
      )}
      <div className="configuration-preview form-field-wide">
        <span>Effective for board</span>
        <strong>{preview.value}</strong>
        <span>From {preview.source}</span>
      </div>
      {submission.kind === "error" && (
        <p className="form-error form-field-wide" role="alert">
          {submission.message}
        </p>
      )}
    </IssueFormDialog>
  );
}

function ConfigurationFieldInput({
  draft,
  field,
  onChange,
  validation,
}: {
  draft: string;
  field: ConfigurationField;
  onChange: (draft: string) => void;
  validation: string | undefined;
}) {
  const errorId = "configuration-field-error";
  return (
    <label className="form-field form-field-wide">
      <span>{field.label}</span>
      {field.input === "strategy" ? (
        <select
          autoFocus
          value={draft}
          onChange={(event) => onChange(event.currentTarget.value)}
        >
          <option value="random">random</option>
          <option value="sequential">sequential</option>
        </select>
      ) : (
        <input
          autoFocus
          type={field.input === "bytes" ? "number" : "text"}
          inputMode={field.input === "bytes" ? "numeric" : "text"}
          min={field.input === "bytes" ? "1" : undefined}
          max={field.input === "bytes" ? maxByteLimit.toString() : undefined}
          value={draft}
          aria-invalid={validation !== undefined}
          aria-describedby={validation === undefined ? undefined : errorId}
          onInput={(event) => onChange(event.currentTarget.value)}
        />
      )}
      {validation !== undefined && (
        <span className="form-error" id={errorId} role="alert">
          {validation}
        </span>
      )}
    </label>
  );
}

interface ConfigurationEditor {
  draft: string;
  field: ConfigurationFieldPath;
  mode: ConfigurationEditorMode;
  scope: ConfigurationScope;
}

type ConfigurationEditorMode = "edit" | "reset";

type ConfigurationSubmission =
  | { kind: "idle" }
  | { kind: "submitting" }
  | { kind: "error"; message: string };

/** configurationUpdateInput builds one typed field-mask mutation. */
export function configurationUpdateInput(
  boardId: string,
  actor: string,
  scope: ConfigurationScope,
  field: ConfigurationFieldPath,
  draft: string | undefined,
): MessageInitShape<typeof UpdateConfigurationRequestSchema> {
  const overrides: MessageInitShape<typeof UpdateConfigurationRequestSchema>["overrides"] = {};
  if (draft !== undefined) {
    switch (field) {
      case "issue.id.prefix":
        overrides.issue = { id: { prefix: draft } };
        break;
      case "issue.id.strategy":
        overrides.issue = { id: { strategy: strategyValue(draft) } };
        break;
      case "issue.summary.max_bytes":
        overrides.issue = { summary: { maxBytes: BigInt(draft) } };
        break;
      case "attachment.max_bytes":
        overrides.attachment = { maxBytes: BigInt(draft) };
        break;
    }
  }
  return {
    boardId,
    scope,
    overrides,
    updateMask: { paths: [field] },
    context: { actor: actor.trim() },
  };
}

/** validateConfigurationDraft mirrors the stable configuration value boundary. */
export function validateConfigurationDraft(
  field: ConfigurationFieldPath,
  draft: string,
): string | undefined {
  switch (field) {
    case "issue.id.prefix":
      return draft.length <= 16 && /^[a-z0-9-]+-$/.test(draft)
        ? undefined
        : "Use lowercase letters, digits, or dashes, end with a dash, and limit the prefix to 16 characters.";
    case "issue.id.strategy":
      return draft === "random" || draft === "sequential"
        ? undefined
        : "Select random or sequential.";
    case "issue.summary.max_bytes":
    case "attachment.max_bytes": {
      try {
        const value = BigInt(draft);
        return /^\d+$/.test(draft) && value >= 1n && value <= maxByteLimit
          ? undefined
          : `Enter a whole number of bytes between 1 and ${maxByteLimit}.`;
      } catch {
        return `Enter a whole number of bytes between 1 and ${maxByteLimit}.`;
      }
    }
  }
}

/** configurationPreview resolves one prospective layer edit through precedence. */
export function configurationPreview(
  view: ConfigurationView,
  scope: ConfigurationScope,
  field: ConfigurationFieldPath,
  draft: string | undefined,
): { value: string; source: string } {
  if (
    draft !== undefined &&
    field.endsWith("max_bytes") &&
    validateConfigurationDraft(field, draft) !== undefined
  ) {
    return {
      value: fieldDisplayValue(field, fieldValue(view.effective, field)),
      source: scopeLabel(fieldOrigin(view.origins, field)?.scope),
    };
  }
  let raw = "";
  let source = "Built-in";
  for (const layer of view.layers) {
    const layerScope = layer.source?.scope ?? ConfigurationScope.UNSPECIFIED;
    const candidate = layerScope === scope
      ? draft
      : fieldOverride(layer.overrides, field);
    if (candidate !== undefined) {
      raw = candidate;
      source = scopeLabel(layerScope);
    }
  }
  return { value: fieldDisplayValue(field, raw), source };
}

function configurationInherited(
  view: ConfigurationView,
  scope: ConfigurationScope,
  field: ConfigurationFieldPath,
): { raw: string; value: string; source: string } {
  let raw = "";
  let source = "Built-in";
  for (const layer of view.layers) {
    const layerScope = layer.source?.scope ?? ConfigurationScope.UNSPECIFIED;
    if (layerScope === scope) {
      break;
    }
    const candidate = fieldOverride(layer.overrides, field);
    if (candidate !== undefined) {
      raw = candidate;
      source = scopeLabel(layerScope);
    }
  }
  return { raw, value: fieldDisplayValue(field, raw), source };
}

function fieldForPath(path: ConfigurationFieldPath): ConfigurationField {
  const field = configurationFields.find((candidate) => candidate.path === path);
  if (field === undefined) {
    throw new Error(`Unknown configuration field ${path}`);
  }
  return field;
}

function layerForScope(
  view: ConfigurationView,
  scope: ConfigurationScope,
): ConfigurationLayer {
  const layer = view.layers.find((candidate) => candidate.source?.scope === scope);
  if (layer === undefined) {
    throw new Error(`Configuration view did not include ${scopeLabel(scope)}`);
  }
  return layer;
}

function fieldValue(
  configuration: Configuration | undefined,
  field: ConfigurationFieldPath,
): string {
  switch (field) {
    case "issue.id.prefix":
      return configuration?.issue?.id?.prefix ?? "";
    case "issue.id.strategy":
      return strategyName(configuration?.issue?.id?.strategy);
    case "issue.summary.max_bytes":
      return configuration?.issue?.summary?.maxBytes.toString() ?? "";
    case "attachment.max_bytes":
      return configuration?.attachment?.maxBytes.toString() ?? "";
  }
}

function fieldOverride(
  overrides: ConfigurationOverrides | undefined,
  field: ConfigurationFieldPath,
): string | undefined {
  switch (field) {
    case "issue.id.prefix":
      return overrides?.issue?.id?.prefix;
    case "issue.id.strategy":
      return overrides?.issue?.id?.strategy === undefined
        ? undefined
        : strategyName(overrides.issue.id.strategy);
    case "issue.summary.max_bytes":
      return overrides?.issue?.summary?.maxBytes?.toString();
    case "attachment.max_bytes":
      return overrides?.attachment?.maxBytes?.toString();
  }
}

function fieldOrigin(
  origins: ConfigurationOrigins | undefined,
  field: ConfigurationFieldPath,
): ConfigurationSource | undefined {
  switch (field) {
    case "issue.id.prefix":
      return origins?.issue?.id?.prefix;
    case "issue.id.strategy":
      return origins?.issue?.id?.strategy;
    case "issue.summary.max_bytes":
      return origins?.issue?.summary?.maxBytes;
    case "attachment.max_bytes":
      return origins?.attachment?.maxBytes;
  }
}

function fieldDisplayValue(field: ConfigurationFieldPath, raw: string | undefined): string {
  if (raw === undefined || raw === "") {
    return "Unavailable";
  }
  return field.endsWith("max_bytes") ? formatBytes(BigInt(raw)) : raw;
}

function formatBytes(bytes: bigint): string {
  const mebibyte = 1_048_576n;
  const kibibyte = 1_024n;
  if (bytes % mebibyte === 0n) {
    return `${bytes / mebibyte} MiB`;
  }
  if (bytes % kibibyte === 0n) {
    return `${bytes / kibibyte} KiB`;
  }
  return `${bytes} bytes`;
}

function strategyValue(value: string): ConfigurationIssueIDStrategy {
  switch (value) {
    case "random":
      return ConfigurationIssueIDStrategy.CONFIGURATION_ISSUE_ID_STRATEGY_RANDOM;
    case "sequential":
      return ConfigurationIssueIDStrategy.CONFIGURATION_ISSUE_ID_STRATEGY_SEQUENTIAL;
    default:
      throw new Error(`Invalid issue ID strategy ${value}`);
  }
}

function strategyName(value: ConfigurationIssueIDStrategy | undefined): string {
  switch (value) {
    case ConfigurationIssueIDStrategy.CONFIGURATION_ISSUE_ID_STRATEGY_RANDOM:
      return "random";
    case ConfigurationIssueIDStrategy.CONFIGURATION_ISSUE_ID_STRATEGY_SEQUENTIAL:
      return "sequential";
    default:
      return "";
  }
}

function scopeLabel(scope: ConfigurationScope | undefined): string {
  switch (scope) {
    case ConfigurationScope.BUILT_IN:
      return "Built-in";
    case ConfigurationScope.STORE:
      return "Store";
    case ConfigurationScope.PROJECT:
      return "Project";
    case ConfigurationScope.BOARD:
      return "Board";
    default:
      return "Unknown";
  }
}

function layerIdentity(layer: ConfigurationLayer): string {
  return layer.source?.scope === ConfigurationScope.BUILT_IN
    ? "Cardamom defaults"
    : (layer.source?.identity ?? "Unknown source");
}

function scopeImpact(scope: ConfigurationScope): string {
  switch (scope) {
    case ConfigurationScope.BUILT_IN:
      return "Read only";
    case ConfigurationScope.STORE:
      return "Local to this store";
    case ConfigurationScope.PROJECT:
      return "All boards in project";
    case ConfigurationScope.BOARD:
      return "This board only";
    default:
      return "Unknown scope";
  }
}

function failureMessage(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure);
}

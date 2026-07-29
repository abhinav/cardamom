import { IssueType } from "./gen/cardamom/private/v1/issue_pb.ts";

/** IssueMetadataDraft is the controlled form representation shared by create and edit. */
export interface IssueMetadataDraft {
  title: string;
  type: IssueType;
  priority: number;
  summary: string;
  details: string;
  labels: string;
  parent: string;
}

interface IssueMetadataFieldsProps {
  autoFocusTitle?: boolean;
  disabled: boolean;
  draft: IssueMetadataDraft;
  onChange: <Field extends keyof IssueMetadataDraft>(
    field: Field,
    value: IssueMetadataDraft[Field],
  ) => void;
}

/** IssueMetadataFields renders the complete metadata contract shared by create and edit. */
export function IssueMetadataFields({
  autoFocusTitle = false,
  disabled,
  draft,
  onChange,
}: IssueMetadataFieldsProps) {
  return (
    <>
      <label className="form-field form-field-wide">
        <span>Title</span>
        <input
          autoFocus={autoFocusTitle}
          disabled={disabled}
          required
          type="text"
          value={draft.title}
          onInput={(event) => onChange("title", event.currentTarget.value)}
        />
      </label>
      <label className="form-field">
        <span>Type</span>
        <select
          disabled={disabled}
          value={draft.type}
          onChange={(event) =>
            onChange(
              "type",
              Number(event.currentTarget.value) as IssueType,
            )
          }
        >
          <option value={IssueType.WORKSTREAM}>Workstream</option>
          <option value={IssueType.TASK}>Task</option>
          <option value={IssueType.CHECKPOINT}>Checkpoint</option>
          <option value={IssueType.ROUTINE}>Routine</option>
        </select>
      </label>
      <label className="form-field">
        <span>Priority</span>
        <input
          type="number"
          min="0"
          max="4"
          disabled={disabled}
          required
          value={draft.priority}
          onInput={(event) =>
            onChange("priority", Number(event.currentTarget.value))
          }
        />
      </label>
      <label className="form-field form-field-wide">
        <span>Summary (Markdown)</span>
        <textarea
          rows={4}
          disabled={disabled}
          value={draft.summary}
          onInput={(event) => onChange("summary", event.currentTarget.value)}
        />
      </label>
      <label className="form-field form-field-wide">
        <span>Details (Markdown)</span>
        <textarea
          rows={7}
          disabled={disabled}
          value={draft.details}
          onInput={(event) => onChange("details", event.currentTarget.value)}
        />
      </label>
      <label className="form-field form-field-wide">
        <span>Labels</span>
        <input
          type="text"
          disabled={disabled}
          placeholder="area:web, priority:high"
          value={draft.labels}
          onInput={(event) => onChange("labels", event.currentTarget.value)}
        />
      </label>
      <label className="form-field">
        <span>Parent</span>
        <input
          type="text"
          disabled={disabled}
          placeholder="Optional issue ID"
          value={draft.parent}
          onInput={(event) => onChange("parent", event.currentTarget.value)}
        />
      </label>
    </>
  );
}

/** metadataLabels normalizes the form's comma- or newline-separated labels. */
export function metadataLabels(value: string): string[] {
  return [
    ...new Set(
      value
        .split(/[,\n]/)
        .map((part) => part.trim())
        .filter(Boolean),
    ),
  ];
}

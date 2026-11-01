import { useId } from "react";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

import { IssueType } from "./gen/cardamom/private/v1/issue_pb.ts";

/** IssueMetadataDraft retains unresolved fields as undefined; edits omit them. */
export interface IssueMetadataDraft {
  title: string | undefined;
  type: IssueType | undefined;
  priority: number | undefined;
  summary: string | undefined;
  details: string | undefined;
  labels: string | undefined;
  parent: string | undefined;
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
  const titleId = useId();
  const typeId = useId();
  const priorityId = useId();
  const summaryId = useId();
  const detailsId = useId();
  const labelsId = useId();
  const parentId = useId();
  return (
    <>
      <div className="form-field form-field-wide">
        <Label htmlFor={titleId}>Title</Label>
        <Input
          id={titleId}
          autoFocus={autoFocusTitle}
          disabled={disabled || draft.title === undefined}
          required
          type="text"
          value={draft.title ?? ""}
          onChange={(event) => onChange("title", event.currentTarget.value)}
          placeholder={draft.title === undefined ? "Unresolved title" : undefined}
        />
      </div>
      <div className="form-field">
        <Label htmlFor={typeId}>Type</Label>
        <Select
          disabled={disabled || draft.type === undefined}
          value={draft.type === undefined ? "" : String(draft.type)}
          onValueChange={(value) => {
            if (value !== null) {
              onChange("type", Number(value) as IssueType);
            }
          }}
        >
          <SelectTrigger id={typeId} className="w-full">
            <SelectValue placeholder="Unresolved type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={String(IssueType.WORKSTREAM)}>Workstream</SelectItem>
            <SelectItem value={String(IssueType.TASK)}>Task</SelectItem>
            <SelectItem value={String(IssueType.CHECKPOINT)}>Checkpoint</SelectItem>
            <SelectItem value={String(IssueType.ROUTINE)}>Routine</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="form-field">
        <Label htmlFor={priorityId}>Priority</Label>
        <Input
          id={priorityId}
          type="number"
          min="0"
          max="4"
          disabled={disabled || draft.priority === undefined}
          required
          value={draft.priority ?? ""}
          onChange={(event) =>
            onChange("priority", Number(event.currentTarget.value))
          }
          placeholder={draft.priority === undefined ? "Unresolved priority" : undefined}
        />
      </div>
      <div className="form-field form-field-wide">
        <Label htmlFor={summaryId}>Summary (Markdown)</Label>
        <Textarea
          id={summaryId}
          rows={4}
          disabled={disabled || draft.summary === undefined}
          value={draft.summary ?? ""}
          onChange={(event) => onChange("summary", event.currentTarget.value)}
          placeholder={draft.summary === undefined ? "Unresolved summary" : undefined}
        />
      </div>
      <div className="form-field form-field-wide">
        <Label htmlFor={detailsId}>Details (Markdown)</Label>
        <Textarea
          id={detailsId}
          rows={7}
          disabled={disabled || draft.details === undefined}
          value={draft.details ?? ""}
          onChange={(event) => onChange("details", event.currentTarget.value)}
          placeholder={draft.details === undefined ? "Unresolved details" : undefined}
        />
      </div>
      <div className="form-field form-field-wide">
        <Label htmlFor={labelsId}>Labels</Label>
        <Input
          id={labelsId}
          type="text"
          disabled={disabled || draft.labels === undefined}
            placeholder={draft.labels === undefined ? "Unresolved labels" : "area:web, priority:high"}
          value={draft.labels ?? ""}
          onChange={(event) => onChange("labels", event.currentTarget.value)}
        />
      </div>
      <div className="form-field">
        <Label htmlFor={parentId}>Parent</Label>
        <Input
          id={parentId}
          type="text"
          disabled={disabled || draft.parent === undefined}
            placeholder={draft.parent === undefined ? "Unresolved parent" : "Optional issue ID"}
          value={draft.parent ?? ""}
          onChange={(event) => onChange("parent", event.currentTarget.value)}
        />
      </div>
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

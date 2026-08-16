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
          disabled={disabled}
          required
          type="text"
          value={draft.title}
          onChange={(event) => onChange("title", event.currentTarget.value)}
        />
      </div>
      <div className="form-field">
        <Label htmlFor={typeId}>Type</Label>
        <Select
          disabled={disabled}
          value={String(draft.type)}
          onValueChange={(value) => {
            if (value !== null) {
              onChange("type", Number(value) as IssueType);
            }
          }}
        >
          <SelectTrigger id={typeId} className="w-full">
            <SelectValue />
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
          disabled={disabled}
          required
          value={draft.priority}
          onChange={(event) =>
            onChange("priority", Number(event.currentTarget.value))
          }
        />
      </div>
      <div className="form-field form-field-wide">
        <Label htmlFor={summaryId}>Summary (Markdown)</Label>
        <Textarea
          id={summaryId}
          rows={4}
          disabled={disabled}
          value={draft.summary}
          onChange={(event) => onChange("summary", event.currentTarget.value)}
        />
      </div>
      <div className="form-field form-field-wide">
        <Label htmlFor={detailsId}>Details (Markdown)</Label>
        <Textarea
          id={detailsId}
          rows={7}
          disabled={disabled}
          value={draft.details}
          onChange={(event) => onChange("details", event.currentTarget.value)}
        />
      </div>
      <div className="form-field form-field-wide">
        <Label htmlFor={labelsId}>Labels</Label>
        <Input
          id={labelsId}
          type="text"
          disabled={disabled}
          placeholder="area:web, priority:high"
          value={draft.labels}
          onChange={(event) => onChange("labels", event.currentTarget.value)}
        />
      </div>
      <div className="form-field">
        <Label htmlFor={parentId}>Parent</Label>
        <Input
          id={parentId}
          type="text"
          disabled={disabled}
          placeholder="Optional issue ID"
          value={draft.parent}
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

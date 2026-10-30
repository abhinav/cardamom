import type { MessageInitShape } from "@bufbuild/protobuf";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type {
  ConflictInspection,
  ConflictRelationsRepairSchema,
  ConflictExecutionRepairSchema,
} from "../gen/cardamom/private/v1/conflicts_pb.ts";
import { ConflictIssueLinks, conflictUnitLabel } from "./conflict-values.tsx";

type RelationsRepair = MessageInitShape<typeof ConflictRelationsRepairSchema>;
type ExecutionRepair = MessageInitShape<typeof ConflictExecutionRepairSchema>;

/** ParentDraft distinguishes an unfinished choice from explicit unparenting. */
export interface ParentDraft {
  child: string;
  parent: string;
  action: string;
}

/** relationRepairInput parses complete browser choices into a compound repair. */
export function relationRepairInput(
  remove: string[],
  parents: ParentDraft[],
): RelationsRepair {
  return {
    remove,
    parents: parents.map((value) => {
      if (
        !value.child.trim() ||
        !["set", "remove"].includes(value.action) ||
        (value.action === "set" && !value.parent.trim())
      ) {
        throw new Error(
          "Choose a child and either a parent or Remove parent for every row.",
        );
      }
      return {
        childId: value.child.trim(),
        ...(value.action === "set" ? { parentId: value.parent.trim() } : {}),
      };
    }),
  };
}

/** RelationsRepairForm submits finite removals and parent choices together. */
export function RelationsRepairForm({
  inspection,
  disabled,
  submit,
}: {
  inspection: ConflictInspection;
  disabled: boolean;
  submit: (value: RelationsRepair) => void;
}) {
  const [remove, setRemove] = useState<string[]>([]);
  const [parents, setParents] = useState<ParentDraft[]>([]);
  const [error, setError] = useState<string>();
  return (
    <form
      className="conflict-repair"
      onSubmit={(event) => {
        event.preventDefault();
        try {
          const value = relationRepairInput(remove, parents);
          setError(undefined);
          submit(value);
        } catch (error) {
          setError((error as Error).message);
        }
      }}
    >
      <h3>Repair relationships</h3>
      <p>
        Remove contributing assignments or choose parents. The complete repair
        must leave a valid graph.
      </p>
      {inspection.alternatives.map((alternative, index) => {
        const reference = inspection.issues.find(
          (value) => value.uid === alternative.unit?.issueUid,
        )?.issue;
        return (
          <div className="conflict-remove" key={alternative.id}>
            <Label>
              <Checkbox
                disabled={disabled}
                checked={remove.includes(alternative.id)}
                onCheckedChange={(checked) =>
                  setRemove((values) =>
                    checked
                      ? [...values, alternative.id]
                      : values.filter((id) => id !== alternative.id),
                  )
                }
              />
              Remove value {index + 1}: {conflictUnitLabel(alternative.unit)}
            </Label>
            {reference && <ConflictIssueLinks issues={[reference]} />}
          </div>
        );
      })}
      {parents.map((draft, index) => (
        <fieldset className="conflict-parent" key={index} disabled={disabled}>
          <legend>Parent choice {index + 1}</legend>
          <Label>
            Child issue ID
            <Input
              value={draft.child}
              onChange={(event) =>
                setParents((rows) =>
                  rows.map((row, i) =>
                    i === index ? { ...row, child: event.target.value } : row,
                  ),
                )
              }
            />
          </Label>
          <RepairChoice
            label="Parent action"
            value={draft.action}
            choices={[
              { value: "set", label: "Set parent" },
              { value: "remove", label: "Remove parent" },
            ]}
            onChange={(action) =>
              setParents((rows) =>
                rows.map((row, i) => (i === index ? { ...row, action } : row)),
              )
            }
          />
          {draft.action === "set" && (
            <Label>
              Parent issue ID
              <Input
                value={draft.parent}
                onChange={(event) =>
                  setParents((rows) =>
                    rows.map((row, i) =>
                      i === index
                        ? { ...row, parent: event.target.value }
                        : row,
                    ),
                  )
                }
              />
            </Label>
          )}
          <Button
            variant="outline"
            type="button"
            onClick={() =>
              setParents((rows) => rows.filter((_, i) => i !== index))
            }
          >
            Remove choice
          </Button>
        </fieldset>
      ))}
      <Button
        variant="outline"
        type="button"
        disabled={disabled}
        onClick={() =>
          setParents((rows) => [...rows, { child: "", parent: "", action: "" }])
        }
      >
        Add parent choice
      </Button>
      {error && <p role="alert">{error}</p>}
      <Button type="submit" disabled={disabled}>
        Apply relationship repair
      </Button>
    </form>
  );
}

/** ExecutionRepairForm leaves kind and lifecycle unchanged unless selected. */
export function ExecutionRepairForm({
  inspection,
  disabled,
  submit,
}: {
  inspection: ConflictInspection;
  disabled: boolean;
  submit: (value: ExecutionRepair) => void;
}) {
  const [waiting, setWaiting] = useState("");
  const [assignment, setAssignment] = useState("");
  const [kind, setKind] = useState("");
  const [lifecycle, setLifecycle] = useState("");
  const reasons = inspection.alternatives
    .map((alternative, index) => ({ alternative, index }))
    .filter(({ alternative }) => alternative.waitingPresent);
  return (
    <form
      className="conflict-repair"
      onSubmit={(event) => {
        event.preventDefault();
        submit({
          waiting,
          waitingAssignment: waiting === "keep" ? assignment : "",
          kind: kind || undefined,
          lifecycle: lifecycle || undefined,
        });
      }}
    >
      <h3>Repair execution state</h3>
      <p>
        Choose waiting explicitly. Choose type or lifecycle only when replacing
        their retained or current values.
      </p>
      <RepairChoice
        label="Waiting decision"
        value={waiting}
        disabled={disabled}
        choices={[
          { value: "keep", label: "Keep waiting" },
          { value: "clear", label: "Clear waiting" },
        ]}
        onChange={setWaiting}
      />
      {waiting === "keep" && reasons.length > 1 && (
        <RepairChoice
          label="Waiting value"
          value={assignment}
          disabled={disabled}
          choices={reasons.map(({ alternative, index }) => ({
            value: alternative.id,
            label: `Value ${index + 1}`,
          }))}
          onChange={setAssignment}
        />
      )}
      <RepairChoice
        label="Type replacement"
        value={kind}
        disabled={disabled}
        choices={[
          { value: "", label: "Keep current or retained type" },
          ...["task", "workstream", "checkpoint", "routine"].map((value) => ({
            value,
            label: value,
          })),
        ]}
        onChange={setKind}
      />
      <RepairChoice
        label="Lifecycle replacement"
        value={lifecycle}
        disabled={disabled}
        choices={[
          { value: "", label: "Keep current or retained lifecycle" },
          ...["open", "closed", "cancelled"].map((value) => ({
            value,
            label: value,
          })),
        ]}
        onChange={setLifecycle}
      />
      <Button
        type="submit"
        disabled={
          disabled ||
          !waiting ||
          (waiting === "keep" && reasons.length > 1 && !assignment)
        }
      >
        Apply execution repair
      </Button>
    </form>
  );
}

function RepairChoice({
  label,
  value,
  choices,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  choices: { value: string; label: string }[];
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <div className="conflict-choice">
      <span>{label}</span>
      <Select
        value={value}
        disabled={disabled}
        onValueChange={(value) => onChange(value ?? "")}
      >
        <SelectTrigger aria-label={label}>
          <SelectValue placeholder="Choose…" />
        </SelectTrigger>
        <SelectContent>
          {choices.map((choice) => (
            <SelectItem key={choice.value} value={choice.value}>
              {choice.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

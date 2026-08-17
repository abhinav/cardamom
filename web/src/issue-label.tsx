/** SelectLabel opens a scoped collection filtered to one label. */
export type SelectLabel = (label: string) => void;

/** IssueLabel renders one label as an explicit collection-navigation control. */
export function IssueLabel({
  label,
  select,
}: {
  label: string;
  select: SelectLabel;
}) {
  return (
    <Button
      type="button"
      className="metadata-chip issue-label"
      variant="secondary"
      size="xs"
      title={`Show all issues labeled ${label}`}
      onClick={() => select(label)}
    >
      {label}
    </Button>
  );
}
import { Button } from "@/components/ui/button";

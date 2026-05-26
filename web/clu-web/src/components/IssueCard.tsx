import { Link } from '@tanstack/react-router'
import type { Issue } from '../lib/api'
import { type Status } from '../lib/issue-display'
import { PriorityBadge, StatusBadge } from './StatusBadge'
import { Badge } from './ui/badge'

interface Props {
  issue: Issue
  draggable?: boolean
  // Workflow-internal labels are hidden on cards by default — visual
  // noise on the board. Detail pages override and show them all.
  showAllLabels?: boolean
  // Hide the status pill on cards already grouped by status (kanban).
  hideStatus?: boolean
}

// IssueCard — compact preview of an issue. Lives on the kanban board
// and could be reused in list-as-cards layouts. Drag uses native
// HTML5; the dataTransfer carries the issue ID so column drop zones
// can wire to the same API regardless of dnd library choice.
export default function IssueCard({
  issue,
  draggable,
  showAllLabels,
  hideStatus,
}: Props) {
  const labels = showAllLabels
    ? issue.labels
    : issue.labels.filter((l) => !isManaged(l))
  return (
    <Link
      to="/issues/$id"
      params={{ id: issue.id }}
      draggable={draggable}
      onDragStart={
        draggable
          ? (e) => {
              e.dataTransfer.setData('text/x-clu-issue-id', issue.id)
              e.dataTransfer.effectAllowed = 'move'
            }
          : undefined
      }
      className="bg-card hover:bg-accent/40 group relative flex flex-col gap-2 rounded-md border p-2.5 text-card-foreground no-underline shadow-xs transition-colors"
    >
      <div className="flex items-start gap-2">
        <PriorityBadge priority={issue.priority} className="shrink-0" />
        <span className="flex-1 text-[13px] leading-snug font-medium">
          {issue.title}
        </span>
      </div>
      <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
        <code className="text-muted-foreground bg-muted/50 rounded px-1.5 py-0.5 font-mono tabular">
          {issue.id}
        </code>
        {!hideStatus && (
          <StatusBadge
            status={issue.status as Status}
            blocked={issue.blocked}
          />
        )}
        {issue.assignee && (
          <span className="text-muted-foreground inline-flex items-center gap-1">
            <span className="bg-accent text-accent-foreground inline-flex size-4 items-center justify-center rounded-full text-[9px] font-semibold uppercase">
              {issue.assignee.charAt(0)}
            </span>
            <span>{issue.assignee}</span>
          </span>
        )}
        {labels.slice(0, 3).map((l) => (
          <Badge key={l} variant="muted">
            {l}
          </Badge>
        ))}
        {labels.length > 3 && (
          <span className="text-muted-foreground">+{labels.length - 3}</span>
        )}
      </div>
    </Link>
  )
}

function isManaged(label: string): boolean {
  return /^(run|step|checkpoint|cap|template):/.test(label)
}

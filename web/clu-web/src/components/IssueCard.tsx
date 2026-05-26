import { Link } from '@tanstack/react-router'
import type { Issue } from '../lib/api'
import {
  displayStatusClass,
  displayStatusLabel,
  priorityClass,
  type Status,
} from '../lib/issue-display'

interface Props {
  issue: Issue
  draggable?: boolean
  // Workflow-internal labels are hidden by default on cards — they're
  // noise in the kanban view. Set to true on detail pages.
  showAllLabels?: boolean
}

// IssueCard renders a single issue as a compact card. Used by both
// the kanban board (with draggable=true) and as a list-view detail.
export default function IssueCard({ issue, draggable, showAllLabels }: Props) {
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
      className="block rounded-lg border border-[var(--line)] bg-[var(--surface-strong)] p-3 text-left no-underline shadow-sm transition hover:border-[var(--lagoon-deep)]/40 hover:shadow-md"
    >
      <div className="flex items-start gap-2">
        <span
          className={`shrink-0 rounded px-1.5 py-0.5 text-xs font-mono font-semibold ${priorityClass(issue.priority)}`}
          title={`priority ${issue.priority}`}
        >
          p{issue.priority}
        </span>
        <span className="flex-1 text-sm font-medium leading-snug text-[var(--sea-ink)]">
          {issue.title}
        </span>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1.5 text-xs">
        <code className="rounded bg-[var(--chip-bg)] px-1.5 py-0.5 text-[var(--sea-ink-soft)]">
          {issue.id}
        </code>
        <span
          className={`rounded px-1.5 py-0.5 font-medium ${displayStatusClass(issue.status as Status, issue.blocked)}`}
        >
          {displayStatusLabel(issue.status as Status, issue.blocked)}
        </span>
        {issue.assignee && (
          <span className="rounded bg-[var(--chip-bg)] px-1.5 py-0.5 text-[var(--sea-ink-soft)]">
            @{issue.assignee}
          </span>
        )}
        {labels.map((l) => (
          <span
            key={l}
            className="rounded bg-zinc-500/10 px-1.5 py-0.5 text-[var(--sea-ink-soft)]"
          >
            {l}
          </span>
        ))}
      </div>
    </Link>
  )
}

function isManaged(label: string): boolean {
  return /^(run|step|checkpoint|cap|template):/.test(label)
}

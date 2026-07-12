// Shared display helpers for rendering issues. Keep colour tokens
// next to the status strings so list / kanban / detail stay in sync.

export type Status = 'open' | 'in_progress' | 'closed' | 'cancelled'

export const STATUS_LABEL: Record<Status, string> = {
  open: 'Open',
  in_progress: 'In progress',
  closed: 'Closed',
  cancelled: 'Cancelled',
}

// Tailwind classes for the status pill. Background + foreground only;
// callers wrap with their own padding/rounding.
export const STATUS_CLASS: Record<Status, string> = {
  open: 'bg-emerald-500/12 text-emerald-700 dark:text-emerald-300',
  in_progress: 'bg-amber-500/12 text-amber-700 dark:text-amber-300',
  closed: 'bg-zinc-500/12 text-zinc-600 dark:text-zinc-400',
  cancelled: 'bg-rose-500/12 text-rose-700 dark:text-rose-300',
}

// Derived status used for display: open + blocked → "blocked" pill.
// Mirrors internal/cli's displayStatus.
export function displayStatus(status: Status, blocked: boolean): string {
  if (status === 'open' && blocked) return 'blocked'
  return status
}

export function displayStatusLabel(status: Status, blocked: boolean): string {
  if (status === 'open' && blocked) return 'Blocked'
  return STATUS_LABEL[status]
}

export function displayStatusClass(status: Status, blocked: boolean): string {
  if (status === 'open' && blocked) {
    return 'bg-rose-500/12 text-rose-700 dark:text-rose-300'
  }
  return STATUS_CLASS[status]
}

// Priority 0 is highest. Map to a colour from urgent → relaxed.
export function priorityClass(p: number): string {
  if (p <= 0) return 'bg-rose-500/15 text-rose-700 dark:text-rose-300'
  if (p === 1) return 'bg-amber-500/15 text-amber-700 dark:text-amber-300'
  if (p === 2) return 'bg-sky-500/15 text-sky-700 dark:text-sky-300'
  return 'bg-zinc-500/15 text-zinc-600 dark:text-zinc-400'
}

export function formatDate(unixSec: number | null | undefined): string {
  if (!unixSec) return '—'
  return new Date(unixSec * 1000).toLocaleString()
}

export function formatRelative(unixSec: number): string {
  const now = Date.now() / 1000
  const diff = now - unixSec
  if (diff < 60) return 'just now'
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  if (diff < 604800) return `${Math.floor(diff / 86400)}d ago`
  return new Date(unixSec * 1000).toLocaleDateString()
}

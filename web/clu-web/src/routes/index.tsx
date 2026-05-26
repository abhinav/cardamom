import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type Issue, type PatchIssueBody } from '../lib/api'
import IssueCard from '../components/IssueCard'

export const Route = createFileRoute('/')({
  component: BoardPage,
})

// Columns to render. "blocked" is derived (status=open AND blocked=true)
// and pulls from the open column at render time. cancelled is hidden by
// default — surface via the list view if needed.
const COLUMNS: Array<{ key: string; label: string; status?: string }> = [
  { key: 'open', label: 'Open', status: 'open' },
  { key: 'in_progress', label: 'In progress', status: 'in_progress' },
  { key: 'blocked', label: 'Blocked' },
  { key: 'closed', label: 'Closed', status: 'closed' },
]

function BoardPage() {
  const qc = useQueryClient()
  const { data: issues = [], isLoading, error } = useQuery<Issue[]>({
    queryKey: ['issues', { board: true }],
    queryFn: () =>
      api.get(
        // Pull every non-cancelled status; we partition client-side
        // (blocked is derived from blocked=true on open issues).
        '/api/issues?status=open&status=in_progress&status=closed&limit=500',
      ),
  })

  // Status mutation is the drag-drop write path: PATCH status on the
  // target issue. Optimistic update so the card moves before the
  // network round-trip; rollback on failure.
  const move = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) => {
      const body: PatchIssueBody = { status }
      return api.patch<Issue>(`/api/issues/${id}`, body)
    },
    onMutate: async ({ id, status }) => {
      await qc.cancelQueries({ queryKey: ['issues'] })
      const prev = qc.getQueryData<Issue[]>(['issues', { board: true }])
      if (prev) {
        qc.setQueryData<Issue[]>(
          ['issues', { board: true }],
          prev.map((i) => (i.id === id ? { ...i, status: status as Issue['status'] } : i)),
        )
      }
      return { prev }
    },
    onError: (_e, _v, ctx) => {
      if (ctx?.prev) {
        qc.setQueryData(['issues', { board: true }], ctx.prev)
      }
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ['issues'] }),
  })

  const byColumn = partitionByColumn(issues)

  if (error) {
    return (
      <div className="rounded-lg border border-rose-500/30 bg-rose-500/10 p-4 text-sm">
        Failed to load issues: {(error as Error).message}
      </div>
    )
  }

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Board</h1>
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-4">
        {COLUMNS.map((col) => (
          <Column
            key={col.key}
            label={col.label}
            issues={byColumn[col.key] ?? []}
            loading={isLoading}
            droppable={col.status !== undefined}
            onDrop={(issueId) => {
              if (!col.status) return
              move.mutate({ id: issueId, status: col.status })
            }}
          />
        ))}
      </div>
    </div>
  )
}

function partitionByColumn(issues: Issue[]): Record<string, Issue[]> {
  const out: Record<string, Issue[]> = {
    open: [],
    in_progress: [],
    blocked: [],
    closed: [],
  }
  for (const i of issues) {
    if (i.status === 'open' && i.blocked) out.blocked.push(i)
    else if (i.status === 'open') out.open.push(i)
    else if (i.status === 'in_progress') out.in_progress.push(i)
    else if (i.status === 'closed') out.closed.push(i)
  }
  return out
}

interface ColumnProps {
  label: string
  issues: Issue[]
  loading: boolean
  droppable: boolean
  onDrop: (issueId: string) => void
}

function Column({ label, issues, loading, droppable, onDrop }: ColumnProps) {
  const [isOver, setIsOver] = useState(false)
  return (
    <section
      onDragOver={
        droppable
          ? (e) => {
              e.preventDefault()
              e.dataTransfer.dropEffect = 'move'
              if (!isOver) setIsOver(true)
            }
          : undefined
      }
      onDragLeave={droppable ? () => setIsOver(false) : undefined}
      onDrop={
        droppable
          ? (e) => {
              e.preventDefault()
              setIsOver(false)
              const id = e.dataTransfer.getData('text/x-clu-issue-id')
              if (id) onDrop(id)
            }
          : undefined
      }
      className={`rounded-xl border bg-[var(--surface)] p-3 transition ${
        isOver
          ? 'border-[var(--lagoon-deep)] bg-[var(--surface-strong)]'
          : 'border-[var(--line)]'
      }`}
    >
      <header className="mb-2 flex items-baseline justify-between px-1">
        <h2 className="text-sm font-semibold text-[var(--sea-ink)]">{label}</h2>
        <span className="text-xs opacity-60">{issues.length}</span>
      </header>
      <div className="flex flex-col gap-2">
        {loading && issues.length === 0 && (
          <div className="rounded-lg border border-dashed border-[var(--line)] p-3 text-center text-xs opacity-50">
            loading…
          </div>
        )}
        {!loading && issues.length === 0 && (
          <div className="rounded-lg border border-dashed border-[var(--line)] p-3 text-center text-xs opacity-50">
            empty
          </div>
        )}
        {issues.map((issue) => (
          <IssueCard key={issue.id} issue={issue} draggable={droppable} />
        ))}
      </div>
    </section>
  )
}

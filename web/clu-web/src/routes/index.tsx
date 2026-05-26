import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Plus, RefreshCw } from 'lucide-react'
import { api, type Issue, type PatchIssueBody } from '../lib/api'
import IssueCard from '../components/IssueCard'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { ScrollArea } from '../components/ui/scroll-area'

export const Route = createFileRoute('/')({
  component: BoardPage,
})

// Column definitions for the kanban. Order matters — left-to-right is
// the natural flow of work. Cancelled is hidden by default; use the
// list view to find it.
const COLUMNS: Array<{ key: string; label: string; status?: string }> = [
  { key: 'open', label: 'Open', status: 'open' },
  { key: 'in_progress', label: 'In progress', status: 'in_progress' },
  { key: 'blocked', label: 'Blocked' },
  { key: 'closed', label: 'Closed', status: 'closed' },
]

function BoardPage() {
  const qc = useQueryClient()
  const { data: issues = [], isLoading, error, refetch, isFetching } = useQuery<
    Issue[]
  >({
    queryKey: ['issues', 'board'],
    queryFn: () =>
      api.get(
        '/api/issues?status=open&status=in_progress&status=closed&limit=500',
      ),
  })

  // Drag-drop write: PATCH status, optimistic + rollback. We invalidate
  // on settle so any server-side derived fields (blocked, updated)
  // refresh from authoritative state.
  const move = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) => {
      const body: PatchIssueBody = { status }
      return api.patch<Issue>(`/api/issues/${id}`, body)
    },
    onMutate: async ({ id, status }) => {
      await qc.cancelQueries({ queryKey: ['issues', 'board'] })
      const prev = qc.getQueryData<Issue[]>(['issues', 'board'])
      if (prev) {
        qc.setQueryData<Issue[]>(
          ['issues', 'board'],
          prev.map((i) =>
            i.id === id ? { ...i, status: status as Issue['status'] } : i,
          ),
        )
      }
      return { prev }
    },
    onError: (_e, _v, ctx) => {
      if (ctx?.prev) qc.setQueryData(['issues', 'board'], ctx.prev)
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ['issues'] }),
  })

  const byColumn = partitionByColumn(issues)

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title="Board"
        subtitle={`${issues.length} issue${issues.length === 1 ? '' : 's'}`}
        actions={
          <>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => refetch()}
              disabled={isFetching}
              aria-label="refresh"
            >
              <RefreshCw
                className={isFetching ? 'animate-spin' : ''}
              />
            </Button>
            <Button size="sm" disabled title="Coming soon">
              <Plus />
              New
            </Button>
          </>
        }
      />

      {error && (
        <div className="border-destructive/40 bg-destructive/10 text-destructive m-6 rounded-md border p-3 text-sm">
          {(error as Error).message}
        </div>
      )}

      <div className="grid min-h-0 flex-1 grid-cols-4 gap-3 px-4 pb-4">
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
      data-droppable={droppable || undefined}
      data-over={isOver || undefined}
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
      className="bg-muted/30 data-[over=true]:bg-accent data-[over=true]:ring-ring/30 flex min-h-0 flex-col rounded-lg border transition-colors data-[over=true]:ring-2"
    >
      <header className="flex items-center justify-between gap-2 border-b px-3 py-2">
        <div className="flex items-center gap-2">
          <h2 className="text-[11px] font-semibold uppercase tracking-wider">
            {label}
          </h2>
          <Badge variant="muted" className="tabular font-mono">
            {issues.length}
          </Badge>
        </div>
        {droppable && (
          <Button
            variant="ghost"
            size="icon-xs"
            className="opacity-50 hover:opacity-100"
            disabled
            title="Coming soon"
          >
            <Plus />
          </Button>
        )}
      </header>
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex flex-col gap-2 p-2">
          {loading && issues.length === 0 && (
            <div className="text-muted-foreground rounded-md border border-dashed py-6 text-center text-xs">
              loading…
            </div>
          )}
          {!loading && issues.length === 0 && (
            <div className="text-muted-foreground rounded-md border border-dashed py-6 text-center text-xs">
              empty
            </div>
          )}
          {issues.map((issue) => (
            <IssueCard
              key={issue.id}
              issue={issue}
              draggable={droppable}
              hideStatus
            />
          ))}
        </div>
      </ScrollArea>
    </section>
  )
}

// PageHeader — re-usable bar that lives at the top of every route.
// Subtitle is the lightweight metadata (counts, last updated).
function PageHeader({
  title,
  subtitle,
  actions,
}: {
  title: string
  subtitle?: string
  actions?: React.ReactNode
}) {
  return (
    <header className="flex items-center justify-between gap-4 border-b px-6 py-3">
      <div>
        <h1 className="text-lg font-semibold leading-none tracking-tight">
          {title}
        </h1>
        {subtitle && (
          <p className="text-muted-foreground mt-1 text-xs">{subtitle}</p>
        )}
      </div>
      <div className="flex items-center gap-1.5">{actions}</div>
    </header>
  )
}

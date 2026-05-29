import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { Columns3, Plus } from 'lucide-react'
import { api, filtersToQuery } from '../lib/api'
import type { Issue, Meta, PatchIssueBody } from '../lib/api'
import { notifyError, notifyOk } from '../lib/toast-helpers'
import IssueCard from '../components/IssueCard'
import NewIssueDialog from '../components/NewIssueDialog'
import LiveIndicator from '../components/LiveIndicator'
import IssueFilterBar, {
  activeIssueFilterCount,
} from '../components/IssueFilterBar'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { ScrollArea } from '../components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../components/ui/select'

export const Route = createFileRoute('/')({
  component: BoardPage,
})

type BoardGroupBy = 'status' | 'agent' | 'priority'

const DEFAULT_BOARD_STATUSES: Issue['status'][] = [
  'open',
  'in_progress',
  'closed',
]

const GROUP_OPTIONS: Array<{ value: BoardGroupBy; label: string }> = [
  { value: 'status', label: 'Status' },
  { value: 'agent', label: 'Agent' },
  { value: 'priority', label: 'Priority' },
]

function BoardPage() {
  const qc = useQueryClient()
  const [newOpen, setNewOpen] = useState(false)
  const [groupBy, setGroupBy] = useState<BoardGroupBy>('status')
  const [statusFilter, setStatusFilter] = useState<string[]>([])
  const [typeFilter, setTypeFilter] = useState<string>('')
  const [agentFilter, setAgentFilter] = useState<string>('')
  const [tagFilter, setTagFilter] = useState<string[]>([])
  const [q, setQ] = useState<string>('')

  const { data: meta } = useQuery<Meta>({
    queryKey: ['meta'],
    queryFn: () => api.get('/api/meta'),
  })
  const { data: tags = [] } = useQuery<string[]>({
    queryKey: ['tags'],
    queryFn: () => api.get('/api/tags'),
  })

  const filterQuery = filtersToQuery({
    status: statusFilter.length ? statusFilter : DEFAULT_BOARD_STATUSES,
    type: typeFilter || undefined,
    agent: agentFilter || undefined,
    tag: tagFilter.length ? tagFilter : undefined,
    q: q || undefined,
    limit: 500,
  })
  const boardQueryKey = ['issues', 'board', { filterQuery }] as const

  const {
    data: issues = [],
    isLoading,
    error,
  } = useQuery<Issue[]>({
    queryKey: boardQueryKey,
    queryFn: () => api.get('/api/issues' + filterQuery),
  })

  const activeFilters = activeIssueFilterCount({
    q,
    statusFilter,
    typeFilter,
    agentFilter,
    tagFilter,
  })

  // Global "c" shortcut opens the New Issue dialog from anywhere on
  // the board, as long as the user isn't typing in a field already.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'c') return
      const t = e.target as HTMLElement | null
      if (
        t?.tagName === 'INPUT' ||
        t?.tagName === 'TEXTAREA' ||
        t?.isContentEditable
      ) {
        return
      }
      e.preventDefault()
      setNewOpen(true)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // Drag-drop write: PATCH the field represented by the active grouping,
  // with optimistic update + rollback. We invalidate on settle so
  // server-side derived fields (blocked, updated) refresh authoritatively.
  const move = useMutation({
    mutationFn: ({
      id,
      patch,
    }: {
      id: string
      patch: PatchIssueBody
      label: string
    }) => api.patch<Issue>(`/api/issues/${id}`, patch),
    onMutate: async ({ id, patch }) => {
      await qc.cancelQueries({ queryKey: ['issues', 'board'] })
      const prev = qc.getQueryData<Issue[]>(boardQueryKey)
      if (prev) {
        qc.setQueryData<Issue[]>(
          boardQueryKey,
          prev.map((i) => (i.id === id ? optimisticIssue(i, patch) : i)),
        )
      }
      return { prev }
    },
    onError: (err, _v, ctx) => {
      if (ctx?.prev) qc.setQueryData(boardQueryKey, ctx.prev)
      notifyError('Could not move card', err)
    },
    onSuccess: (i, v) => notifyOk(`Moved ${i.id} -> ${v.label}`),
    onSettled: () => qc.invalidateQueries({ queryKey: ['issues'] }),
  })

  const columns = useMemo(
    () => buildColumns(groupBy, issues, statusFilter),
    [groupBy, issues, statusFilter],
  )
  const canDragCards = columns.some((c) => c.droppable)

  function dropOnColumn(column: BoardColumn, issueId: string) {
    if (!column.patch) return
    const issue = issues.find((i) => i.id === issueId)
    if (!issue || isNoopMove(issue, column.patch)) return
    if (column.patch.assignee === null && issue.status === 'in_progress') {
      notifyError(
        'Could not move card',
        new Error(
          'In-progress issues need an assignee. Move it to Open first.',
        ),
      )
      return
    }
    move.mutate({ id: issueId, patch: column.patch, label: column.label })
  }

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title="Board"
        subtitle={`${issues.length} issue${issues.length === 1 ? '' : 's'}${
          activeFilters > 0 ? ` · ${activeFilters} filter applied` : ''
        }`}
        actions={
          <>
            <LiveIndicator />
            <div className="flex items-center gap-1.5">
              <span className="text-muted-foreground text-xs">Group</span>
              <Select
                value={groupBy}
                onValueChange={(v) => setGroupBy(v as BoardGroupBy)}
              >
                <SelectTrigger size="sm" className="w-32">
                  <Columns3 className="size-3.5" />
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {GROUP_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button size="sm" onClick={() => setNewOpen(true)}>
              <Plus />
              New issue
              <kbd className="bg-foreground/10 ml-1 hidden rounded px-1 text-[10px] font-mono sm:inline">
                c
              </kbd>
            </Button>
          </>
        }
      />

      <div className="border-b px-6 py-3">
        <IssueFilterBar
          q={q}
          setQ={setQ}
          statusFilter={statusFilter}
          setStatusFilter={setStatusFilter}
          typeFilter={typeFilter}
          setTypeFilter={setTypeFilter}
          agentFilter={agentFilter}
          setAgentFilter={setAgentFilter}
          tagFilter={tagFilter}
          setTagFilter={setTagFilter}
          tags={tags}
          statuses={meta?.statuses ?? []}
          types={meta?.types ?? []}
        />
      </div>

      {error && (
        <div className="border-destructive/40 bg-destructive/10 text-destructive m-6 rounded-md border p-3 text-sm">
          {error instanceof Error ? error.message : String(error)}
        </div>
      )}

      <div
        className="grid min-h-0 flex-1 gap-4 overflow-x-auto px-6 pb-6 pt-4"
        style={{
          gridTemplateColumns: `repeat(${columns.length}, minmax(16rem, 1fr))`,
        }}
      >
        {columns.map((col) => (
          <Column
            key={col.key}
            label={col.label}
            issues={col.issues}
            loading={isLoading}
            droppable={col.droppable}
            cardDraggable={canDragCards}
            hideStatus={col.hideStatus}
            hideAssignee={col.hideAssignee}
            hidePriority={col.hidePriority}
            onAdd={
              col.key === 'status:open' ? () => setNewOpen(true) : undefined
            }
            onDrop={(issueId) => dropOnColumn(col, issueId)}
          />
        ))}
      </div>

      <NewIssueDialog open={newOpen} onOpenChange={setNewOpen} />
    </div>
  )
}

interface BoardColumn {
  key: string
  label: string
  issues: Issue[]
  droppable: boolean
  patch?: PatchIssueBody
  hideStatus?: boolean
  hideAssignee?: boolean
  hidePriority?: boolean
}

function buildColumns(
  groupBy: BoardGroupBy,
  issues: Issue[],
  statusFilter: string[],
): BoardColumn[] {
  switch (groupBy) {
    case 'agent':
      return buildAgentColumns(issues)
    case 'priority':
      return buildPriorityColumns(issues)
    case 'status':
      return buildStatusColumns(issues, statusFilter)
  }
}

function buildStatusColumns(
  issues: Issue[],
  statusFilter: string[],
): BoardColumn[] {
  const visibleStatuses = new Set(
    statusFilter.length ? statusFilter : DEFAULT_BOARD_STATUSES,
  )
  const columns: BoardColumn[] = []

  if (visibleStatuses.has('open')) {
    columns.push(statusColumn('open', 'Open'))
    columns.push({
      key: 'status:blocked',
      label: 'Blocked',
      issues: [],
      droppable: false,
      hideStatus: true,
    })
  }
  if (visibleStatuses.has('in_progress')) {
    columns.push(statusColumn('in_progress', 'In progress'))
  }
  if (visibleStatuses.has('closed')) {
    columns.push(statusColumn('closed', 'Closed'))
  }
  if (visibleStatuses.has('cancelled')) {
    columns.push(statusColumn('cancelled', 'Cancelled'))
  }

  const byKey = new Map(columns.map((c) => [c.key, c]))
  for (const issue of issues) {
    if (issue.status === 'open' && issue.blocked) {
      byKey.get('status:blocked')?.issues.push(issue)
      continue
    }
    byKey.get(`status:${issue.status}`)?.issues.push(issue)
  }
  return columns
}

function statusColumn(status: Issue['status'], label: string): BoardColumn {
  return {
    key: `status:${status}`,
    label,
    issues: [],
    droppable: true,
    patch: { status },
    hideStatus: true,
  }
}

function buildAgentColumns(issues: Issue[]): BoardColumn[] {
  const assignees = Array.from(
    new Set(issues.flatMap((i) => (i.assignee ? [i.assignee] : []))),
  ).sort((a, b) => a.localeCompare(b))
  const columns: BoardColumn[] = [
    {
      key: 'agent:__none',
      label: 'Unassigned',
      issues: [],
      droppable: true,
      patch: { assignee: null },
      hideAssignee: true,
    },
    ...assignees.map(
      (assignee): BoardColumn => ({
        key: `agent:${assignee}`,
        label: assignee,
        issues: [],
        droppable: true,
        patch: { assignee },
        hideAssignee: true,
      }),
    ),
  ]

  const byKey = new Map(columns.map((c) => [c.key, c]))
  for (const issue of issues) {
    const key = issue.assignee ? `agent:${issue.assignee}` : 'agent:__none'
    byKey.get(key)?.issues.push(issue)
  }
  return columns
}

function buildPriorityColumns(issues: Issue[]): BoardColumn[] {
  const labels = ['P0 urgent', 'P1 high', 'P2 normal', 'P3 low', 'P4 lowest']
  const columns: BoardColumn[] = labels.map((label, priority) => ({
    key: `priority:${priority}`,
    label,
    issues: [],
    droppable: true,
    patch: { priority },
    hidePriority: true,
  }))
  const byKey = new Map(columns.map((c) => [c.key, c]))
  for (const issue of issues) {
    byKey.get(`priority:${issue.priority}`)?.issues.push(issue)
  }
  return columns
}

function optimisticIssue(issue: Issue, patch: PatchIssueBody): Issue {
  return {
    ...issue,
    status:
      patch.status === undefined
        ? issue.status
        : (patch.status as Issue['status']),
    priority: patch.priority ?? issue.priority,
    assignee: patch.assignee === undefined ? issue.assignee : patch.assignee,
  }
}

function isNoopMove(issue: Issue, patch: PatchIssueBody): boolean {
  if (patch.status !== undefined && patch.status !== issue.status) return false
  if (patch.priority !== undefined && patch.priority !== issue.priority) {
    return false
  }
  if (
    patch.assignee !== undefined &&
    patch.assignee !== (issue.assignee ?? null)
  ) {
    return false
  }
  return true
}

interface ColumnProps {
  label: string
  issues: Issue[]
  loading: boolean
  droppable: boolean
  cardDraggable: boolean
  onDrop: (issueId: string) => void
  onAdd?: () => void
  hideStatus?: boolean
  hideAssignee?: boolean
  hidePriority?: boolean
}

function Column({
  label,
  issues,
  loading,
  droppable,
  cardDraggable,
  onDrop,
  onAdd,
  hideStatus,
  hideAssignee,
  hidePriority,
}: ColumnProps) {
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
        {onAdd && (
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onAdd}
            aria-label="add issue"
            title="add issue"
          >
            <Plus />
          </Button>
        )}
      </header>
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex flex-col gap-2 p-2">
          {loading && issues.length === 0 && (
            <div className="text-muted-foreground rounded-md border border-dashed py-6 text-center text-xs">
              loading...
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
              draggable={cardDraggable}
              hideStatus={hideStatus}
              hideAssignee={hideAssignee}
              hidePriority={hidePriority}
            />
          ))}
        </div>
      </ScrollArea>
    </section>
  )
}

// PageHeader - reusable bar that lives at the top of every route.
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
    <header className="flex items-center justify-between gap-4 border-b px-6 py-4">
      <div>
        <h1 className="text-lg font-semibold leading-none tracking-tight">
          {title}
        </h1>
        {subtitle && (
          <p className="text-muted-foreground mt-1.5 text-xs">{subtitle}</p>
        )}
      </div>
      <div className="flex items-center gap-1.5">{actions}</div>
    </header>
  )
}

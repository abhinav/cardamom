import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { ChevronDown, Plus, Search, Tag as TagIcon, X } from 'lucide-react'
import { api, filtersToQuery, type Issue, type Meta } from '../lib/api'
import { formatRelative, type Status } from '../lib/issue-display'
import { PriorityBadge, StatusBadge } from '../components/StatusBadge'
import NewIssueDialog from '../components/NewIssueDialog'
import LiveIndicator from '../components/LiveIndicator'
import { notifyError, notifyOk } from '../lib/toast-helpers'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Badge } from '../components/ui/badge'
import { Checkbox } from '../components/ui/checkbox'
import { ScrollArea } from '../components/ui/scroll-area'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../components/ui/table'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '../components/ui/dropdown-menu'

export const Route = createFileRoute('/list')({
  component: ListPage,
})

// Workflow-internal labels (run:*, step:*, etc.) are not shown as
// user-facing tags. Shared with detail / cards.
function isManagedLabel(l: string): boolean {
  return /^(run|step|checkpoint|cap|template):/.test(l)
}

function ListPage() {
  const qc = useQueryClient()
  const [statusFilter, setStatusFilter] = useState<string[]>([])
  const [typeFilter, setTypeFilter] = useState<string>('')
  const [agentFilter, setAgentFilter] = useState<string>('')
  const [tagFilter, setTagFilter] = useState<string[]>([])
  const [q, setQ] = useState<string>('')
  const [newOpen, setNewOpen] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())

  const { data: meta } = useQuery<Meta>({
    queryKey: ['meta'],
    queryFn: () => api.get('/api/meta'),
  })
  const { data: tags = [] } = useQuery<string[]>({
    queryKey: ['tags'],
    queryFn: () => api.get('/api/tags'),
  })

  const filterQuery = filtersToQuery({
    status: statusFilter.length ? statusFilter : undefined,
    type: typeFilter || undefined,
    agent: agentFilter || undefined,
    tag: tagFilter.length ? tagFilter : undefined,
    q: q || undefined,
    limit: 500,
  })

  const { data: issues = [], isLoading, error } = useQuery<Issue[]>({
    queryKey: ['issues', 'list', { filterQuery }],
    queryFn: () => api.get('/api/issues' + filterQuery),
  })

  // Drop selections that no longer match the current filter so the
  // bulk bar's count stays accurate.
  const visibleIds = useMemo(() => new Set(issues.map((i) => i.id)), [issues])
  const validSelected = useMemo(
    () => new Set([...selected].filter((id) => visibleIds.has(id))),
    [selected, visibleIds],
  )

  const activeFilters =
    statusFilter.length +
    (typeFilter ? 1 : 0) +
    (agentFilter ? 1 : 0) +
    tagFilter.length +
    (q ? 1 : 0)

  function toggleSelect(id: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }
  function toggleSelectAll(check: boolean) {
    setSelected(check ? new Set(issues.map((i) => i.id)) : new Set())
  }
  function clearSelection() {
    setSelected(new Set())
  }
  function toggleTagFilter(t: string) {
    setTagFilter((prev) =>
      prev.includes(t) ? prev.filter((x) => x !== t) : [...prev, t],
    )
  }

  // headerCheckState is the tri-state for the select-all checkbox:
  // none / some / all of the visible rows.
  const headerCheckState: boolean | 'indeterminate' =
    validSelected.size === 0
      ? false
      : validSelected.size === issues.length
        ? true
        : 'indeterminate'

  return (
    <div className="relative flex h-full flex-col">
      <header className="flex items-center justify-between gap-4 border-b px-6 py-4">
        <div>
          <h1 className="text-lg font-semibold leading-none tracking-tight">
            Issues
          </h1>
          <p className="text-muted-foreground mt-1.5 text-xs">
            {issues.length} result{issues.length === 1 ? '' : 's'}
            {activeFilters > 0 ? ` · ${activeFilters} filter applied` : ''}
          </p>
        </div>
        <div className="flex items-center gap-1.5">
          <LiveIndicator />
          <Button size="sm" onClick={() => setNewOpen(true)}>
            <Plus />
            New issue
          </Button>
        </div>
      </header>

      <div className="border-b px-6 py-3">
        <FilterBar
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
          {(error as Error).message}
        </div>
      )}

      <ScrollArea className="min-h-0 flex-1">
        <Table className="tabular">
          <TableHeader className="bg-background sticky top-0 z-10 border-b">
            <TableRow>
              <TableHead className="w-10">
                <Checkbox
                  checked={headerCheckState}
                  onCheckedChange={(c) => toggleSelectAll(c === true)}
                  aria-label="select all rows"
                  disabled={issues.length === 0}
                />
              </TableHead>
              <TableHead className="w-24">ID</TableHead>
              <TableHead>Title</TableHead>
              <TableHead className="w-32">Status</TableHead>
              <TableHead className="w-16">Pri</TableHead>
              <TableHead className="w-24">Type</TableHead>
              <TableHead className="w-32">Assignee</TableHead>
              <TableHead>Tags</TableHead>
              <TableHead className="w-28">Updated</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow>
                <TableCell
                  colSpan={9}
                  className="text-muted-foreground text-center"
                >
                  loading…
                </TableCell>
              </TableRow>
            )}
            {!isLoading && issues.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={9}
                  className="text-muted-foreground text-center"
                >
                  no matching issues
                </TableCell>
              </TableRow>
            )}
            {issues.map((i) => {
              const isSel = validSelected.has(i.id)
              const userTags = i.labels.filter((l) => !isManagedLabel(l))
              return (
                <TableRow
                  key={i.id}
                  data-state={isSel ? 'selected' : undefined}
                  className="cursor-pointer"
                >
                  <TableCell className="align-middle">
                    <Checkbox
                      checked={isSel}
                      onCheckedChange={() => toggleSelect(i.id)}
                      aria-label={`select ${i.id}`}
                      onClick={(e) => e.stopPropagation()}
                    />
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    <Link
                      to="/issues/$id"
                      params={{ id: i.id }}
                      className="text-muted-foreground hover:text-foreground no-underline"
                    >
                      {i.id}
                    </Link>
                  </TableCell>
                  <TableCell className="max-w-md truncate">
                    <Link
                      to="/issues/$id"
                      params={{ id: i.id }}
                      className="text-foreground no-underline"
                    >
                      {i.title}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      status={i.status as Status}
                      blocked={i.blocked}
                    />
                  </TableCell>
                  <TableCell>
                    <PriorityBadge priority={i.priority} />
                  </TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {i.type}
                  </TableCell>
                  <TableCell className="text-xs">
                    {i.assignee ?? (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex max-w-xs flex-wrap gap-1">
                      {userTags.length === 0 ? (
                        <span className="text-muted-foreground text-xs">—</span>
                      ) : (
                        userTags.slice(0, 4).map((t) => (
                          <Badge
                            key={t}
                            variant={
                              tagFilter.includes(t) ? 'secondary' : 'muted'
                            }
                            asChild
                          >
                            <button
                              type="button"
                              onClick={(e) => {
                                e.preventDefault()
                                e.stopPropagation()
                                toggleTagFilter(t)
                              }}
                              title={
                                tagFilter.includes(t)
                                  ? `remove ${t} filter`
                                  : `filter by ${t}`
                              }
                            >
                              {t}
                            </button>
                          </Badge>
                        ))
                      )}
                      {userTags.length > 4 && (
                        <span className="text-muted-foreground text-xs">
                          +{userTags.length - 4}
                        </span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {formatRelative(i.updated)}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </ScrollArea>

      <NewIssueDialog open={newOpen} onOpenChange={setNewOpen} />

      <BulkActionBar
        selected={validSelected}
        issues={issues}
        onCleared={clearSelection}
        onDone={() => qc.invalidateQueries({ queryKey: ['issues'] })}
      />
    </div>
  )
}

interface FilterBarProps {
  q: string
  setQ: (v: string) => void
  statusFilter: string[]
  setStatusFilter: (v: string[]) => void
  typeFilter: string
  setTypeFilter: (v: string) => void
  agentFilter: string
  setAgentFilter: (v: string) => void
  tagFilter: string[]
  setTagFilter: (v: string[]) => void
  tags: string[]
  statuses: string[]
  types: string[]
}

function FilterBar({
  q,
  setQ,
  statusFilter,
  setStatusFilter,
  typeFilter,
  setTypeFilter,
  agentFilter,
  setAgentFilter,
  tagFilter,
  setTagFilter,
  tags,
  statuses,
  types,
}: FilterBarProps) {
  function toggleStatus(s: string) {
    setStatusFilter(
      statusFilter.includes(s)
        ? statusFilter.filter((x) => x !== s)
        : [...statusFilter, s],
    )
  }
  function toggleTag(t: string) {
    setTagFilter(
      tagFilter.includes(t)
        ? tagFilter.filter((x) => x !== t)
        : [...tagFilter, t],
    )
  }

  function clearAll() {
    setQ('')
    setStatusFilter([])
    setTypeFilter('')
    setAgentFilter('')
    setTagFilter([])
  }

  const anyActive =
    !!q ||
    statusFilter.length > 0 ||
    !!typeFilter ||
    !!agentFilter ||
    tagFilter.length > 0

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative">
        <Search className="text-muted-foreground absolute left-2 top-1/2 size-3.5 -translate-y-1/2" />
        <Input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="search title…"
          className="h-8 w-64 pl-7"
        />
      </div>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm">
            Status
            {statusFilter.length > 0 && (
              <Badge variant="secondary" className="ml-1 font-mono">
                {statusFilter.length}
              </Badge>
            )}
            <ChevronDown className="opacity-50" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-44">
          <DropdownMenuLabel>Filter by status</DropdownMenuLabel>
          <DropdownMenuSeparator />
          {statuses.map((s) => (
            <DropdownMenuCheckboxItem
              key={s}
              checked={statusFilter.includes(s)}
              onCheckedChange={() => toggleStatus(s)}
              onSelect={(e) => e.preventDefault()}
            >
              {s}
            </DropdownMenuCheckboxItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm">
            Type{typeFilter ? `: ${typeFilter}` : ''}
            <ChevronDown className="opacity-50" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-40">
          <DropdownMenuLabel>Filter by type</DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuRadioGroup
            value={typeFilter}
            onValueChange={(v) => setTypeFilter(v === '__all' ? '' : v)}
          >
            <DropdownMenuRadioItem value="__all">all types</DropdownMenuRadioItem>
            {types.map((t) => (
              <DropdownMenuRadioItem key={t} value={t}>
                {t}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm" disabled={tags.length === 0}>
            <TagIcon className="size-3.5" />
            Tags
            {tagFilter.length > 0 && (
              <Badge variant="secondary" className="ml-1 font-mono">
                {tagFilter.length}
              </Badge>
            )}
            <ChevronDown className="opacity-50" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          className="max-h-72 w-52 overflow-y-auto"
        >
          <DropdownMenuLabel>Filter by tag (any)</DropdownMenuLabel>
          <DropdownMenuSeparator />
          {tags.length === 0 && (
            <div className="text-muted-foreground px-2 py-1.5 text-xs">
              no tags in use yet
            </div>
          )}
          {tags.map((t) => (
            <DropdownMenuCheckboxItem
              key={t}
              checked={tagFilter.includes(t)}
              onCheckedChange={() => toggleTag(t)}
              onSelect={(e) => e.preventDefault()}
            >
              {t}
            </DropdownMenuCheckboxItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>

      <Input
        value={agentFilter}
        onChange={(e) => setAgentFilter(e.target.value)}
        placeholder="agent…"
        className="h-8 w-32"
      />

      {anyActive && (
        <Button
          variant="ghost"
          size="sm"
          onClick={clearAll}
          className="text-muted-foreground"
        >
          <X />
          clear
        </Button>
      )}
    </div>
  )
}

// BulkActionBar — floating bottom bar that appears whenever the user
// has selected one or more rows in the list. Actions hit the existing
// per-id endpoints in parallel (Promise.allSettled so a single 4xx
// doesn't abort the rest); summary toast reports the count.
function BulkActionBar({
  selected,
  issues,
  onCleared,
  onDone,
}: {
  selected: Set<string>
  issues: Issue[]
  onCleared: () => void
  onDone: () => void
}) {
  const [busy, setBusy] = useState(false)
  if (selected.size === 0) return null
  const ids = Array.from(selected)
  const selectedIssues = issues.filter((i) => selected.has(i.id))
  const anyOpen = selectedIssues.some(
    (i) => i.status !== 'closed' && i.status !== 'cancelled',
  )
  const anyClosed = selectedIssues.some(
    (i) => i.status === 'closed' || i.status === 'cancelled',
  )

  async function applyEach(
    label: string,
    each: (id: string) => Promise<unknown>,
  ) {
    setBusy(true)
    try {
      const results = await Promise.allSettled(ids.map((id) => each(id)))
      const failed = results.filter((r) => r.status === 'rejected')
      if (failed.length === 0) {
        notifyOk(`${label} (${ids.length})`)
        onCleared()
      } else if (failed.length === ids.length) {
        notifyError(`${label} failed for all ${ids.length}`, (failed[0] as PromiseRejectedResult).reason)
      } else {
        notifyOk(
          `${label} — ${ids.length - failed.length}/${ids.length}`,
          `${failed.length} failed. Check the console for details.`,
        )
        failed.forEach((f) =>
          // eslint-disable-next-line no-console
          console.error('bulk failure:', (f as PromiseRejectedResult).reason),
        )
      }
    } finally {
      setBusy(false)
      onDone()
    }
  }

  return (
    <div
      // Center on the main area (not the viewport — sidebar steals
      // ~240px on the left). pointer-events-none on the outer wrapper
      // so the rest of the page is still clickable behind the bar.
      className="pointer-events-none absolute inset-x-0 bottom-4 z-30 flex justify-center"
    >
      <div className="bg-popover text-popover-foreground pointer-events-auto flex items-center gap-1 rounded-full border px-2 py-1.5 shadow-lg">
        <span className="px-3 text-xs font-medium">
          {selected.size} selected
        </span>
        <span className="bg-border h-4 w-px" />
        {anyOpen && (
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() =>
              applyEach('Closed', (id) => api.post(`/api/issues/${id}/close`))
            }
          >
            Close
          </Button>
        )}
        {anyClosed && (
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() =>
              applyEach('Reopened', (id) =>
                api.post(`/api/issues/${id}/reopen`),
              )
            }
          >
            Reopen
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          onClick={() => {
            const t = window.prompt('Add tag to selected:')
            if (!t || !t.trim()) return
            applyEach('Tagged', (id) =>
              api.post(`/api/issues/${id}/labels`, { labels: [t.trim()] }),
            )
          }}
        >
          <TagIcon className="size-3.5" />
          Tag…
        </Button>
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          onClick={() => {
            const p = window.prompt('Set priority (0 highest, 4 lowest):', '2')
            if (p == null) return
            const n = Number(p)
            if (!Number.isInteger(n) || n < 0 || n > 4) return
            applyEach('Priority set', (id) =>
              api.patch(`/api/issues/${id}`, { priority: n }),
            )
          }}
        >
          Priority…
        </Button>
        <span className="bg-border h-4 w-px" />
        <Button
          variant="ghost"
          size="sm"
          onClick={onCleared}
          aria-label="clear selection"
        >
          <X />
        </Button>
      </div>
    </div>
  )
}

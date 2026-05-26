import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { ChevronDown, RefreshCw, Search, X } from 'lucide-react'
import { api, filtersToQuery, type Issue, type Meta } from '../lib/api'
import { formatRelative, type Status } from '../lib/issue-display'
import { PriorityBadge, StatusBadge } from '../components/StatusBadge'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Badge } from '../components/ui/badge'
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

function ListPage() {
  const [statusFilter, setStatusFilter] = useState<string[]>([])
  const [typeFilter, setTypeFilter] = useState<string>('')
  const [agentFilter, setAgentFilter] = useState<string>('')
  const [q, setQ] = useState<string>('')

  const { data: meta } = useQuery<Meta>({
    queryKey: ['meta'],
    queryFn: () => api.get('/api/meta'),
  })

  const filterQuery = filtersToQuery({
    status: statusFilter.length ? statusFilter : undefined,
    type: typeFilter || undefined,
    agent: agentFilter || undefined,
    q: q || undefined,
    limit: 500,
  })

  const { data: issues = [], isLoading, error, refetch, isFetching } = useQuery<
    Issue[]
  >({
    queryKey: ['issues', 'list', { filterQuery }],
    queryFn: () => api.get('/api/issues' + filterQuery),
  })

  const activeFilters =
    statusFilter.length + (typeFilter ? 1 : 0) + (agentFilter ? 1 : 0) + (q ? 1 : 0)

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between gap-4 border-b px-6 py-3">
        <div>
          <h1 className="text-lg font-semibold leading-none tracking-tight">
            Issues
          </h1>
          <p className="text-muted-foreground mt-1 text-xs">
            {issues.length} result{issues.length === 1 ? '' : 's'}
            {activeFilters > 0 ? ` · ${activeFilters} filter applied` : ''}
          </p>
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => refetch()}
          disabled={isFetching}
          aria-label="refresh"
        >
          <RefreshCw className={isFetching ? 'animate-spin' : ''} />
        </Button>
      </header>

      <div className="border-b px-6 py-2">
        <FilterBar
          q={q}
          setQ={setQ}
          statusFilter={statusFilter}
          setStatusFilter={setStatusFilter}
          typeFilter={typeFilter}
          setTypeFilter={setTypeFilter}
          agentFilter={agentFilter}
          setAgentFilter={setAgentFilter}
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
              <TableHead className="w-24">ID</TableHead>
              <TableHead>Title</TableHead>
              <TableHead className="w-32">Status</TableHead>
              <TableHead className="w-16">Pri</TableHead>
              <TableHead className="w-24">Type</TableHead>
              <TableHead className="w-32">Assignee</TableHead>
              <TableHead className="w-28">Updated</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className="text-muted-foreground text-center"
                >
                  loading…
                </TableCell>
              </TableRow>
            )}
            {!isLoading && issues.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className="text-muted-foreground text-center"
                >
                  no matching issues
                </TableCell>
              </TableRow>
            )}
            {issues.map((i) => (
              <TableRow key={i.id} className="cursor-pointer">
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
                  {i.assignee ?? <span className="text-muted-foreground">—</span>}
                </TableCell>
                <TableCell className="text-muted-foreground text-xs">
                  {formatRelative(i.updated)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </ScrollArea>
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

  function clearAll() {
    setQ('')
    setStatusFilter([])
    setTypeFilter('')
    setAgentFilter('')
  }

  const anyActive =
    !!q || statusFilter.length > 0 || !!typeFilter || !!agentFilter

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

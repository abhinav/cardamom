import { ChevronDown, Search, Tag as TagIcon, X } from 'lucide-react'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Badge } from './ui/badge'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './ui/dropdown-menu'

export interface IssueFilterValues {
  q: string
  statusFilter: string[]
  typeFilter: string
  agentFilter: string
  tagFilter: string[]
}

interface IssueFilterBarProps extends IssueFilterValues {
  setQ: (v: string) => void
  setStatusFilter: (v: string[]) => void
  setTypeFilter: (v: string) => void
  setAgentFilter: (v: string) => void
  setTagFilter: (v: string[]) => void
  tags: string[]
  statuses: string[]
  types: string[]
}

export function activeIssueFilterCount({
  q,
  statusFilter,
  typeFilter,
  agentFilter,
  tagFilter,
}: IssueFilterValues): number {
  return (
    statusFilter.length +
    (typeFilter ? 1 : 0) +
    (agentFilter ? 1 : 0) +
    tagFilter.length +
    (q ? 1 : 0)
  )
}

export default function IssueFilterBar({
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
}: IssueFilterBarProps) {
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
          placeholder="search title..."
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
            <DropdownMenuRadioItem value="__all">
              all types
            </DropdownMenuRadioItem>
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
        placeholder="agent..."
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

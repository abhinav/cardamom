import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api, filtersToQuery, type Issue, type Meta } from '../lib/api'
import {
  displayStatusClass,
  displayStatusLabel,
  formatRelative,
  priorityClass,
  type Status,
} from '../lib/issue-display'

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
  const { data: tags = [] } = useQuery<string[]>({
    queryKey: ['tags'],
    queryFn: () => api.get('/api/tags'),
  })

  const filterQuery = filtersToQuery({
    status: statusFilter.length ? statusFilter : undefined,
    type: typeFilter || undefined,
    agent: agentFilter || undefined,
    q: q || undefined,
    limit: 500,
  })

  const { data: issues = [], isLoading, error } = useQuery<Issue[]>({
    queryKey: ['issues', { filterQuery }],
    queryFn: () => api.get('/api/issues' + filterQuery),
  })

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Issues</h1>

      <div className="mb-4 flex flex-wrap items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="search title…"
          className="min-w-48 flex-1 rounded border border-[var(--line)] bg-transparent px-2 py-1 text-sm"
        />
        <MultiToggle
          options={meta?.statuses ?? []}
          values={statusFilter}
          onChange={setStatusFilter}
        />
        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          className="rounded border border-[var(--line)] bg-transparent px-2 py-1 text-sm"
        >
          <option value="">all types</option>
          {meta?.types.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
        <input
          value={agentFilter}
          onChange={(e) => setAgentFilter(e.target.value)}
          placeholder="agent…"
          className="w-32 rounded border border-[var(--line)] bg-transparent px-2 py-1 text-sm"
        />
        {tags.length > 0 && (
          <span className="text-xs opacity-50">
            {tags.length} tag{tags.length === 1 ? '' : 's'} in use
          </span>
        )}
      </div>

      {error && (
        <div className="rounded-lg border border-rose-500/30 bg-rose-500/10 p-4 text-sm">
          {(error as Error).message}
        </div>
      )}

      <div className="overflow-x-auto rounded-lg border border-[var(--line)] bg-[var(--surface-strong)]">
        <table className="min-w-full text-sm">
          <thead className="border-b border-[var(--line)] text-xs uppercase tracking-wide opacity-60">
            <tr>
              <th className="px-3 py-2 text-left">ID</th>
              <th className="px-3 py-2 text-left">Title</th>
              <th className="px-3 py-2 text-left">Status</th>
              <th className="px-3 py-2 text-left">Pri</th>
              <th className="px-3 py-2 text-left">Type</th>
              <th className="px-3 py-2 text-left">Assignee</th>
              <th className="px-3 py-2 text-left">Updated</th>
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr>
                <td colSpan={7} className="px-3 py-4 text-center opacity-50">
                  loading…
                </td>
              </tr>
            )}
            {!isLoading && issues.length === 0 && (
              <tr>
                <td colSpan={7} className="px-3 py-4 text-center opacity-50">
                  no matching issues
                </td>
              </tr>
            )}
            {issues.map((i) => (
              <tr
                key={i.id}
                className="border-t border-[var(--line)] hover:bg-[var(--link-bg-hover)]"
              >
                <td className="px-3 py-2 font-mono text-xs">
                  <Link
                    to="/issues/$id"
                    params={{ id: i.id }}
                    className="no-underline"
                  >
                    {i.id}
                  </Link>
                </td>
                <td className="px-3 py-2">
                  <Link
                    to="/issues/$id"
                    params={{ id: i.id }}
                    className="no-underline"
                  >
                    {i.title}
                  </Link>
                </td>
                <td className="px-3 py-2">
                  <span
                    className={`rounded px-1.5 py-0.5 text-xs font-medium ${displayStatusClass(i.status as Status, i.blocked)}`}
                  >
                    {displayStatusLabel(i.status as Status, i.blocked)}
                  </span>
                </td>
                <td className="px-3 py-2">
                  <span
                    className={`rounded px-1.5 py-0.5 text-xs font-mono font-semibold ${priorityClass(i.priority)}`}
                  >
                    p{i.priority}
                  </span>
                </td>
                <td className="px-3 py-2 text-xs opacity-70">{i.type}</td>
                <td className="px-3 py-2 text-xs">{i.assignee ?? '—'}</td>
                <td className="px-3 py-2 text-xs opacity-60">
                  {formatRelative(i.updated)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// MultiToggle is a row of pill toggles (status filter). Inline because
// only used here — promote to ui/ if a second caller appears.
function MultiToggle({
  options,
  values,
  onChange,
}: {
  options: string[]
  values: string[]
  onChange: (next: string[]) => void
}) {
  return (
    <div className="flex gap-1">
      {options.map((opt) => {
        const active = values.includes(opt)
        return (
          <button
            key={opt}
            type="button"
            onClick={() =>
              onChange(
                active ? values.filter((v) => v !== opt) : [...values, opt],
              )
            }
            className={`rounded-full border px-2 py-0.5 text-xs font-medium transition ${
              active
                ? 'border-[var(--lagoon-deep)] bg-[var(--lagoon-deep)]/15 text-[var(--sea-ink)]'
                : 'border-[var(--line)] bg-transparent opacity-60 hover:opacity-100'
            }`}
          >
            {opt}
          </button>
        )
      })}
    </div>
  )
}

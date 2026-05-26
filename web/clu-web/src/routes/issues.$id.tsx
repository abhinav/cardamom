import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import {
  api,
  type Comment,
  type Issue,
  type IssueDetail,
  type Meta,
  type PatchIssueBody,
} from '../lib/api'
import {
  displayStatusClass,
  displayStatusLabel,
  formatDate,
  formatRelative,
  priorityClass,
  type Status,
} from '../lib/issue-display'
import { useIdentity } from '../lib/use-identity'
import { Button } from '../components/ui/button'

export const Route = createFileRoute('/issues/$id')({
  component: IssueDetailPage,
})

function IssueDetailPage() {
  const { id } = Route.useParams()
  const qc = useQueryClient()
  const navigate = useNavigate()

  const { data: issue, isLoading, error } = useQuery<IssueDetail>({
    queryKey: ['issue', id],
    queryFn: () => api.get(`/api/issues/${id}`),
  })

  const { data: meta } = useQuery<Meta>({
    queryKey: ['meta'],
    queryFn: () => api.get('/api/meta'),
  })

  const patch = useMutation({
    mutationFn: (body: PatchIssueBody) =>
      api.patch<Issue>(`/api/issues/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['issue', id] }),
  })

  const claim = useMutation({
    mutationFn: () => api.post<Issue>(`/api/issues/${id}/claim`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['issue', id] }),
  })

  const closeOrReopen = useMutation({
    mutationFn: (action: 'close' | 'reopen') =>
      api.post<Issue>(`/api/issues/${id}/${action}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['issue', id] }),
  })

  if (error) {
    return (
      <div className="rounded-lg border border-rose-500/30 bg-rose-500/10 p-4 text-sm">
        Failed to load: {(error as Error).message}
        <div className="mt-2">
          <Link to="/" className="text-xs underline">
            ← back
          </Link>
        </div>
      </div>
    )
  }
  if (isLoading || !issue) {
    return <div className="opacity-50">loading…</div>
  }

  const isCheckpoint = issue.type === 'checkpoint'
  const isOpen = issue.status === 'open' || issue.status === 'in_progress'

  return (
    <article className="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_320px]">
      <div className="space-y-6">
        <header>
          <div className="flex items-baseline gap-3">
            <code className="text-sm opacity-60">{issue.id}</code>
            <span
              className={`rounded px-1.5 py-0.5 text-xs font-medium ${displayStatusClass(issue.status as Status, issue.blocked)}`}
            >
              {displayStatusLabel(issue.status as Status, issue.blocked)}
            </span>
            <span
              className={`rounded px-1.5 py-0.5 text-xs font-mono font-semibold ${priorityClass(issue.priority)}`}
            >
              p{issue.priority}
            </span>
          </div>
          <EditableTitle
            value={issue.title}
            onSave={(title) => patch.mutate({ title })}
          />
        </header>

        <EditableDescription
          value={issue.description ?? ''}
          onSave={(description) =>
            patch.mutate({ description: description || null })
          }
        />

        {issue.notes && (
          <section>
            <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide opacity-60">
              Notes
            </h3>
            <pre className="whitespace-pre-wrap rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3 font-sans text-sm">
              {issue.notes}
            </pre>
          </section>
        )}

        <CommentsSection
          issueId={issue.id}
          comments={issue.comments}
          onChange={() => qc.invalidateQueries({ queryKey: ['issue', id] })}
        />

        {isCheckpoint && isOpen && (
          <CheckpointActions
            id={issue.id}
            onDone={(passed) => {
              qc.invalidateQueries({ queryKey: ['issue', id] })
              if (passed) {
                // After a pass the gate is closed — board moves it.
              } else {
                // Fail cascades; bounce to the board.
                navigate({ to: '/' })
              }
            }}
          />
        )}
      </div>

      <aside className="space-y-4 text-sm">
        <Section title="Status">
          <select
            value={issue.status}
            onChange={(e) => patch.mutate({ status: e.target.value })}
            className="w-full rounded border border-[var(--line)] bg-transparent px-2 py-1 text-sm"
          >
            {meta?.statuses.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </Section>

        <Section title="Priority">
          <input
            type="number"
            min={0}
            value={issue.priority}
            onChange={(e) =>
              patch.mutate({ priority: Number(e.target.value) })
            }
            className="w-full rounded border border-[var(--line)] bg-transparent px-2 py-1 text-sm"
          />
        </Section>

        <Section title="Type">
          <select
            value={issue.type}
            onChange={(e) => patch.mutate({ type: e.target.value })}
            className="w-full rounded border border-[var(--line)] bg-transparent px-2 py-1 text-sm"
          >
            {meta?.types.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </Section>

        <Section title="Assignee">
          <NullableTextInput
            value={issue.assignee ?? ''}
            onSave={(v) => patch.mutate({ assignee: v || null })}
            placeholder="unassigned"
          />
        </Section>

        <Section title="Agent (lane)">
          <NullableTextInput
            value={issue.agent ?? ''}
            onSave={(v) => patch.mutate({ agent: v || null })}
            placeholder="—"
          />
        </Section>

        <Section title="Tags">
          <TagEditor
            tags={issue.labels.filter((l) => !isManaged(l))}
            onSave={(tags) => {
              // Send the full new set; server diffs and preserves managed labels.
              const managed = issue.labels.filter(isManaged)
              patch.mutate({ tags: [...tags, ...managed] })
            }}
          />
        </Section>

        {(issue.depends_on.length > 0 || issue.blocks.length > 0) && (
          <Section title="Deps">
            {issue.depends_on.length > 0 && (
              <div>
                <div className="text-xs opacity-60">depends on</div>
                <ul className="space-y-0.5 text-xs">
                  {issue.depends_on.map((d) => (
                    <li key={d}>
                      <Link to="/issues/$id" params={{ id: d }}>
                        {d}
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            )}
            {issue.blocks.length > 0 && (
              <div className="mt-2">
                <div className="text-xs opacity-60">blocks</div>
                <ul className="space-y-0.5 text-xs">
                  {issue.blocks.map((b) => (
                    <li key={b}>
                      <Link to="/issues/$id" params={{ id: b }}>
                        {b}
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </Section>
        )}

        <Section title="Actions">
          <div className="flex flex-col gap-1.5">
            {!issue.assignee && isOpen && (
              <Button size="sm" onClick={() => claim.mutate()}>
                Claim
              </Button>
            )}
            {issue.status !== 'closed' && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => closeOrReopen.mutate('close')}
              >
                Close
              </Button>
            )}
            {issue.status === 'closed' && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => closeOrReopen.mutate('reopen')}
              >
                Reopen
              </Button>
            )}
          </div>
        </Section>

        <Section title="Timestamps">
          <dl className="space-y-1 text-xs opacity-70">
            <div>
              <dt className="inline opacity-60">created </dt>
              <dd className="inline">{formatDate(issue.created)}</dd>
            </div>
            <div>
              <dt className="inline opacity-60">updated </dt>
              <dd className="inline">{formatDate(issue.updated)}</dd>
            </div>
            {issue.closed && (
              <div>
                <dt className="inline opacity-60">closed </dt>
                <dd className="inline">{formatDate(issue.closed)}</dd>
              </div>
            )}
          </dl>
        </Section>
      </aside>
    </article>
  )
}

// ---- inline editors ----

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section>
      <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide opacity-60">
        {title}
      </h3>
      {children}
    </section>
  )
}

function EditableTitle({
  value,
  onSave,
}: {
  value: string
  onSave: (v: string) => void
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)
  useEffect(() => setDraft(value), [value])

  if (!editing) {
    return (
      <h1
        className="mt-1 cursor-text text-2xl font-semibold leading-tight"
        onClick={() => setEditing(true)}
      >
        {value}
      </h1>
    )
  }
  return (
    <form
      className="mt-1"
      onSubmit={(e) => {
        e.preventDefault()
        if (draft.trim() && draft !== value) onSave(draft.trim())
        setEditing(false)
      }}
    >
      <input
        autoFocus
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => setEditing(false)}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            setDraft(value)
            setEditing(false)
          }
        }}
        className="w-full rounded border border-[var(--line)] bg-transparent px-2 py-1 text-2xl font-semibold"
      />
    </form>
  )
}

function EditableDescription({
  value,
  onSave,
}: {
  value: string
  onSave: (v: string) => void
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)
  useEffect(() => setDraft(value), [value])

  if (!editing) {
    return (
      <section
        className="cursor-text rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3 text-sm"
        onClick={() => setEditing(true)}
      >
        {value ? (
          <pre className="whitespace-pre-wrap font-sans">{value}</pre>
        ) : (
          <span className="opacity-40">click to add a description…</span>
        )}
      </section>
    )
  }
  return (
    <section>
      <textarea
        autoFocus
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        rows={Math.max(4, draft.split('\n').length + 1)}
        className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-strong)] p-3 font-sans text-sm"
      />
      <div className="mt-1 flex justify-end gap-1">
        <Button size="sm" variant="ghost" onClick={() => setEditing(false)}>
          Cancel
        </Button>
        <Button
          size="sm"
          onClick={() => {
            if (draft !== value) onSave(draft)
            setEditing(false)
          }}
        >
          Save
        </Button>
      </div>
    </section>
  )
}

function NullableTextInput({
  value,
  onSave,
  placeholder,
}: {
  value: string
  onSave: (v: string) => void
  placeholder?: string
}) {
  const [draft, setDraft] = useState(value)
  useEffect(() => setDraft(value), [value])
  return (
    <input
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={() => {
        if (draft !== value) onSave(draft.trim())
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter') (e.currentTarget as HTMLInputElement).blur()
        if (e.key === 'Escape') {
          setDraft(value)
          ;(e.currentTarget as HTMLInputElement).blur()
        }
      }}
      placeholder={placeholder}
      className="w-full rounded border border-[var(--line)] bg-transparent px-2 py-1 text-sm"
    />
  )
}

function TagEditor({
  tags,
  onSave,
}: {
  tags: string[]
  onSave: (tags: string[]) => void
}) {
  const [adding, setAdding] = useState(false)
  const [draft, setDraft] = useState('')

  function commit() {
    const v = draft.trim()
    if (v && !tags.includes(v)) onSave([...tags, v])
    setDraft('')
    setAdding(false)
  }

  return (
    <div className="flex flex-wrap items-center gap-1">
      {tags.map((t) => (
        <span
          key={t}
          className="inline-flex items-center gap-1 rounded bg-zinc-500/10 px-1.5 py-0.5 text-xs"
        >
          {t}
          <button
            className="opacity-50 hover:opacity-100"
            onClick={() => onSave(tags.filter((x) => x !== t))}
            aria-label={`remove ${t}`}
          >
            ×
          </button>
        </span>
      ))}
      {adding ? (
        <input
          autoFocus
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              commit()
            }
            if (e.key === 'Escape') {
              setDraft('')
              setAdding(false)
            }
          }}
          className="w-24 rounded border border-[var(--line)] bg-transparent px-1.5 py-0.5 text-xs"
        />
      ) : (
        <button
          className="rounded border border-dashed border-[var(--line)] px-1.5 py-0.5 text-xs opacity-60 hover:opacity-100"
          onClick={() => setAdding(true)}
        >
          + tag
        </button>
      )}
    </div>
  )
}

function CommentsSection({
  issueId,
  comments,
  onChange,
}: {
  issueId: string
  comments: Comment[]
  onChange: () => void
}) {
  const [agent] = useIdentity()
  const [draft, setDraft] = useState('')

  const add = useMutation({
    mutationFn: (body: string) =>
      api.post<Comment>(`/api/issues/${issueId}/comments`, { body }),
    onSuccess: () => {
      setDraft('')
      onChange()
    },
  })

  return (
    <section>
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide opacity-60">
        Comments ({comments.length})
      </h3>
      <ul className="space-y-2">
        {comments.map((c) => (
          <li
            key={c.id}
            className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3"
          >
            <div className="mb-1 flex items-baseline gap-2 text-xs opacity-60">
              <span className="font-semibold">{c.author}</span>
              <span>{formatRelative(c.created)}</span>
            </div>
            <pre className="whitespace-pre-wrap font-sans text-sm">
              {c.body}
            </pre>
          </li>
        ))}
      </ul>

      <form
        className="mt-3"
        onSubmit={(e) => {
          e.preventDefault()
          if (!draft.trim()) return
          add.mutate(draft.trim())
        }}
      >
        <textarea
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          rows={3}
          placeholder={
            agent
              ? `comment as ${agent}…`
              : 'set your identity in the header to comment'
          }
          disabled={!agent}
          className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-strong)] p-2 text-sm disabled:opacity-40"
        />
        <div className="mt-1 flex items-center justify-between">
          {add.error && (
            <span className="text-xs text-rose-500">
              {(add.error as Error).message}
            </span>
          )}
          <Button
            size="sm"
            type="submit"
            disabled={!draft.trim() || !agent || add.isPending}
            className="ml-auto"
          >
            Comment
          </Button>
        </div>
      </form>
    </section>
  )
}

function CheckpointActions({
  id,
  onDone,
}: {
  id: string
  onDone: (passed: boolean) => void
}) {
  const [agent] = useIdentity()
  const [reason, setReason] = useState('')

  const resolve = useMutation({
    mutationFn: ({ pass }: { pass: boolean }) =>
      api.post(`/api/checkpoints/${id}/${pass ? 'approve' : 'fail'}`, {
        reason: reason || undefined,
      }),
    onSuccess: (_data, { pass }) => onDone(pass),
  })

  return (
    <section className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-4">
      <h3 className="text-sm font-semibold">Checkpoint</h3>
      <p className="mt-1 text-xs opacity-70">
        Approve to close this gate and unblock downstream work. Fail to
        cancel-cascade.
      </p>
      <input
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        placeholder="optional reason…"
        className="mt-2 w-full rounded border border-[var(--line)] bg-transparent px-2 py-1 text-sm"
      />
      <div className="mt-2 flex gap-2">
        <Button
          size="sm"
          disabled={!agent || resolve.isPending}
          onClick={() => resolve.mutate({ pass: true })}
        >
          Approve
        </Button>
        <Button
          size="sm"
          variant="destructive"
          disabled={!agent || resolve.isPending}
          onClick={() => resolve.mutate({ pass: false })}
        >
          Fail
        </Button>
        {!agent && (
          <span className="self-center text-xs opacity-60">
            set identity first
          </span>
        )}
      </div>
      {resolve.error && (
        <p className="mt-2 text-xs text-rose-500">
          {(resolve.error as Error).message}
        </p>
      )}
    </section>
  )
}

function isManaged(label: string): boolean {
  return /^(run|step|checkpoint|cap|template):/.test(label)
}

import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import {
  ArrowLeft,
  CheckCircle2,
  MessageSquare,
  RotateCcw,
  ShieldCheck,
  UserPlus,
  X,
} from 'lucide-react'
import {
  api,
  type Comment,
  type Issue,
  type IssueDetail,
  type Meta,
  type PatchIssueBody,
} from '../lib/api'
import {
  formatDate,
  formatRelative,
  type Status,
} from '../lib/issue-display'
import { useIdentity } from '../lib/use-identity'
import { PriorityBadge, StatusBadge } from '../components/StatusBadge'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { Input } from '../components/ui/input'
import { Card, CardContent, CardHeader } from '../components/ui/card'
import { Separator } from '../components/ui/separator'
import { ScrollArea } from '../components/ui/scroll-area'

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
      <div className="p-6">
        <Link
          to="/"
          className="text-muted-foreground inline-flex items-center gap-1 text-sm no-underline"
        >
          <ArrowLeft className="size-4" />
          back to board
        </Link>
        <div className="border-destructive/40 bg-destructive/10 text-destructive mt-4 rounded-md border p-3 text-sm">
          Failed to load: {(error as Error).message}
        </div>
      </div>
    )
  }
  if (isLoading || !issue) {
    return <div className="text-muted-foreground p-6 text-sm">loading…</div>
  }

  const isCheckpoint = issue.type === 'checkpoint'
  const isOpen = issue.status === 'open' || issue.status === 'in_progress'

  return (
    <div className="flex h-full flex-col">
      {/* Page header with breadcrumb */}
      <header className="flex items-center justify-between gap-4 border-b px-6 py-3">
        <div className="flex items-center gap-3">
          <Button asChild variant="ghost" size="icon-sm">
            <Link to="/" aria-label="back">
              <ArrowLeft />
            </Link>
          </Button>
          <code className="text-muted-foreground bg-muted/50 rounded px-2 py-0.5 font-mono text-xs tabular">
            {issue.id}
          </code>
          <StatusBadge
            status={issue.status as Status}
            blocked={issue.blocked}
          />
          <PriorityBadge priority={issue.priority} />
        </div>
        <div className="flex items-center gap-1.5">
          {!issue.assignee && isOpen && (
            <Button size="sm" variant="outline" onClick={() => claim.mutate()}>
              <UserPlus />
              Claim
            </Button>
          )}
          {issue.status !== 'closed' && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => closeOrReopen.mutate('close')}
            >
              <CheckCircle2 />
              Close
            </Button>
          )}
          {issue.status === 'closed' && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => closeOrReopen.mutate('reopen')}
            >
              <RotateCcw />
              Reopen
            </Button>
          )}
        </div>
      </header>

      <ScrollArea className="min-h-0 flex-1">
        <div className="grid grid-cols-[minmax(0,1fr)_320px] gap-6 px-6 py-6">
          {/* Main column */}
          <div className="flex min-w-0 flex-col gap-5">
            <EditableTitle
              value={issue.title}
              onSave={(title) => patch.mutate({ title })}
            />

            <EditableDescription
              value={issue.description ?? ''}
              onSave={(description) =>
                patch.mutate({ description: description || null })
              }
            />

            {issue.notes && (
              <SectionCard title="Notes">
                <pre className="text-sm whitespace-pre-wrap font-sans">
                  {issue.notes}
                </pre>
              </SectionCard>
            )}

            {isCheckpoint && isOpen && (
              <CheckpointActions
                id={issue.id}
                onDone={(passed) => {
                  qc.invalidateQueries({ queryKey: ['issue', id] })
                  if (!passed) navigate({ to: '/' })
                }}
              />
            )}

            <CommentsSection
              issueId={issue.id}
              comments={issue.comments}
              onChange={() =>
                qc.invalidateQueries({ queryKey: ['issue', id] })
              }
            />
          </div>

          {/* Sidebar */}
          <aside className="flex flex-col gap-4 text-sm">
            <Field label="Status">
              <select
                value={issue.status}
                onChange={(e) => patch.mutate({ status: e.target.value })}
                className="border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring h-8 w-full rounded-md border px-2 py-1 text-sm focus-visible:outline-none focus-visible:ring-2"
              >
                {meta?.statuses.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </Field>

            <Field label="Priority">
              <Input
                type="number"
                min={0}
                value={issue.priority}
                onChange={(e) =>
                  patch.mutate({ priority: Number(e.target.value) })
                }
                className="h-8"
              />
            </Field>

            <Field label="Type">
              <select
                value={issue.type}
                onChange={(e) => patch.mutate({ type: e.target.value })}
                className="border-input bg-background h-8 w-full rounded-md border px-2 text-sm"
              >
                {meta?.types.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
            </Field>

            <Field label="Assignee">
              <NullableTextInput
                value={issue.assignee ?? ''}
                onSave={(v) => patch.mutate({ assignee: v || null })}
                placeholder="unassigned"
              />
            </Field>

            <Field label="Agent (lane)">
              <NullableTextInput
                value={issue.agent ?? ''}
                onSave={(v) => patch.mutate({ agent: v || null })}
                placeholder="—"
              />
            </Field>

            <Field label="Tags">
              <TagEditor
                tags={issue.labels.filter((l) => !isManaged(l))}
                onSave={(tags) => {
                  // Diff-preserving: server keeps run:/step:/etc.
                  const managed = issue.labels.filter(isManaged)
                  patch.mutate({ tags: [...tags, ...managed] })
                }}
              />
            </Field>

            {(issue.depends_on.length > 0 || issue.blocks.length > 0) && (
              <Field label="Dependencies">
                {issue.depends_on.length > 0 && (
                  <div className="mb-2">
                    <div className="text-muted-foreground mb-1 text-[11px] uppercase tracking-wide">
                      depends on
                    </div>
                    <div className="flex flex-wrap gap-1">
                      {issue.depends_on.map((d) => (
                        <Badge key={d} variant="outline" asChild>
                          <Link
                            to="/issues/$id"
                            params={{ id: d }}
                            className="font-mono no-underline"
                          >
                            {d}
                          </Link>
                        </Badge>
                      ))}
                    </div>
                  </div>
                )}
                {issue.blocks.length > 0 && (
                  <div>
                    <div className="text-muted-foreground mb-1 text-[11px] uppercase tracking-wide">
                      blocks
                    </div>
                    <div className="flex flex-wrap gap-1">
                      {issue.blocks.map((b) => (
                        <Badge key={b} variant="outline" asChild>
                          <Link
                            to="/issues/$id"
                            params={{ id: b }}
                            className="font-mono no-underline"
                          >
                            {b}
                          </Link>
                        </Badge>
                      ))}
                    </div>
                  </div>
                )}
              </Field>
            )}

            <Separator />

            <dl className="text-muted-foreground space-y-1 text-xs">
              <Meta label="Created" value={formatDate(issue.created)} />
              <Meta label="Updated" value={formatDate(issue.updated)} />
              {issue.closed && (
                <Meta label="Closed" value={formatDate(issue.closed)} />
              )}
            </dl>
          </aside>
        </div>
      </ScrollArea>
    </div>
  )
}

// ---- shared bits ----

function Field({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div>
      <div className="text-muted-foreground mb-1 text-[11px] font-medium uppercase tracking-wide">
        {label}
      </div>
      {children}
    </div>
  )
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between">
      <dt>{label}</dt>
      <dd className="font-mono tabular text-[11px]">{value}</dd>
    </div>
  )
}

function SectionCard({
  title,
  children,
  icon,
  action,
}: {
  title: string
  children: React.ReactNode
  icon?: React.ReactNode
  action?: React.ReactNode
}) {
  return (
    <Card>
      <CardHeader className="border-b pb-3">
        <div className="flex items-center justify-between">
          <div className="text-muted-foreground flex items-center gap-2 text-[11px] font-medium uppercase tracking-wide">
            {icon}
            {title}
          </div>
          {action}
        </div>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  )
}

// ---- inline editors ----

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
        className="hover:bg-accent/40 -mx-2 cursor-text rounded-md px-2 py-1 text-2xl font-semibold leading-tight tracking-tight"
        onClick={() => setEditing(true)}
      >
        {value}
      </h1>
    )
  }
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        if (draft.trim() && draft !== value) onSave(draft.trim())
        setEditing(false)
      }}
    >
      <Input
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
        className="h-auto px-2 py-1 text-2xl font-semibold"
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
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  useEffect(() => setDraft(value), [value])
  useEffect(() => {
    if (editing) textareaRef.current?.focus()
  }, [editing])

  if (!editing) {
    return (
      <Card
        className="hover:bg-accent/30 cursor-text gap-0 py-0"
        onClick={() => setEditing(true)}
      >
        <CardContent className="px-4 py-3">
          {value ? (
            <pre className="text-sm whitespace-pre-wrap font-sans">{value}</pre>
          ) : (
            <span className="text-muted-foreground text-sm">
              click to add a description…
            </span>
          )}
        </CardContent>
      </Card>
    )
  }
  return (
    <Card className="gap-0 py-0">
      <CardContent className="px-0 py-0">
        <textarea
          ref={textareaRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          rows={Math.max(4, draft.split('\n').length + 1)}
          className="placeholder:text-muted-foreground w-full resize-y rounded-t-xl bg-transparent p-4 text-sm outline-none"
        />
        <div className="flex justify-end gap-1 border-t p-2">
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
      </CardContent>
    </Card>
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
    <Input
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
      className="h-8"
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
        <Badge key={t} variant="secondary" className="gap-1 pl-1.5 pr-1">
          {t}
          <button
            onClick={() => onSave(tags.filter((x) => x !== t))}
            aria-label={`remove ${t}`}
            className="hover:bg-foreground/10 inline-flex size-3.5 items-center justify-center rounded-sm"
          >
            <X className="size-2.5" />
          </button>
        </Badge>
      ))}
      {adding ? (
        <Input
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
          className="h-6 w-24 text-xs"
        />
      ) : (
        <Button
          variant="outline"
          size="xs"
          onClick={() => setAdding(true)}
          className="border-dashed"
        >
          + tag
        </Button>
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
    <SectionCard
      title={`Comments (${comments.length})`}
      icon={<MessageSquare className="size-3.5" />}
    >
      <ul className="space-y-3">
        {comments.length === 0 && (
          <li className="text-muted-foreground py-2 text-xs">
            no comments yet
          </li>
        )}
        {comments.map((c) => (
          <li key={c.id} className="flex gap-3">
            <div className="bg-accent text-accent-foreground flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold uppercase">
              {c.author.charAt(0)}
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-baseline gap-2 text-xs">
                <span className="font-medium">{c.author}</span>
                <span className="text-muted-foreground">
                  {formatRelative(c.created)}
                </span>
              </div>
              <pre className="mt-1 text-sm whitespace-pre-wrap font-sans">
                {c.body}
              </pre>
            </div>
          </li>
        ))}
      </ul>

      <form
        className="mt-4 border-t pt-4"
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
            agent ? `comment as ${agent}…` : 'set identity in sidebar to comment'
          }
          disabled={!agent}
          className="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring w-full rounded-md border p-2 text-sm focus-visible:outline-none focus-visible:ring-2 disabled:opacity-50"
        />
        <div className="mt-2 flex items-center justify-between">
          {add.error && (
            <span className="text-destructive text-xs">
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
    </SectionCard>
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
    <Card className="border-amber-500/30 bg-amber-500/5">
      <CardHeader>
        <div className="text-muted-foreground flex items-center gap-2 text-[11px] font-medium uppercase tracking-wide">
          <ShieldCheck className="size-3.5" />
          Checkpoint
        </div>
      </CardHeader>
      <CardContent>
        <p className="text-muted-foreground text-xs">
          Approve to close the gate and unblock downstream work. Fail to
          cancel-cascade.
        </p>
        <Input
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="optional reason…"
          className="mt-3"
        />
        <div className="mt-3 flex gap-2">
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
            <span className="text-muted-foreground self-center text-xs">
              set identity first
            </span>
          )}
        </div>
        {resolve.error && (
          <p className="text-destructive mt-2 text-xs">
            {(resolve.error as Error).message}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function isManaged(label: string): boolean {
  return /^(run|step|checkpoint|cap|template):/.test(label)
}

import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import {
  ArrowLeft,
  Ban,
  CheckCircle2,
  MessageSquare,
  MoreHorizontal,
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
import { notifyError, notifyOk } from '../lib/toast-helpers'
import { PriorityBadge, StatusBadge } from '../components/StatusBadge'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { Input } from '../components/ui/input'
import { Card, CardContent, CardHeader } from '../components/ui/card'
import { Separator } from '../components/ui/separator'
import { ScrollArea } from '../components/ui/scroll-area'
import LiveIndicator from '../components/LiveIndicator'
import { Markdown } from '../components/Markdown'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '../components/ui/tooltip'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '../components/ui/dropdown-menu'

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
    onError: (err) => notifyError('Could not update issue', err),
  })

  const claim = useMutation({
    mutationFn: () => api.post<Issue>(`/api/issues/${id}/claim`),
    onSuccess: (i) => {
      qc.invalidateQueries({ queryKey: ['issue', id] })
      notifyOk('Claimed', `${i.id} assigned to ${i.assignee ?? 'you'}`)
    },
    onError: (err) => notifyError('Could not claim issue', err),
  })

  const closeOrReopen = useMutation({
    mutationFn: (action: 'close' | 'reopen') =>
      api.post<Issue>(`/api/issues/${id}/${action}`),
    onSuccess: (_data, action) => {
      qc.invalidateQueries({ queryKey: ['issue', id] })
      notifyOk(action === 'close' ? 'Closed' : 'Reopened')
    },
    onError: (err) => notifyError('Action failed', err),
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
      <header className="flex items-center justify-between gap-4 border-b px-6 py-4">
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
          <LiveIndicator />
          {!issue.assignee && isOpen && (
            <Button size="sm" variant="outline" onClick={() => claim.mutate()}>
              <UserPlus />
              Claim
            </Button>
          )}
          {issue.status !== 'closed' && issue.status !== 'cancelled' && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => closeOrReopen.mutate('close')}
            >
              <CheckCircle2 />
              Close
            </Button>
          )}
          {(issue.status === 'closed' || issue.status === 'cancelled') && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => closeOrReopen.mutate('reopen')}
            >
              <RotateCcw />
              Reopen
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label="more actions"
              >
                <MoreHorizontal />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => {
                  if (!window.confirm('Cancel this issue?'))
                    return
                  patch.mutate({ status: 'cancelled' })
                }}
              >
                <Ban />
                Cancel
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>

      <ScrollArea className="min-h-0 flex-1">
        <div className="mx-auto grid max-w-6xl grid-cols-[minmax(0,1fr)_320px] gap-8 px-6 py-8">
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
              <Select
                value={issue.status}
                onValueChange={(v) => patch.mutate({ status: v })}
              >
                <SelectTrigger size="sm" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {meta?.statuses.map((s) => (
                    <SelectItem key={s} value={s}>
                      {s.replace('_', ' ')}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field label="Priority">
              <Select
                value={String(issue.priority)}
                onValueChange={(v) => patch.mutate({ priority: Number(v) })}
              >
                <SelectTrigger size="sm" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[0, 1, 2, 3, 4].map((p) => (
                    <SelectItem key={p} value={String(p)}>
                      p{p}
                      {p === 0 ? ' (urgent)' : p === 4 ? ' (low)' : ''}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field label="Type">
              <Select
                value={issue.type}
                onValueChange={(v) => patch.mutate({ type: v })}
              >
                <SelectTrigger size="sm" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {meta?.types.map((t) => (
                    <SelectItem key={t} value={t}>
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field label="Assignee">
              <NullableTextInput
                value={issue.assignee ?? ''}
                onSave={(v) => patch.mutate({ assignee: v || null })}
                placeholder="unassigned"
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
              <TimeRow label="Created" t={issue.created} />
              <TimeRow label="Updated" t={issue.updated} />
              {issue.closed && <TimeRow label="Closed" t={issue.closed} />}
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

// TimeRow — relative "2h ago" with hover tooltip for the absolute
// timestamp. Far easier to scan than raw locale strings.
function TimeRow({ label, t }: { label: string; t: number }) {
  return (
    <div className="flex items-center justify-between">
      <dt>{label}</dt>
      <dd>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="tabular cursor-help text-[11px]">
              {formatRelative(t)}
            </span>
          </TooltipTrigger>
          <TooltipContent side="left">{formatDate(t)}</TooltipContent>
        </Tooltip>
      </dd>
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
            <Markdown source={value} />
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
      notifyOk('Comment posted')
    },
    onError: (err) => notifyError('Could not post comment', err),
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
              <div className="mt-1">
                <Markdown source={c.body} />
              </div>
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
    onSuccess: (_data, { pass }) => {
      onDone(pass)
      notifyOk(pass ? 'Checkpoint approved' : 'Checkpoint failed')
    },
    onError: (err) => notifyError('Checkpoint action failed', err),
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

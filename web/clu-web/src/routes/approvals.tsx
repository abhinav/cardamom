import { createFileRoute, Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { CheckCircle2, ShieldCheck, ThumbsDown, UserCheck } from 'lucide-react'
import { api, type PendingCheckpoint } from '../lib/api'
import { formatRelative } from '../lib/issue-display'
import { useIdentity } from '../lib/use-identity'
import { notifyError, notifyOk } from '../lib/toast-helpers'
import { Markdown } from '../components/Markdown'
import LiveIndicator from '../components/LiveIndicator'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { Card, CardContent, CardHeader } from '../components/ui/card'
import { Input } from '../components/ui/input'
import { ScrollArea } from '../components/ui/scroll-area'

export const Route = createFileRoute('/approvals')({
  component: ApprovalsPage,
})

function ApprovalsPage() {
  const { data: pending = [], isLoading, error } = useQuery<PendingCheckpoint[]>({
    queryKey: ['checkpoints'],
    queryFn: () => api.get('/api/checkpoints'),
  })

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between gap-4 border-b px-6 py-4">
        <div>
          <h1 className="text-lg font-semibold leading-none tracking-tight">
            Approvals
          </h1>
          <p className="text-muted-foreground mt-1.5 text-xs">
            Checkpoints waiting for a human decision
          </p>
        </div>
        <LiveIndicator />
      </header>

      {error && (
        <div className="border-destructive/40 bg-destructive/10 text-destructive m-6 rounded-md border p-3 text-sm">
          {(error as Error).message}
        </div>
      )}

      <ScrollArea className="min-h-0 flex-1">
        <div className="mx-auto max-w-3xl space-y-4 px-6 py-6">
          {isLoading && (
            <div className="text-muted-foreground py-8 text-center text-sm">
              loading…
            </div>
          )}
          {!isLoading && pending.length === 0 && (
            <div className="rounded-lg border border-dashed py-12 text-center">
              <CheckCircle2 className="text-muted-foreground mx-auto size-6" />
              <p className="mt-2 text-sm font-medium">Inbox zero</p>
              <p className="text-muted-foreground mt-1 text-xs">
                No checkpoints waiting on approval right now.
              </p>
            </div>
          )}
          {pending.map((cp) => (
            <CheckpointCard key={cp.id} cp={cp} />
          ))}
        </div>
      </ScrollArea>
    </div>
  )
}

function CheckpointCard({ cp }: { cp: PendingCheckpoint }) {
  const qc = useQueryClient()
  const [agent] = useIdentity()
  const [reason, setReason] = useState('')

  // Approval-kind checkpoints require the approver to be on the
  // declared list. Manual-kind accepts anyone with a set identity.
  const approvers = cp.approvers ?? []
  const requiresList = cp.kind === 'approval' && approvers.length > 0
  const youCanApprove =
    !!agent && (!requiresList || approvers.includes(agent))

  const resolve = useMutation({
    mutationFn: ({ pass }: { pass: boolean }) =>
      api.post(`/api/checkpoints/${cp.id}/${pass ? 'approve' : 'fail'}`, {
        reason: reason.trim() || undefined,
      }),
    onSuccess: (_d, { pass }) => {
      notifyOk(pass ? `Approved ${cp.id}` : `Rejected ${cp.id}`)
      qc.invalidateQueries({ queryKey: ['checkpoints'] })
      qc.invalidateQueries({ queryKey: ['issues'] })
    },
    onError: (err) => notifyError('Checkpoint action failed', err),
  })

  return (
    <Card>
      <CardHeader className="border-b pb-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <ShieldCheck className="text-amber-400 size-4 shrink-0" />
              <Link
                to="/issues/$id"
                params={{ id: cp.id }}
                className="text-foreground text-base font-semibold no-underline hover:underline"
              >
                {cp.title}
              </Link>
            </div>
            <div className="text-muted-foreground mt-1 flex items-center gap-2 text-xs">
              <code className="bg-muted/50 rounded px-1.5 py-0.5 font-mono">
                {cp.id}
              </code>
              <span>·</span>
              <span>opened {formatRelative(cp.created)}</span>
              {cp.kind === 'manual' && (
                <>
                  <span>·</span>
                  <Badge variant="muted">manual</Badge>
                </>
              )}
            </div>
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {cp.description && (
          <div className="bg-muted/30 rounded-md border p-3">
            <Markdown source={cp.description} />
          </div>
        )}

        {/* Approvers strip */}
        {requiresList && (
          <div>
            <div className="text-muted-foreground mb-1.5 flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide">
              <UserCheck className="size-3.5" />
              Approvers
            </div>
            <div className="flex flex-wrap gap-1.5">
              {approvers.map((a) => {
                const isYou = a === agent
                return (
                  <Badge
                    key={a}
                    variant={isYou ? 'success' : 'muted'}
                    title={isYou ? 'that’s you' : undefined}
                  >
                    {a}
                    {isYou ? ' (you)' : ''}
                  </Badge>
                )
              })}
            </div>
          </div>
        )}

        {/* What this gates */}
        {cp.blocks.length > 0 && (
          <div>
            <div className="text-muted-foreground mb-1.5 text-[11px] font-medium uppercase tracking-wide">
              Blocks ({cp.blocks.length})
            </div>
            <div className="flex flex-wrap gap-1">
              {cp.blocks.map((b) => (
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

        {/* Action row */}
        <div className="border-t pt-3">
          <Input
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="optional reason / note…"
            className="mb-2"
          />
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              disabled={!youCanApprove || resolve.isPending}
              onClick={() => resolve.mutate({ pass: true })}
              title={
                youCanApprove
                  ? 'approve and close the gate'
                  : !agent
                    ? 'set identity to approve'
                    : `only ${approvers.join(', ')} can approve`
              }
            >
              <CheckCircle2 />
              Approve
            </Button>
            <Button
              size="sm"
              variant="destructive"
              disabled={!agent || resolve.isPending}
              onClick={() => {
                if (
                  !window.confirm(
                    'Reject this checkpoint? Cascade cancels every downstream step.',
                  )
                )
                  return
                resolve.mutate({ pass: false })
              }}
              title="reject + cancel-cascade downstream"
            >
              <ThumbsDown />
              Reject
            </Button>
            {!agent && (
              <span className="text-muted-foreground text-xs">
                set identity in the sidebar to act
              </span>
            )}
            {agent && requiresList && !approvers.includes(agent) && (
              <span className="text-muted-foreground text-xs">
                only {approvers.join(', ')} can approve
              </span>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

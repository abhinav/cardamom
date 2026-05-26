import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Sparkles } from 'lucide-react'
import { api, type Issue } from '../lib/api'
import { formatRelative, type Status } from '../lib/issue-display'
import { PriorityBadge, StatusBadge } from '../components/StatusBadge'
import LiveIndicator from '../components/LiveIndicator'
import { Badge } from '../components/ui/badge'
import { ScrollArea } from '../components/ui/scroll-area'

export const Route = createFileRoute('/ready')({
  component: ReadyPage,
})

// "Ready" answers the question every agent (and human) opens this UI
// for: what can I actually pick up right now? Server filter narrows
// to open + unassigned; we drop blocked client-side since `blocked`
// is a derived field (no server-side filter for it today).
function ReadyPage() {
  const { data: issues = [], isLoading, error } = useQuery<Issue[]>({
    queryKey: ['issues', 'ready'],
    queryFn: () =>
      api.get('/api/issues?status=open&no_assignee=1&limit=500'),
  })

  const ready = issues.filter((i) => !i.blocked)

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between gap-4 border-b px-6 py-4">
        <div>
          <h1 className="text-lg font-semibold leading-none tracking-tight">
            Ready
          </h1>
          <p className="text-muted-foreground mt-1.5 text-xs">
            Open, unassigned, not blocked by dependencies
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
        <div className="mx-auto max-w-4xl px-6 py-6">
          {isLoading && (
            <div className="text-muted-foreground py-8 text-center text-sm">
              loading…
            </div>
          )}
          {!isLoading && ready.length === 0 && (
            <div className="rounded-lg border border-dashed py-12 text-center">
              <Sparkles className="text-muted-foreground mx-auto size-6" />
              <p className="mt-2 text-sm font-medium">All caught up</p>
              <p className="text-muted-foreground mt-1 text-xs">
                No unassigned, unblocked issues right now.
              </p>
            </div>
          )}
          <ul className="space-y-2">
            {ready.map((i) => (
              <li key={i.id}>
                <Link
                  to="/issues/$id"
                  params={{ id: i.id }}
                  className="bg-card hover:bg-accent/60 hover:border-foreground/20 block rounded-lg border p-3 no-underline transition-colors"
                >
                  <div className="flex items-start gap-3">
                    <PriorityBadge priority={i.priority} className="mt-0.5 shrink-0" />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-baseline gap-2">
                        <span className="text-sm font-medium">{i.title}</span>
                        <code className="text-muted-foreground bg-muted/50 rounded px-1.5 py-0.5 font-mono text-[10px] tabular">
                          {i.id}
                        </code>
                      </div>
                      <div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-2 text-xs">
                        <StatusBadge status={i.status as Status} blocked={false} />
                        <span>{i.type}</span>
                        <span>·</span>
                        <span>{formatRelative(i.updated)}</span>
                        {i.labels
                          .filter((l) => !/^(run|step|checkpoint|cap|template):/.test(l))
                          .slice(0, 4)
                          .map((l) => (
                            <Badge key={l} variant="muted">
                              {l}
                            </Badge>
                          ))}
                      </div>
                    </div>
                  </div>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      </ScrollArea>
    </div>
  )
}

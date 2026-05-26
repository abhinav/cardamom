import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { AlertTriangle, GitBranch, Play } from 'lucide-react'
import { api, type TemplatesList, type TemplateSummary } from '../lib/api'
import LiveIndicator from '../components/LiveIndicator'
import RunWorkflowDialog from '../components/RunWorkflowDialog'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { Card, CardContent, CardHeader } from '../components/ui/card'
import { ScrollArea } from '../components/ui/scroll-area'

export const Route = createFileRoute('/workflows')({
  component: WorkflowsPage,
})

// WorkflowsPage — discovery + launch UI for workflow templates.
// Templates live as YAML in <project>/templates; this page is the
// only way to fire one from outside the CLI. Pairs with the
// Approvals page since templates can include checkpoint gates.
function WorkflowsPage() {
  const [running, setRunning] = useState<TemplateSummary | null>(null)
  const { data, isLoading, error } = useQuery<TemplatesList>({
    queryKey: ['templates'],
    queryFn: () => api.get('/api/templates'),
  })

  const templates = data?.templates ?? []
  const loadErrors = data?.errors ?? []

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between gap-4 border-b px-6 py-4">
        <div>
          <h1 className="text-lg font-semibold leading-none tracking-tight">
            Workflows
          </h1>
          <p className="text-muted-foreground mt-1.5 text-xs">
            Pre-defined multi-step processes you can instantiate
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
        <div className="mx-auto max-w-4xl space-y-4 px-6 py-6">
          {/* LoadErrors — broken YAML files surfaced so they don't go
              unnoticed. Healthy templates still render below. */}
          {loadErrors.length > 0 && (
            <Card className="border-amber-500/40 bg-amber-500/5">
              <CardHeader>
                <div className="flex items-center gap-2 text-amber-300 text-[11px] font-medium uppercase tracking-wide">
                  <AlertTriangle className="size-3.5" />
                  {loadErrors.length} template
                  {loadErrors.length === 1 ? '' : 's'} failed to load
                </div>
              </CardHeader>
              <CardContent>
                <ul className="space-y-1 text-xs">
                  {loadErrors.map((e) => (
                    <li key={e.file}>
                      <code className="font-mono">{e.file}</code>
                      <span className="text-muted-foreground"> — {e.error}</span>
                    </li>
                  ))}
                </ul>
              </CardContent>
            </Card>
          )}

          {isLoading && (
            <div className="text-muted-foreground py-8 text-center text-sm">
              loading…
            </div>
          )}
          {!isLoading && templates.length === 0 && (
            <div className="rounded-lg border border-dashed py-12 text-center">
              <GitBranch className="text-muted-foreground mx-auto size-6" />
              <p className="mt-2 text-sm font-medium">No templates yet</p>
              <p className="text-muted-foreground mt-1 text-xs">
                Add YAML files to{' '}
                <code className="font-mono">.clu/templates/</code> and they'll
                appear here.
              </p>
            </div>
          )}

          {templates.map((t) => (
            <TemplateCard
              key={t.name}
              template={t}
              onRun={() => setRunning(t)}
            />
          ))}
        </div>
      </ScrollArea>

      <RunWorkflowDialog
        template={running}
        onOpenChange={(open) => !open && setRunning(null)}
      />
    </div>
  )
}

function TemplateCard({
  template,
  onRun,
}: {
  template: TemplateSummary
  onRun: () => void
}) {
  const required = template.vars.filter((v) => v.required).length
  return (
    <Card>
      <CardHeader className="border-b pb-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <GitBranch className="text-muted-foreground size-4 shrink-0" />
              <h2 className="text-base font-semibold">{template.name}</h2>
            </div>
            {template.description && (
              <p className="text-muted-foreground mt-1 whitespace-pre-wrap text-xs">
                {template.description.trim()}
              </p>
            )}
          </div>
          <Button size="sm" onClick={onRun}>
            <Play />
            Run
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-1.5 text-xs">
          <Badge variant="muted" className="font-mono tabular">
            {template.step_count} step{template.step_count === 1 ? '' : 's'}
          </Badge>
          {template.vars.length > 0 && (
            <Badge variant="muted">
              {template.vars.length} var{template.vars.length === 1 ? '' : 's'}
              {required > 0 && ` (${required} required)`}
            </Badge>
          )}
          {template.vars.map((v) => (
            <Badge
              key={v.name}
              variant={v.required ? 'warning' : 'outline'}
              title={
                v.pattern
                  ? `pattern: ${v.pattern}`
                  : v.default
                    ? `default: ${v.default}`
                    : v.required
                      ? 'required'
                      : 'optional'
              }
              className="font-mono"
            >
              {v.required && '*'}
              {v.name}
            </Badge>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

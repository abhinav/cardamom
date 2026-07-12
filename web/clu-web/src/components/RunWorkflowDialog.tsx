import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { GitBranch, Play } from 'lucide-react'
import {
  api,
  ApiError,
  type Plan,
  type RunResult,
  type TemplateSummary,
} from '../lib/api'
import { notifyError, notifyOk } from '../lib/toast-helpers'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { WorkflowGraph } from './WorkflowGraph'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/dialog'

interface Props {
  template: TemplateSummary | null
  onOpenChange: (open: boolean) => void
}

// RunWorkflowDialog — full-scope run experience. Dynamic form per
// template var (respects required / default / pattern / label),
// debounced plan preview that re-fetches on var change, Run commits
// + navigates to the parent issue.
//
// `template` doubles as the open signal — pass a TemplateSummary to
// open the dialog for that template; pass null to close. Avoids the
// race where the dialog tries to render a stale template after the
// parent clears it.
export default function RunWorkflowDialog({ template, onOpenChange }: Props) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const open = template !== null

  // Form state. Initialised from each var's default on template open
  // so the preview is meaningful before the user types anything.
  const [vars, setVars] = useState<Record<string, string>>({})
  useEffect(() => {
    if (!template) return
    const init: Record<string, string> = {}
    for (const v of template.vars) {
      if (v.default !== undefined) init[v.name] = v.default
    }
    setVars(init)
  }, [template])

  // Debounced preview key — re-renders the dialog cheaply on every
  // keystroke, but the plan query only re-runs ~250ms after the last
  // change. queryKey carries the var snapshot so TanStack Query
  // dedupes equivalent re-renders.
  const [debouncedVars, setDebouncedVars] = useState(vars)
  useEffect(() => {
    const t = setTimeout(() => setDebouncedVars(vars), 250)
    return () => clearTimeout(t)
  }, [vars])

  // Plan preview. Disabled until the dialog is open + template
  // resolved. Errors here are normal (missing required var, pattern
  // mismatch) and surface as inline form feedback — not a toast.
  const planQuery = useQuery<Plan, ApiError>({
    queryKey: ['template-plan', template?.name, debouncedVars],
    queryFn: () =>
      api.post(`/api/templates/${template!.name}/plan`, { vars: debouncedVars }),
    enabled: open,
    retry: false,
  })

  // Client-side pattern check — server validates too, but surfacing
  // it locally lets us highlight the offending field without a
  // round-trip. Computed only when the field has a value.
  const fieldErrors = useMemo(() => {
    if (!template) return {}
    const out: Record<string, string> = {}
    for (const v of template.vars) {
      const value = vars[v.name] ?? ''
      if (v.required && value.trim() === '') {
        out[v.name] = 'required'
        continue
      }
      if (value && v.pattern) {
        try {
          const re = new RegExp(v.pattern)
          if (!re.test(value)) out[v.name] = `must match ${v.pattern}`
        } catch {
          // bad pattern is the template author's problem; server returns 400
        }
      }
    }
    return out
  }, [template, vars])

  const hasFieldErrors = Object.keys(fieldErrors).length > 0

  const run = useMutation<RunResult, ApiError>({
    mutationFn: () =>
      api.post(`/api/templates/${template!.name}/run`, { vars }),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ['issues'] })
      qc.invalidateQueries({ queryKey: ['checkpoints'] })
      notifyOk(`Started ${template!.name}`, `${res.parent_id} + ${res.plan.steps.length} steps`)
      onOpenChange(false)
      navigate({ to: '/issues/$id', params: { id: res.parent_id } })
    },
    onError: (err) => notifyError('Run failed', err),
  })

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onOpenChange(false)}>
      <DialogContent
        // Wider than the default dialog — large workflows (17+ steps)
        // need horizontal room for the L→R graph, and the 3-col var
        // grid above looks anaemic in a narrower frame.
        className="sm:max-w-[min(96vw,1100px)]"
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault()
            if (!hasFieldErrors && !run.isPending) run.mutate()
          }
        }}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GitBranch className="text-muted-foreground size-4" />
            Run {template?.name}
          </DialogTitle>
          {template?.description && (
            <DialogDescription className="whitespace-pre-wrap">
              {template.description.trim()}
            </DialogDescription>
          )}
        </DialogHeader>

        {/* Variables — 3-column grid stacked above the graph so wide
            graphs aren't fighting a sidebar for horizontal room. */}
        <section className="space-y-2">
          <div className="text-muted-foreground text-[11px] font-medium uppercase tracking-wide">
            Variables
          </div>
          {template?.vars.length === 0 && (
            <p className="text-muted-foreground text-sm">
              This template has no variables. Just hit Run.
            </p>
          )}
          {template && template.vars.length > 0 && (
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {template.vars.map((v) => (
                <div key={v.name}>
                  <label className="mb-1 flex items-baseline gap-1.5 text-xs font-medium">
                    <span className="truncate">{v.label || v.name}</span>
                    {v.required && (
                      <span className="text-amber-400" title="required">
                        *
                      </span>
                    )}
                  </label>
                  <Input
                    value={vars[v.name] ?? ''}
                    onChange={(e) =>
                      setVars((prev) => ({ ...prev, [v.name]: e.target.value }))
                    }
                    placeholder={v.default ? `default: ${v.default}` : v.name}
                    aria-invalid={fieldErrors[v.name] ? 'true' : undefined}
                    title={
                      v.pattern
                        ? `must match /${v.pattern}/`
                        : v.default
                          ? `default: ${v.default}`
                          : undefined
                    }
                    className="h-8"
                  />
                  {/* Pattern hint moved under the input so it doesn't
                      compete with the label for the same line; muted
                      and easy to ignore once you've seen it. */}
                  {v.pattern && !fieldErrors[v.name] && (
                    <p className="text-muted-foreground mt-1 font-mono text-[10px]">
                      /{v.pattern}/
                    </p>
                  )}
                  {fieldErrors[v.name] && (
                    <p className="text-destructive mt-1 text-[11px]">
                      {fieldErrors[v.name]}
                    </p>
                  )}
                </div>
              ))}
            </div>
          )}
        </section>

        {/* Plan preview — full width, left-to-right graph. Step
            count grows horizontally rather than fighting the dialog's
            vertical space. */}
        <section className="space-y-2">
          <div className="text-muted-foreground flex items-center justify-between text-[11px] font-medium uppercase tracking-wide">
            <span>Preview</span>
            {planQuery.isFetching && (
              <span className="text-muted-foreground/70 normal-case font-normal">
                refreshing…
              </span>
            )}
          </div>
          {planQuery.error && !planQuery.data && (
            <div className="border-destructive/40 bg-destructive/10 text-destructive rounded-md border p-2 text-xs">
              {planQuery.error.message}
            </div>
          )}
          {planQuery.data && (
            <WorkflowGraph plan={planQuery.data} className="h-[400px] w-full" />
          )}
          {!planQuery.data && !planQuery.error && (
            <div className="text-muted-foreground rounded-md border border-dashed p-3 text-xs">
              fill required vars to preview
            </div>
          )}
        </section>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            disabled={hasFieldErrors || run.isPending || !planQuery.data}
            onClick={() => run.mutate()}
            title="Run + open the parent issue (⌘↵)"
          >
            <Play />
            {run.isPending ? 'Running…' : 'Run'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}


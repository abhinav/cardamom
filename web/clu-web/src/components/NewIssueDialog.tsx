import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useNavigate } from '@tanstack/react-router'
import { api, type CreateIssueBody, type Issue, type Meta } from '../lib/api'
import { notifyError, notifyOk } from '../lib/toast-helpers'
import { Button } from './ui/button'
import { Input } from './ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './ui/select'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  // Optional preset for "+ New" launched from a specific kanban
  // column or filter — currently only `agent` (lane) is reachable
  // from the UI; status defaults to open server-side.
  presetAgent?: string | null
}

// NewIssueDialog — the canonical "create issue" form. Used by both the
// board's "+ New" header button and the per-column quick-add buttons.
// Submits via POST /api/issues, then either navigates to the new
// issue's detail page (if user wants to fill in more) or just toasts
// and closes.
export default function NewIssueDialog({ open, onOpenChange, presetAgent }: Props) {
  const qc = useQueryClient()
  const navigate = useNavigate()

  const { data: meta } = useQuery<Meta>({
    queryKey: ['meta'],
    queryFn: () => api.get('/api/meta'),
    enabled: open,
  })

  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [type, setType] = useState('task')
  const [priority, setPriority] = useState(2)
  const [tags, setTags] = useState('')
  const [agent, setAgent] = useState(presetAgent ?? '')

  // Reset form when the dialog opens — otherwise the previous draft
  // bleeds across opens.
  useEffect(() => {
    if (open) {
      setTitle('')
      setDescription('')
      setType('task')
      setPriority(2)
      setTags('')
      setAgent(presetAgent ?? '')
    }
  }, [open, presetAgent])

  const create = useMutation({
    mutationFn: (body: CreateIssueBody) => api.post<Issue>('/api/issues', body),
    onSuccess: (i) => {
      qc.invalidateQueries({ queryKey: ['issues'] })
      notifyOk(`Created ${i.id}`, i.title)
      onOpenChange(false)
    },
    onError: (err) => notifyError('Could not create issue', err),
  })

  function submit(navigateToDetail: boolean) {
    const t = title.trim()
    if (!t) return
    const labels = tags
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    create.mutate(
      {
        title: t,
        type,
        priority,
        description: description.trim() || undefined,
        labels: labels.length ? labels : undefined,
        agent: agent.trim() || null,
      } as CreateIssueBody & { description?: string },
      {
        onSuccess: (i) => {
          if (navigateToDetail) {
            navigate({ to: '/issues/$id', params: { id: i.id } })
          }
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-xl"
        onKeyDown={(e) => {
          // Cmd/Ctrl-Enter submits + stays put; Cmd/Ctrl-Shift-Enter
          // navigates to the new issue. Plain Enter inside the title
          // field also submits (more discoverable).
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault()
            submit(e.shiftKey)
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>New issue</DialogTitle>
          <DialogDescription>
            Enter a short title. Everything else is optional — you can edit
            after creation.
          </DialogDescription>
        </DialogHeader>

        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault()
            submit(false)
          }}
        >
          <Input
            autoFocus
            placeholder="Title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className="text-base"
          />

          <textarea
            placeholder="Description (optional, markdown supported)"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={4}
            className="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring w-full resize-y rounded-md border p-2 text-sm focus-visible:outline-none focus-visible:ring-2"
          />

          <div className="grid grid-cols-3 gap-2">
            <LabelledField label="Type">
              <Select value={type} onValueChange={setType}>
                <SelectTrigger size="sm" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(meta?.types ?? ['task']).map((t) => (
                    <SelectItem key={t} value={t}>
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </LabelledField>

            <LabelledField label="Priority">
              <Select
                value={String(priority)}
                onValueChange={(v) => setPriority(Number(v))}
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
            </LabelledField>

            <LabelledField label="Agent (lane)">
              <Input
                value={agent}
                onChange={(e) => setAgent(e.target.value)}
                placeholder="unassigned"
                className="h-8"
              />
            </LabelledField>
          </div>

          <LabelledField label="Tags">
            <Input
              value={tags}
              onChange={(e) => setTags(e.target.value)}
              placeholder="comma-separated (e.g. backend, urgent)"
              className="h-8"
            />
          </LabelledField>
        </form>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="outline"
            disabled={!title.trim() || create.isPending}
            onClick={() => submit(true)}
            title="Create and open the issue (⇧⌘↵)"
          >
            Create + open
          </Button>
          <Button
            disabled={!title.trim() || create.isPending}
            onClick={() => submit(false)}
            title="Create and stay (⌘↵)"
          >
            <Plus />
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function LabelledField({
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

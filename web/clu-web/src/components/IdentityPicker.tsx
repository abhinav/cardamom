import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronsUpDown, User } from 'lucide-react'
import { api, type ActiveAgent } from '../lib/api'
import { useIdentity } from '../lib/use-identity'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover'

// IdentityPicker — sidebar control for "who am I". Sets a localStorage
// key that the API client attaches as X-Clu-Agent on every mutation.
// Every claim/comment/checkpoint needs an identity, so a missing
// value is hinted with a muted "set identity".
export default function IdentityPicker() {
  const [agent, setAgent] = useIdentity()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState(agent)

  const { data: agents = [] } = useQuery<ActiveAgent[]>({
    queryKey: ['agents'],
    queryFn: () => api.get('/api/agents'),
    refetchInterval: 10_000,
    enabled: open,
  })

  function save(name: string) {
    setAgent(name.trim())
    setOpen(false)
  }

  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (o) setDraft(agent)
      }}
    >
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="w-full justify-between font-normal"
        >
          <span className="flex items-center gap-2 truncate">
            <User className="size-3.5" />
            <span className="truncate">
              {agent || (
                <span className="text-muted-foreground">set identity</span>
              )}
            </span>
          </span>
          <ChevronsUpDown className="size-3.5 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        side="right"
        align="end"
        sideOffset={8}
        className="w-64 p-2"
      >
        <form
          className="flex gap-1"
          onSubmit={(e) => {
            e.preventDefault()
            save(draft)
          }}
        >
          <Input
            autoFocus
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder="your-name"
            className="h-8"
          />
          <Button size="sm" type="submit">
            Save
          </Button>
        </form>
        {agents.length > 0 && (
          <>
            <div className="text-muted-foreground mt-3 px-1 text-[10px] font-medium uppercase tracking-wide">
              Live agents
            </div>
            <ul className="mt-1 space-y-0.5">
              {agents.map((a) => (
                <li key={a.name}>
                  <button
                    type="button"
                    className="hover:bg-accent w-full rounded-sm px-2 py-1 text-left text-sm"
                    onClick={() => save(a.name)}
                  >
                    {a.name}
                  </button>
                </li>
              ))}
            </ul>
          </>
        )}
        <p className="text-muted-foreground mt-3 px-1 text-[11px]">
          Sent as <code className="font-mono">X-Clu-Agent</code> on writes.
        </p>
      </PopoverContent>
    </Popover>
  )
}

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type ActiveAgent } from '../lib/api'
import { useIdentity } from '../lib/use-identity'
import { Input } from './ui/input'
import { Button } from './ui/button'

// IdentityPicker is the "who am I" control. Bare input that writes
// to localStorage on change; an inline list of currently-live agents
// (from /api/agents) shows below so users can click to pick.
//
// Localhost-only tool — no auth — but every mutation (claim, comment,
// checkpoint approve) needs an identity, so this is required UI.
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
    <div className="relative">
      <Button
        variant="outline"
        size="sm"
        onClick={() => {
          setDraft(agent)
          setOpen((o) => !o)
        }}
      >
        <span className="opacity-60">as</span>
        <span className="font-semibold">
          {agent || '(unset)'}
        </span>
      </Button>
      {open && (
        <div className="absolute right-0 top-full mt-1 z-50 w-64 rounded-lg border border-[var(--line)] bg-[var(--surface-strong)] p-3 shadow-lg backdrop-blur-md">
          <label className="text-xs font-semibold uppercase tracking-wide opacity-60">
            Agent identity
          </label>
          <form
            className="mt-1 flex gap-1"
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
              className="text-sm"
            />
            <Button size="sm" type="submit">
              Save
            </Button>
          </form>
          {agents.length > 0 && (
            <div className="mt-3">
              <div className="text-xs font-semibold uppercase tracking-wide opacity-60">
                Live agents
              </div>
              <ul className="mt-1 space-y-0.5">
                {agents.map((a) => (
                  <li key={a.name}>
                    <button
                      className="w-full rounded px-2 py-1 text-left text-sm hover:bg-[var(--link-bg-hover)]"
                      onClick={() => save(a.name)}
                    >
                      {a.name}
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <p className="mt-3 text-xs opacity-60">
            Stored locally; sent as <code>X-Clu-Agent</code> on writes.
          </p>
        </div>
      )}
    </div>
  )
}

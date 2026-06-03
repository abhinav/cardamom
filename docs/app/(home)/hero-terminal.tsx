'use client'

import { useEffect, useRef, useState } from 'react'

type TicketState = 'blocked' | 'ready' | 'in_progress' | 'closed'
type Ticket = { id: string; title: string; prio?: number; state: TicketState }
type Line = { kind: 'cmd' | 'out'; text: string }

type Step =
  | { t: 'cmd'; text: string }
  | { t: 'out'; lines: string[] }
  | { t: 'board'; tickets: Ticket[] }
  | { t: 'wait'; ms: number }

const A = 'clu-a3f8'
const B = 'clu-b210'

// The looping demo script: type a command, print its output, then mutate the
// little ticket board so the two stay in sync — the product's core loop.
const SCRIPT: Step[] = [
  { t: 'cmd', text: 'clu init' },
  { t: 'out', lines: ['✓ created .clu/data.sqlite'] },
  { t: 'wait', ms: 450 },

  { t: 'cmd', text: 'clu create -p1 "fix login redirect"' },
  { t: 'out', lines: [`→ ${A}`] },
  { t: 'board', tickets: [{ id: A, title: 'fix login redirect', prio: 1, state: 'ready' }] },
  { t: 'wait', ms: 450 },

  { t: 'cmd', text: `clu create -d ${A} "add redirect tests"` },
  { t: 'out', lines: [`→ ${B}  (blocked)`] },
  {
    t: 'board',
    tickets: [
      { id: A, title: 'fix login redirect', prio: 1, state: 'ready' },
      { id: B, title: 'add redirect tests', state: 'blocked' },
    ],
  },
  { t: 'wait', ms: 650 },

  { t: 'cmd', text: 'clu claim' },
  { t: 'out', lines: [`claimed ${A}`] },
  {
    t: 'board',
    tickets: [
      { id: A, title: 'fix login redirect', prio: 1, state: 'in_progress' },
      { id: B, title: 'add redirect tests', state: 'blocked' },
    ],
  },
  { t: 'wait', ms: 750 },

  { t: 'cmd', text: `clu close ${A}` },
  { t: 'out', lines: [`closed ${A} · unblocked ${B}`] },
  {
    t: 'board',
    tickets: [
      { id: A, title: 'fix login redirect', prio: 1, state: 'closed' },
      { id: B, title: 'add redirect tests', state: 'ready' },
    ],
  },
  { t: 'wait', ms: 750 },

  { t: 'cmd', text: 'clu claim' },
  { t: 'out', lines: [`claimed ${B}`] },
  {
    t: 'board',
    tickets: [
      { id: A, title: 'fix login redirect', prio: 1, state: 'closed' },
      { id: B, title: 'add redirect tests', state: 'in_progress' },
    ],
  },
  { t: 'wait', ms: 750 },

  { t: 'cmd', text: `clu close ${B}` },
  { t: 'out', lines: [`closed ${B}  ✓ all clear`] },
  {
    t: 'board',
    tickets: [
      { id: A, title: 'fix login redirect', prio: 1, state: 'closed' },
      { id: B, title: 'add redirect tests', state: 'closed' },
    ],
  },
  { t: 'wait', ms: 2200 },
]

const STATE_META: Record<TicketState, { label: string; dot: string; ring: string }> = {
  blocked: { label: 'blocked', dot: 'oklch(0.6 0.02 240)', ring: 'oklch(0.6 0.02 240 / 35%)' },
  ready: { label: 'ready', dot: 'oklch(0.78 0.16 195)', ring: 'oklch(0.78 0.16 195 / 45%)' },
  in_progress: { label: 'in progress', dot: 'oklch(0.78 0.14 60)', ring: 'oklch(0.78 0.14 60 / 45%)' },
  closed: { label: 'closed', dot: 'oklch(0.72 0.17 150)', ring: 'oklch(0.72 0.17 150 / 40%)' },
}

// Static final frame for reduced-motion / no-JS: the whole loop, settled.
const STATIC_LINES: Line[] = SCRIPT.flatMap((s) =>
  s.t === 'cmd' ? [{ kind: 'cmd', text: s.text } as Line] : s.t === 'out' ? s.lines.map((l) => ({ kind: 'out', text: l }) as Line) : [],
)
const STATIC_TICKETS = (SCRIPT.filter((s) => s.t === 'board').at(-1) as Extract<Step, { t: 'board' }>).tickets

export function HeroTerminal() {
  const [lines, setLines] = useState<Line[]>([])
  const [typing, setTyping] = useState('')
  const [tickets, setTickets] = useState<Ticket[]>([])
  const [done, setDone] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const reduce =
      typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (reduce) {
      setLines(STATIC_LINES)
      setTickets(STATIC_TICKETS)
      setDone(true)
      return
    }

    let cancelled = false
    const timers: ReturnType<typeof setTimeout>[] = []
    const sleep = (ms: number) =>
      new Promise<void>((res) => {
        timers.push(setTimeout(res, ms))
      })

    async function run() {
      while (!cancelled) {
        setLines([])
        setTyping('')
        setTickets([])
        for (const step of SCRIPT) {
          if (cancelled) return
          if (step.t === 'cmd') {
            for (let i = 1; i <= step.text.length; i++) {
              if (cancelled) return
              setTyping(step.text.slice(0, i))
              await sleep(26)
            }
            setLines((l) => [...l, { kind: 'cmd', text: step.text }])
            setTyping('')
            await sleep(140)
          } else if (step.t === 'out') {
            setLines((l) => [...l, ...step.lines.map((t) => ({ kind: 'out', text: t }) as Line)])
            await sleep(140)
          } else if (step.t === 'board') {
            setTickets(step.tickets)
          } else if (step.t === 'wait') {
            await sleep(step.ms)
          }
        }
      }
    }
    run()
    return () => {
      cancelled = true
      timers.forEach(clearTimeout)
    }
  }, [])

  return (
    <div className="grid gap-4 md:grid-cols-5">
      {/* Terminal */}
      <div
        className="overflow-hidden rounded-xl border md:col-span-3"
        style={{ borderColor: 'oklch(1 0 0 / 10%)', background: 'oklch(0.08 0.01 240)' }}
      >
        <div
          className="flex items-center gap-2 border-b px-4 py-2.5"
          style={{ borderColor: 'oklch(1 0 0 / 5%)' }}
        >
          <div className="flex gap-1.5">
            <span className="block h-2.5 w-2.5 rounded-full bg-red-500/60" />
            <span className="block h-2.5 w-2.5 rounded-full bg-yellow-500/60" />
            <span className="block h-2.5 w-2.5 rounded-full bg-green-500/60" />
          </div>
          <span className="text-fd-muted-foreground ml-2 font-mono text-[10px]">~/project — zsh</span>
        </div>
        <div
          ref={scrollRef}
          className="flex h-[184px] flex-col justify-end overflow-hidden p-4 font-mono text-[11px] leading-relaxed"
        >
          {lines.map((line, i) => (
            <div
              key={i}
              className={line.kind === 'cmd' ? 'text-fd-foreground' : 'text-fd-muted-foreground'}
            >
              {line.kind === 'cmd' ? (
                <>
                  <span style={{ color: 'oklch(0.78 0.16 195)' }}>$ </span>
                  {line.text}
                </>
              ) : (
                line.text
              )}
            </div>
          ))}
          {!done && (
            <div className="text-fd-foreground">
              <span style={{ color: 'oklch(0.78 0.16 195)' }}>$ </span>
              {typing}
              <span className="terminal-cursor" style={{ color: 'oklch(0.78 0.16 195)' }}>
                ▋
              </span>
            </div>
          )}
        </div>
      </div>

      {/* Ticket board */}
      <div
        className="rounded-xl border p-3 md:col-span-2"
        style={{ borderColor: 'oklch(1 0 0 / 10%)', background: 'oklch(0.1 0.01 240 / 60%)' }}
      >
        <div className="text-fd-muted-foreground mb-2 font-mono text-[9px] tracking-[0.2em] uppercase">
          tickets
        </div>
        <div className="min-h-[88px] space-y-1.5">
          {tickets.length === 0 && (
            <div className="text-fd-muted-foreground/50 py-6 text-center font-mono text-[10px]">
              no issues yet
            </div>
          )}
          {tickets.map((tk) => {
            const m = STATE_META[tk.state]
            return (
              <div
                key={tk.id}
                className="ticket-in flex items-center gap-2 rounded-lg border px-2.5 py-1.5 transition-all duration-500"
                style={{
                  borderColor: m.ring,
                  background: 'oklch(1 0 0 / 3%)',
                  opacity: tk.state === 'closed' ? 0.6 : 1,
                }}
              >
                <span
                  className="block h-2 w-2 shrink-0 rounded-full transition-colors duration-500"
                  style={{ background: m.dot, boxShadow: `0 0 8px ${m.ring}` }}
                />
                <span className="text-fd-muted-foreground shrink-0 font-mono text-[10px]">{tk.id}</span>
                <span
                  className="text-fd-foreground flex-1 truncate text-[11px]"
                  style={{ textDecoration: tk.state === 'closed' ? 'line-through' : 'none' }}
                >
                  {tk.title}
                </span>
                {tk.prio != null && (
                  <span
                    className="shrink-0 rounded px-1 font-mono text-[9px]"
                    style={{ background: 'oklch(0.78 0.14 60 / 18%)', color: 'oklch(0.82 0.14 60)' }}
                  >
                    P{tk.prio}
                  </span>
                )}
                <span
                  className="shrink-0 font-mono text-[9px] transition-colors duration-500"
                  style={{ color: m.dot }}
                >
                  {m.label}
                </span>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

import Link from 'next/link'

import { HeroTerminal } from './hero-terminal'

const learningPaths = [
  {
    title: 'Getting Started',
    description: 'Install clu, init a project, create your first issues, and run the core loop.',
    href: '/docs/getting-started',
    icon: '▶',
    accent: 'oklch(0.78 0.16 195)',
    links: [
      { label: 'Install', href: '/docs/getting-started/install' },
      { label: 'Quickstart', href: '/docs/getting-started/quickstart' },
      { label: 'Status semantics', href: '/docs/getting-started/status' },
    ],
  },
  {
    title: 'Multi-agent',
    description:
      'Declare agents, route work by capability, and use atomic claim + watch for push-style task delivery.',
    href: '/docs/multi-agent',
    icon: '◆',
    accent: 'oklch(0.72 0.18 285)',
    links: [
      { label: 'Declaring agents', href: '/docs/multi-agent#declaring-agents' },
      { label: 'Capability routing', href: '/docs/multi-agent#capability-routing' },
      { label: 'Watching for work', href: '/docs/multi-agent#watching-for-work' },
    ],
  },
  {
    title: 'Workflows',
    description:
      'YAML templates that materialise as issues + deps + labels — with optional human approval gates.',
    href: '/docs/workflows',
    icon: '●',
    accent: 'oklch(0.78 0.14 60)',
    links: [
      { label: 'Template format', href: '/docs/workflows#template-format' },
      { label: 'Checkpoints', href: '/docs/workflows#checkpoints' },
      { label: 'Running a workflow', href: '/docs/workflows#running' },
    ],
  },
]

const categoryCards = [
  {
    title: 'Commands',
    description: 'Every kong subcommand, its flags, and what it does.',
    href: '/docs/commands',
    icon: '⌨',
  },
  {
    title: 'Storage',
    description: 'The .clu/ directory: SQLite schema, config.yaml, templates.',
    href: '/docs/storage',
    icon: '💾',
  },
  {
    title: 'Web UI',
    description: 'The bundled kanban + list + detail TanStack app served by `clu web`.',
    href: '/docs/web',
    icon: '◫',
  },
  {
    title: 'Agent guide',
    description: 'Operational guide for AI agents driving clu from inside a session.',
    href: '/docs/agents',
    icon: '🤖',
  },
  {
    title: 'Design notes',
    description: 'Sticky decisions: SQLite, Bun, Kong, hand-rolled migrations.',
    href: '/docs/design',
    icon: '◇',
  },
  {
    title: 'FAQ',
    description: 'What clu is not, and why.',
    href: '/docs/faq',
    icon: '?',
  },
]

export default function HomePage() {
  return (
    <main className="relative flex min-h-screen flex-col overflow-hidden px-6 py-16 md:py-24">
      {/* Ambient drifting TRON grid */}
      <div className="grid-backdrop pointer-events-none absolute inset-0 -z-10" />
      <div
        className="pointer-events-none absolute inset-0 -z-10"
        style={{
          background:
            'radial-gradient(ellipse 60% 50% at 50% 20%, oklch(0.78 0.16 195 / 10%) 0%, transparent 70%), radial-gradient(ellipse 40% 40% at 75% 70%, oklch(0.72 0.18 285 / 6%) 0%, transparent 70%)',
        }}
      />

      {/* Hero */}
      <div className="animate-fade-in-up mx-auto w-full max-w-5xl">
        <div
          className="relative overflow-hidden rounded-2xl border p-8 md:p-10"
          style={{
            borderColor: 'oklch(1 0 0 / 10%)',
            background: 'linear-gradient(135deg, oklch(1 0 0 / 3%) 0%, transparent 100%)',
          }}
        >
          <div
            className="absolute top-0 left-0 h-28 w-28 rounded-tl-2xl"
            style={{
              borderLeft: '2px solid oklch(0.78 0.16 195 / 25%)',
              borderTop: '2px solid oklch(0.78 0.16 195 / 25%)',
            }}
          />
          <div
            className="absolute right-0 bottom-0 h-28 w-28 rounded-br-2xl"
            style={{
              borderRight: '2px solid oklch(0.72 0.18 285 / 25%)',
              borderBottom: '2px solid oklch(0.72 0.18 285 / 25%)',
            }}
          />

          <div className="relative space-y-5">
            <p
              className="text-xs font-medium tracking-[0.25em] uppercase"
              style={{ color: 'oklch(0.78 0.16 195)' }}
            >
              Documentation
            </p>
            <h1 className="text-4xl font-bold tracking-tight md:text-5xl">clu</h1>
            <p className="text-fd-muted-foreground max-w-xl text-base md:text-lg">
              A SQLite-backed issue tracker for coordinating AI coding agents on a single machine.
              Local-first, single binary, no daemon, no network.
            </p>

            <div className="flex flex-wrap items-center gap-3 pt-2">
              <Link
                href="/docs/getting-started"
                className="text-fd-primary-foreground inline-flex items-center gap-2 rounded-lg px-6 py-2.5 text-sm font-semibold transition-all hover:brightness-110"
                style={{
                  background:
                    'linear-gradient(135deg, oklch(0.78 0.16 195) 0%, oklch(0.62 0.18 210) 100%)',
                  boxShadow: '0 0 24px oklch(0.78 0.16 195 / 25%)',
                }}
              >
                Get started <span aria-hidden>→</span>
              </Link>
              <Link
                href="/docs/commands"
                className="border-fd-border text-fd-foreground hover:bg-fd-accent inline-flex items-center gap-2 rounded-lg border px-6 py-2.5 text-sm font-medium transition-colors"
              >
                Command reference
              </Link>
            </div>
          </div>
        </div>
      </div>

      {/* The core loop — live terminal + reacting tickets */}
      <div className="animate-fade-in-up mx-auto mt-8 w-full max-w-5xl">
        <p
          className="mb-4 text-xs font-medium tracking-[0.25em] uppercase"
          style={{ color: 'oklch(0.78 0.16 195)' }}
        >
          The core loop
        </p>
        <HeroTerminal />
      </div>

      {/* Learning Paths */}
      <div className="mx-auto mt-12 w-full max-w-5xl">
        <p
          className="mb-6 text-xs font-medium tracking-[0.25em] uppercase"
          style={{ color: 'oklch(0.78 0.16 195)' }}
        >
          Start here
        </p>

        <div className="grid gap-4 md:grid-cols-3">
          {learningPaths.map((path, i) => (
            <div
              key={path.title}
              className="animate-fade-in-up group border-fd-border bg-fd-card hover:border-fd-primary/30 relative overflow-hidden rounded-xl border transition-colors"
              style={{ animationDelay: `${i * 80}ms` }}
            >
              <div
                className="h-[2px]"
                style={{ background: `linear-gradient(90deg, ${path.accent}, transparent)` }}
              />
              <div className="p-5">
                <Link href={path.href} className="mb-3 flex items-center gap-2">
                  <span className="text-2xl">{path.icon}</span>
                  <span
                    className="text-fd-foreground group-hover:text-fd-primary text-lg font-semibold transition-colors"
                    style={{
                      fontFamily: 'var(--font-orbitron), sans-serif',
                      letterSpacing: '0.08em',
                    }}
                  >
                    {path.title}
                  </span>
                </Link>
                <p className="text-fd-muted-foreground mb-4 text-sm leading-relaxed">
                  {path.description}
                </p>
                <div
                  className="space-y-1 border-t pt-3"
                  style={{ borderColor: 'oklch(1 0 0 / 5%)' }}
                >
                  {path.links.map((link) => (
                    <Link
                      key={link.href}
                      href={link.href}
                      className="group/link text-fd-muted-foreground hover:bg-fd-accent hover:text-fd-accent-foreground flex items-center justify-between rounded-md px-2 py-1.5 text-sm transition-colors"
                    >
                      <span>{link.label}</span>
                      <span className="-translate-x-2 text-xs opacity-0 transition-all group-hover/link:translate-x-0 group-hover/link:opacity-100">
                        →
                      </span>
                    </Link>
                  ))}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Categories */}
      <div className="mx-auto mt-16 w-full max-w-5xl">
        <p
          className="mb-6 text-xs font-medium tracking-[0.25em] uppercase"
          style={{ color: 'oklch(0.72 0.18 285)' }}
        >
          Browse
        </p>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {categoryCards.map((card, i) => (
            <Link
              key={card.title}
              href={card.href}
              className="animate-fade-in-up group border-fd-border bg-fd-card hover:border-fd-primary/30 block rounded-xl border p-5 transition-colors"
              style={{ animationDelay: `${(i + 3) * 60}ms` }}
            >
              <div className="flex items-start gap-3">
                <span className="text-xl">{card.icon}</span>
                <div>
                  <h3 className="text-fd-foreground group-hover:text-fd-primary text-base font-semibold transition-colors">
                    {card.title}
                  </h3>
                  <p className="text-fd-muted-foreground mt-0.5 text-xs">{card.description}</p>
                </div>
              </div>
            </Link>
          ))}
        </div>
      </div>

      {/* CTA */}
      <div className="mx-auto mt-16 w-full max-w-5xl">
        <div
          className="animate-fade-in-up relative overflow-hidden rounded-2xl"
          style={{ animationDelay: '400ms' }}
        >
          <div
            className="absolute inset-0 rounded-2xl opacity-25"
            style={{
              background:
                'linear-gradient(90deg, oklch(0.78 0.16 195), oklch(0.72 0.18 285), oklch(0.78 0.16 195))',
            }}
          />
          <div className="bg-fd-background absolute inset-[1px] rounded-2xl" />

          <div className="relative flex flex-col items-center justify-between gap-6 p-8 md:flex-row md:p-10">
            <div className="space-y-1">
              <h3 className="text-fd-foreground text-xl font-bold">Built for many small agents.</h3>
              <p className="text-fd-muted-foreground max-w-md text-sm">
                Pick up work without stepping on each other. Gate risky steps behind human
                approval. Surface what&apos;s unblocked vs. waiting.
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-3">
              <Link
                href="/docs/getting-started/quickstart"
                className="text-fd-primary-foreground inline-flex items-center gap-2 rounded-lg px-6 py-2.5 text-sm font-semibold transition-all hover:brightness-110"
                style={{
                  background:
                    'linear-gradient(135deg, oklch(0.78 0.16 195) 0%, oklch(0.62 0.18 210) 100%)',
                  boxShadow: '0 0 20px oklch(0.78 0.16 195 / 20%)',
                }}
              >
                Quickstart
              </Link>
              <a
                href="https://github.com/arjia-labs/clu"
                target="_blank"
                rel="noopener noreferrer"
                className="border-fd-border text-fd-foreground hover:bg-fd-accent inline-flex items-center gap-2 rounded-lg border px-6 py-2.5 text-sm font-medium transition-colors"
              >
                GitHub
              </a>
            </div>
          </div>
        </div>
      </div>
    </main>
  )
}

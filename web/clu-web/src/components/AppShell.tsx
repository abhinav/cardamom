import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Columns3, List, ShieldCheck, Sparkles, Zap } from 'lucide-react'
import IdentityPicker from './IdentityPicker'
import ThemeToggle from './ThemeToggle'
import { Badge } from './ui/badge'
import { api, type PendingCheckpoint } from '../lib/api'
import { useChangeStream } from '../lib/use-change-stream'

// AppShell is the global frame: collapsible-feeling left sidebar with
// brand + nav + identity, and a full-bleed main area that owns its
// own page header. No 1080px cap — the kanban needs the width.
//
// useChangeStream is mounted here (one EventSource for the whole
// session) so any write — including `clu` CLI invocations in another
// terminal — refetches issue queries automatically.
export default function AppShell({ children }: { children: React.ReactNode }) {
  useChangeStream()
  // Pending-approval count drives the sidebar badge. Cheap query;
  // SSE invalidates ['checkpoints'] alongside ['issues'] indirectly
  // (every checkpoint write touches an issue row).
  const { data: pending = [] } = useQuery<PendingCheckpoint[]>({
    queryKey: ['checkpoints'],
    queryFn: () => api.get('/api/checkpoints'),
  })
  return (
    <div className="flex h-screen w-screen overflow-hidden bg-background text-foreground">
      <aside className="bg-sidebar text-sidebar-foreground flex w-60 shrink-0 flex-col border-r border-sidebar-border">
        <div className="flex items-center gap-2.5 px-4 py-4">
          <div className="bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-md shadow-sm">
            <Sparkles className="size-4" />
          </div>
          <div className="leading-tight">
            <div className="text-sm font-semibold tracking-tight">clu</div>
            <div className="text-muted-foreground text-[10px] uppercase tracking-wider">
              issue tracker
            </div>
          </div>
        </div>

        <nav className="flex flex-col gap-0.5 px-3 pt-2">
          <NavLink to="/" icon={<Columns3 className="size-4" />} exact>
            Board
          </NavLink>
          <NavLink to="/ready" icon={<Zap className="size-4" />}>
            Ready
          </NavLink>
          <NavLink
            to="/approvals"
            icon={<ShieldCheck className="size-4" />}
            trailing={
              pending.length > 0 ? (
                <Badge variant="warning" className="font-mono">
                  {pending.length}
                </Badge>
              ) : null
            }
          >
            Approvals
          </NavLink>
          <NavLink to="/list" icon={<List className="size-4" />}>
            List
          </NavLink>
        </nav>

        <div className="mt-auto flex items-center gap-2 border-t border-sidebar-border p-3">
          <div className="flex-1 min-w-0">
            <IdentityPicker />
          </div>
          <ThemeToggle />
        </div>
      </aside>

      <main className="flex-1 overflow-hidden">{children}</main>
    </div>
  )
}

function NavLink({
  to,
  icon,
  exact,
  trailing,
  children,
}: {
  to: string
  icon: React.ReactNode
  exact?: boolean
  trailing?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <Link
      to={to}
      activeOptions={exact ? { exact: true } : undefined}
      className="text-sidebar-foreground/70 hover:text-sidebar-foreground hover:bg-sidebar-accent data-[status=active]:bg-sidebar-accent data-[status=active]:text-sidebar-foreground inline-flex items-center gap-2 rounded-md px-2.5 py-1.5 text-sm font-medium no-underline"
    >
      {icon}
      <span className="flex-1">{children}</span>
      {trailing}
    </Link>
  )
}

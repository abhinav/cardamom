import { Link } from '@tanstack/react-router'
import { Columns3, List, Sparkles } from 'lucide-react'
import IdentityPicker from './IdentityPicker'
import ThemeToggle from './ThemeToggle'

// AppShell is the global frame: collapsible-feeling left sidebar with
// brand + nav + identity, and a full-bleed main area that owns its
// own page header. No 1080px cap — the kanban needs the width.
export default function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-screen w-screen overflow-hidden bg-background text-foreground">
      <aside className="bg-sidebar text-sidebar-foreground flex w-56 shrink-0 flex-col border-r border-sidebar-border">
        <div className="flex items-center gap-2 px-4 pt-5 pb-4">
          <div className="bg-primary text-primary-foreground flex size-7 items-center justify-center rounded-md">
            <Sparkles className="size-4" />
          </div>
          <div className="leading-tight">
            <div className="text-sm font-semibold tracking-tight">clu</div>
            <div className="text-muted-foreground text-[10px] uppercase tracking-wider">
              issue tracker
            </div>
          </div>
        </div>

        <nav className="flex flex-col gap-0.5 px-2">
          <NavLink to="/" icon={<Columns3 className="size-4" />} exact>
            Board
          </NavLink>
          <NavLink to="/list" icon={<List className="size-4" />}>
            List
          </NavLink>
        </nav>

        <div className="mt-auto flex flex-col gap-2 border-t border-sidebar-border p-3">
          <IdentityPicker />
          <div className="flex justify-end">
            <ThemeToggle />
          </div>
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
  children,
}: {
  to: string
  icon: React.ReactNode
  exact?: boolean
  children: React.ReactNode
}) {
  return (
    <Link
      to={to}
      activeOptions={exact ? { exact: true } : undefined}
      className="text-sidebar-foreground/70 hover:text-sidebar-foreground hover:bg-sidebar-accent data-[status=active]:bg-sidebar-accent data-[status=active]:text-sidebar-foreground inline-flex items-center gap-2 rounded-md px-2.5 py-1.5 text-sm font-medium no-underline"
    >
      {icon}
      {children}
    </Link>
  )
}

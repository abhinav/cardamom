import { Link } from '@tanstack/react-router'
import IdentityPicker from './IdentityPicker'
import ThemeToggle from './ThemeToggle'

// Header is the persistent top bar: logo + primary nav + identity
// picker + theme toggle. Identity picker is the most important
// control after navigation — every mutation flows through it.
export default function Header() {
  return (
    <header className="sticky top-0 z-50 border-b border-[var(--line)] bg-[var(--header-bg)] backdrop-blur-lg">
      <nav className="page-wrap flex items-center gap-3 py-3">
        <Link
          to="/"
          className="inline-flex items-center gap-2 rounded-full border border-[var(--chip-line)] bg-[var(--chip-bg)] px-3 py-1.5 text-sm font-semibold no-underline text-[var(--sea-ink)]"
        >
          <span className="h-2 w-2 rounded-full bg-[linear-gradient(90deg,#56c6be,#7ed3bf)]" />
          clu
        </Link>

        <div className="flex items-center gap-4 text-sm font-semibold">
          <Link
            to="/"
            className="nav-link"
            activeProps={{ className: 'nav-link is-active' }}
            activeOptions={{ exact: true }}
          >
            Board
          </Link>
          <Link
            to="/list"
            className="nav-link"
            activeProps={{ className: 'nav-link is-active' }}
          >
            List
          </Link>
        </div>

        <div className="ml-auto flex items-center gap-2">
          <IdentityPicker />
          <ThemeToggle />
        </div>
      </nav>
    </header>
  )
}

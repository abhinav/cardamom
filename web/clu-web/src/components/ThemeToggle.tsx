import { useEffect, useState } from 'react'
import { Moon, Sun } from 'lucide-react'
import { Button } from './ui/button'

type Mode = 'light' | 'dark'

function readMode(): Mode {
  if (typeof window === 'undefined') return 'dark'
  const stored = window.localStorage.getItem('theme')
  return stored === 'light' ? 'light' : 'dark'
}

function applyMode(mode: Mode) {
  const root = document.documentElement
  root.classList.remove('light', 'dark')
  root.classList.add(mode)
  root.style.colorScheme = mode
}

// Tiny binary toggle: dark by default (set by the pre-paint script in
// __root.tsx), click flips to light. Drops the legacy "auto" mode —
// the user picks a side, the choice persists.
export default function ThemeToggle() {
  const [mode, setMode] = useState<Mode>('dark')

  useEffect(() => {
    setMode(readMode())
  }, [])

  function flip() {
    const next: Mode = mode === 'dark' ? 'light' : 'dark'
    setMode(next)
    applyMode(next)
    window.localStorage.setItem('theme', next)
  }

  return (
    <Button
      variant="ghost"
      size="icon-sm"
      onClick={flip}
      aria-label={`switch to ${mode === 'dark' ? 'light' : 'dark'} mode`}
      title={`switch to ${mode === 'dark' ? 'light' : 'dark'} mode`}
    >
      {mode === 'dark' ? <Sun /> : <Moon />}
    </Button>
  )
}

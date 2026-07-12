// Identity is whatever name the user typed into the picker; stored in
// localStorage so it persists across tabs/reloads. The X-Clu-Agent
// header on every API call is derived from this value (api.ts wires
// that up).
//
// SSR-safe: localStorage is only touched inside the typeof check, so
// the first render before hydration sees an empty identity. The picker
// re-reads on mount via a useSyncExternalStore subscriber.

const KEY = 'clu.agent'

type Listener = () => void
const listeners = new Set<Listener>()

export function getAgent(): string {
  if (typeof window === 'undefined') return ''
  try {
    return window.localStorage.getItem(KEY) ?? ''
  } catch {
    return ''
  }
}

export function setAgent(name: string): void {
  if (typeof window === 'undefined') return
  try {
    if (name === '') window.localStorage.removeItem(KEY)
    else window.localStorage.setItem(KEY, name)
  } catch {
    // localStorage unavailable (private mode etc); pretend to set
  }
  listeners.forEach((fn) => fn())
}

export function subscribe(fn: Listener): () => void {
  listeners.add(fn)
  // Also listen to storage events so other tabs sync.
  const onStorage = (e: StorageEvent) => {
    if (e.key === KEY) fn()
  }
  if (typeof window !== 'undefined') {
    window.addEventListener('storage', onStorage)
  }
  return () => {
    listeners.delete(fn)
    if (typeof window !== 'undefined') {
      window.removeEventListener('storage', onStorage)
    }
  }
}

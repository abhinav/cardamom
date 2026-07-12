import { useEffect, useSyncExternalStore } from 'react'
import { useQueryClient } from '@tanstack/react-query'

// useChangeStream subscribes to the server's SSE change stream so any
// write — from this client, another tab, or a `clu` CLI in another
// terminal — triggers a refetch of issue-related queries.
//
// Mount once high in the tree (AppShell). The hook owns one global
// EventSource (process-level singleton) so multiple components can
// observe the same connection status via useLiveStatus without
// fighting over reconnect logic.

const API_BASE =
  (import.meta.env.VITE_CLU_API_URL as string | undefined) ??
  'http://127.0.0.1:7777'

// LiveStatus describes the SSE connection state. "connecting" is the
// initial state and the state while the browser is retrying; "live"
// after we've received the synthetic "ready" event from the server.
export type LiveStatus = 'connecting' | 'live' | 'offline'

// Module-level singleton state. We open one EventSource for the
// whole app — multiple `useChangeStream`/`useLiveStatus` calls share
// it via the listener set.
let es: EventSource | null = null
let status: LiveStatus = 'connecting'
const listeners = new Set<() => void>()

function setStatus(next: LiveStatus) {
  if (status === next) return
  status = next
  listeners.forEach((fn) => fn())
}

function ensureOpen(onChange: () => void) {
  if (es) return
  if (typeof window === 'undefined' || typeof EventSource === 'undefined') {
    return
  }
  setStatus('connecting')
  es = new EventSource(`${API_BASE}/api/events`)
  es.addEventListener('ready', () => setStatus('live'))
  es.addEventListener('issues-changed', () => onChange())
  // EventSource reconnects automatically. onerror fires while it's
  // retrying; flip to offline so the indicator reflects reality.
  es.onerror = () => setStatus('offline')
  // When the connection re-establishes we'll get another "ready".
  es.onopen = () => {
    // onopen fires before "ready" — keep "connecting" until the
    // server-side event confirms the stream is wired through any
    // intermediate proxy.
    if (status === 'offline') setStatus('connecting')
  }
}

export function useChangeStream() {
  const qc = useQueryClient()
  useEffect(() => {
    ensureOpen(() => {
      // Every write that touches an issue row bumps MAX(updated)
      // → SSE fires. Invalidate everything that derives from the
      // issues table: list/board/ready/detail (`issues`, `issue`)
      // and pending-checkpoints (`checkpoints`, drives the
      // approvals page + the sidebar badge). Cheap — TanStack
      // Query only refetches queries with active observers.
      qc.invalidateQueries({ queryKey: ['issues'] })
      qc.invalidateQueries({ queryKey: ['issue'] })
      qc.invalidateQueries({ queryKey: ['checkpoints'] })
    })
    // Intentionally no cleanup — the EventSource is a singleton kept
    // open for the lifetime of the app. Closing it on unmount would
    // break other components that also subscribed.
  }, [qc])
}

// useLiveStatus reads the current SSE connection state. Re-renders
// when it changes. Cheap; backed by useSyncExternalStore.
export function useLiveStatus(): LiveStatus {
  return useSyncExternalStore(
    (fn) => {
      listeners.add(fn)
      return () => listeners.delete(fn)
    },
    () => status,
    () => 'connecting',
  )
}

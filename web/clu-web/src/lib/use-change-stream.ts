import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'

// useChangeStream subscribes to the server's SSE change stream so any
// write — from this client, another tab, or a `clu` CLI in another
// terminal — triggers a refetch of issue-related queries.
//
// Mount once high in the tree (AppShell). EventSource reconnects
// automatically on transient network errors; we don't manage state
// beyond opening/closing.
//
// SSR-safe: the effect only runs after hydration so `EventSource`
// access is gated on the window.

const API_BASE =
  (import.meta.env.VITE_CLU_API_URL as string | undefined) ??
  'http://127.0.0.1:7777'

export function useChangeStream() {
  const qc = useQueryClient()
  useEffect(() => {
    if (typeof window === 'undefined' || typeof EventSource === 'undefined') {
      return
    }
    const es = new EventSource(`${API_BASE}/api/events`)
    const onChange = () => {
      // Single coarse event triggers two invalidations. TanStack
      // Query matches by key prefix, so ['issue'] covers every
      // ['issue', '<id>'] detail query.
      qc.invalidateQueries({ queryKey: ['issues'] })
      qc.invalidateQueries({ queryKey: ['issue'] })
    }
    es.addEventListener('issues-changed', onChange)
    // 'ready' isn't actionable today; reserved for status indicators.
    return () => {
      es.removeEventListener('issues-changed', onChange)
      es.close()
    }
  }, [qc])
}

import { useSyncExternalStore } from 'react'
import { getAgent, setAgent, subscribe } from './identity'

// useIdentity returns [agent, setAgent]. Re-renders on changes from
// any tab (storage event) or from this tab's own setAgent calls.
// SSR returns empty string so the first paint is consistent.
export function useIdentity(): [string, (name: string) => void] {
  const agent = useSyncExternalStore(
    subscribe,
    getAgent,
    () => '',
  )
  return [agent, setAgent]
}

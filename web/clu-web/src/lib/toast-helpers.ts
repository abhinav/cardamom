import { toast } from 'sonner'
import { ApiError } from './api'

// formatApiError turns an ApiError (or anything else) into a short
// user-readable string. Falls back to the message untouched.
export function formatApiError(err: unknown): string {
  if (err instanceof ApiError) {
    return err.message || `error ${err.status}`
  }
  if (err instanceof Error) return err.message
  return String(err)
}

// notifyError is the canonical "show this mutation failed" call.
// Always logs to console too so devs see stack traces even when
// the toast description is brief.
export function notifyError(headline: string, err: unknown) {
  // eslint-disable-next-line no-console
  console.error(headline, err)
  toast.error(headline, { description: formatApiError(err) })
}

// notifyOk is a small wrapper around toast.success that keeps the
// call sites short. Description optional; common case is just the
// confirmation.
export function notifyOk(headline: string, description?: string) {
  toast.success(headline, description ? { description } : undefined)
}

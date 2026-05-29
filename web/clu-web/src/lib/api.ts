// Thin fetch wrapper around the `clu http` REST API.
//
// All requests pass through here so the X-Clu-Agent header (the
// caller's identity) is attached uniformly. Base URL is read from
// VITE_CLU_API_URL with a localhost fallback; in dev the Vite proxy
// can also rewrite /api/* to the Go server.

import { getAgent } from './identity'

const API_BASE =
  (import.meta.env.VITE_CLU_API_URL as string | undefined) ??
  'http://127.0.0.1:7777'

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {}
  const agent = getAgent()
  if (agent) headers['X-Clu-Agent'] = agent
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    let msg = res.statusText
    try {
      const err = (await res.json()) as { error?: string }
      if (err.error) msg = err.error
    } catch {
      // body wasn't JSON; keep statusText
    }
    throw new ApiError(res.status, msg)
  }
  // 204 No Content
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  delete: <T>(path: string) => request<T>('DELETE', path),
}

// ---- API surface mirrors internal/http handlers ----

export interface Issue {
  id: string
  title: string
  type: string
  status: 'open' | 'in_progress' | 'closed' | 'cancelled'
  priority: number
  assignee?: string | null
  created: number
  updated: number
  started_at?: number | null
  closed?: number | null
  defer_until?: number | null
  description?: string | null
  notes?: string | null
  labels: string[]
  blocked: boolean
}

export interface IssueDetail extends Issue {
  depends_on: string[]
  blocks: string[]
  comments: Comment[]
}

export interface Comment {
  id: number
  issue_id: string
  author: string
  body: string
  created: number
}

export interface Meta {
  statuses: string[]
  types: string[]
  id_prefix: string
  schema_version: number
}

// PendingCheckpoint mirrors internal/http's pendingCheckpointOut.
// Embeds the Issue fields directly (the Go side flattens via
// embedded struct).
export interface PendingCheckpoint extends Issue {
  kind: 'approval' | 'manual'
  approvers?: string[]
  blocks: string[]
}

// ---- Workflow templates ----

export interface TemplateVar {
  name: string
  label?: string
  default?: string
  required?: boolean
  pattern?: string
}

export interface TemplateSummary {
  name: string
  description?: string
  vars: TemplateVar[]
  step_count: number
}

export interface TemplateLoadError {
  file: string
  error: string
}

export interface TemplatesList {
  templates: TemplateSummary[]
  errors?: TemplateLoadError[]
}

export interface PlanStep {
  id: string
  title: string
  description?: string
  type: string
  priority: number
  needs?: string[]
  agent?: string
  wait?: { manual?: boolean; approval?: string[] } | null
  is_leaf: boolean
}

export interface Plan {
  template: string
  title: string
  vars: Record<string, string>
  steps: PlanStep[]
}

export interface RunResult {
  parent_id: string
  plan: Plan
}

export interface ActiveAgent {
  name: string
  pid: number
  host: string
  capabilities: string
  started_at: number
  last_seen: number
}

export interface IssueFilters {
  status?: string[]
  agent?: string
  type?: string
  tag?: string[]
  q?: string
  sort?: string
  reverse?: boolean
  limit?: number
}

export function filtersToQuery(f: IssueFilters): string {
  const p = new URLSearchParams()
  f.status?.forEach((s) => p.append('status', s))
  f.tag?.forEach((t) => p.append('tag', t))
  if (f.agent) p.set('agent', f.agent)
  if (f.type) p.set('type', f.type)
  if (f.q) p.set('q', f.q)
  if (f.sort) p.set('sort', f.sort)
  if (f.reverse) p.set('reverse', '1')
  if (f.limit) p.set('limit', String(f.limit))
  const s = p.toString()
  return s ? '?' + s : ''
}

// Mutation request shapes match the Go side's JSON bodies.
export interface PatchIssueBody {
  title?: string
  type?: string
  status?: string
  priority?: number
  assignee?: string | null
  description?: string | null
  tags?: string[]
}

export interface CreateIssueBody {
  title: string
  type?: string
  priority: number
  agent?: string | null
  labels?: string[]
  parents?: string[]
  description?: string
  notes?: string
}

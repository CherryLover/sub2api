/**
 * Public Key Usage API (no authentication cookie / bearer session required).
 *
 * These endpoints back the public `/key-usage` page:
 *   POST /api/v1/key-usage/session  -> exchange a raw API key for a short lived,
 *                                      non-reversible lookup token (safe to put in the URL)
 *   GET  /api/v1/key-usage/report   -> usage payload + per-window stats + rankings
 *
 * Implemented with the raw `fetch` API on purpose: the shared axios client attaches
 * dashboard credentials and force-logs-out on 401, which must never happen on a
 * public page where a 401 simply means "this lookup token expired".
 */

import { buildApiUrl } from './url'

// ==================== Types ====================

export type KeyUsageMetric = 'cost' | 'tokens' | 'requests'
export type KeyUsageWindowKey = 'today' | 'last_7d' | 'last_30d'
export type KeyUsageRankingScope = 'account' | 'site'

export const KEY_USAGE_METRICS: KeyUsageMetric[] = ['cost', 'tokens', 'requests']
export const KEY_USAGE_WINDOWS: KeyUsageWindowKey[] = ['today', 'last_7d', 'last_30d']
export const KEY_USAGE_SCOPES: KeyUsageRankingScope[] = ['account', 'site']

export interface KeyUsageModelStat {
  model: string
  requests: number
  tokens: number
  cost_usd: number
}

export interface KeyUsageWindowStat {
  requests: number
  tokens: number
  cost_usd: number
  models: KeyUsageModelStat[]
}

export interface KeyUsageRankingEntry {
  rank: number
  key_name: string
  requests: number
  tokens: number
  cost_usd: number
  is_self: boolean
}

export interface KeyUsageRankingWindow {
  total_keys: number
  self_rank: number
  top: KeyUsageRankingEntry[]
  self: KeyUsageRankingEntry | null
}

export type KeyUsageRankingGroup = Record<KeyUsageWindowKey, KeyUsageRankingWindow>

export interface KeyUsageKeyInfo {
  name: string
  created_at: string | null
  status: string
}

export interface KeyUsageReport {
  key: KeyUsageKeyInfo | null
  // The raw `/v1/usage` payload, embedded verbatim.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  usage: any
  windows: Partial<Record<KeyUsageWindowKey, KeyUsageWindowStat>> | null
  rankings: Partial<Record<KeyUsageRankingScope, Partial<KeyUsageRankingGroup>>> | null
  metric: KeyUsageMetric
  generated_at: string | null
}

export interface KeyUsageSession {
  token: string
  expires_at: string | null
}

// ==================== Errors ====================

export class KeyUsageRequestError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'KeyUsageRequestError'
    this.status = status
  }
}

export function isUnauthorized(err: unknown): boolean {
  return err instanceof KeyUsageRequestError && err.status === 401
}

/** True when the endpoint itself is not deployed yet (backend rollout in progress). */
export function isEndpointMissing(err: unknown): boolean {
  return err instanceof KeyUsageRequestError && (err.status === 404 || err.status === 405 || err.status === 501)
}

// ==================== Helpers ====================

async function readError(res: Response, fallback: string): Promise<KeyUsageRequestError> {
  const body = await res.json().catch(() => null)
  const message = body?.error?.message || body?.message || `${fallback} (${res.status})`
  return new KeyUsageRequestError(message, res.status)
}

export function isValidMetric(value: unknown): value is KeyUsageMetric {
  return typeof value === 'string' && (KEY_USAGE_METRICS as string[]).includes(value)
}

// ==================== Endpoints ====================

/**
 * Exchange a raw API key for a lookup token.
 * The token is what ends up in the shareable `?t=` URL — never the raw key.
 */
export async function createKeyUsageSession(key: string, fallbackMessage: string): Promise<KeyUsageSession> {
  const res = await fetch(buildApiUrl('/key-usage/session'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ key }),
  })
  if (!res.ok) {
    throw await readError(res, fallbackMessage)
  }
  const body = await res.json()
  const token = typeof body?.token === 'string' ? body.token : ''
  if (!token) {
    throw new KeyUsageRequestError(fallbackMessage, res.status)
  }
  return { token, expires_at: body?.expires_at ?? null }
}

export interface FetchReportOptions {
  /** Lookup token issued by `createKeyUsageSession`. */
  token?: string
  /** Raw API key, sent as `Authorization: Bearer` when no token is available. */
  key?: string
  metric: KeyUsageMetric
  /** Extra query params (date range / timezone) forwarded to the report endpoint. */
  extraParams?: URLSearchParams | null
  fallbackMessage: string
}

export async function fetchKeyUsageReport(options: FetchReportOptions): Promise<KeyUsageReport> {
  const params = new URLSearchParams()
  if (options.token) {
    params.set('token', options.token)
  }
  params.set('metric', options.metric)
  if (options.extraParams) {
    options.extraParams.forEach((value, name) => {
      if (name !== 'token' && name !== 'metric') {
        params.set(name, value)
      }
    })
  }

  const headers: Record<string, string> = {}
  if (!options.token && options.key) {
    headers['Authorization'] = 'Bearer ' + options.key
  }

  const res = await fetch(`${buildApiUrl('/key-usage/report')}?${params.toString()}`, { headers })
  if (!res.ok) {
    throw await readError(res, options.fallbackMessage)
  }
  return (await res.json()) as KeyUsageReport
}

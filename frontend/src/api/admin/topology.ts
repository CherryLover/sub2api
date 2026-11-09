/**
 * 调度拓扑 API：快照（普通 JSON，走 apiClient 解包）+ 实时路由事件（SSE）。
 *
 * SSE 用 fetch + ReadableStream 读取，因为 EventSource 带不了 Authorization 头。
 * 事件字段与 backend/internal/service/routing_observer.go 的 RoutingEvent 同名。
 */

import { apiClient } from '../client'
import { ADMIN_UI_REQUEST_HEADER } from '../adminUIRequest'
import { refreshAuthTokens } from '../tokenRefresh'
import { buildApiUrl } from '../url'
import { getLocale } from '@/i18n'

// ==================== Snapshot ====================

export interface TopologyTotals {
  in_flight: number
  queued: number
  requests_5m: number
  failed_5m: number
  retries_same_5m: number
  retries_switch_5m: number
  failed_after_retry_5m: number
  accounts_available: number
  accounts_total: number
}

export interface TopologyKey {
  id: number
  name: string
  group_id: number | null
  group_name: string
  in_flight: number
  queued: number
  requests_5m: number
  failed_5m: number
}

export interface TopologyGroup {
  id: number
  name: string
  platform: string
  strategy: string
  in_flight: number
  requests_5m: number
  failed_5m: number
  account_ids: number[]
}

export interface TopologyQuotaWindow {
  used_percent: number
  reset_at: string | null
}

export type TopologyAccountStatus = 'ok' | 'rate_limited' | 'cooldown' | 'error' | 'disabled'

export interface TopologyAccount {
  id: number
  name: string
  platform: string
  status: TopologyAccountStatus
  status_text: string
  recover_at: string | null
  concurrency_current: number
  concurrency_max: number
  quota_5h: TopologyQuotaWindow | null
  quota_7d: TopologyQuotaWindow | null
  billing: 'subscription' | 'pay_as_you_go'
}

export interface TopologySnapshot {
  generated_at: string
  totals: TopologyTotals
  gate_blocks_5m: Record<string, number>
  keys: TopologyKey[]
  groups: TopologyGroup[]
  accounts: TopologyAccount[]
}

export async function getTopologySnapshot(signal?: AbortSignal): Promise<TopologySnapshot> {
  const { data } = await apiClient.get<TopologySnapshot>('/admin/topology/snapshot', { signal })
  return data
}

// ==================== Routing events ====================

export type RoutingEventType = 'request_start' | 'blocked' | 'attempt' | 'attempt_failed' | 'request_done'
export type RoutingNext = 'same_account' | 'switch' | 'give_up'

export interface RoutingEvent {
  type: RoutingEventType
  request_id: string
  api_key_id?: number
  group_id?: number
  account_id?: number
  attempt?: number
  status_code?: number
  next?: RoutingNext
  reason?: string
  cooldown_until?: string
  at: string
}

export interface SSEMessage {
  event: string
  data: string
  id?: string
}

/**
 * Incremental text/event-stream parser (WHATWG rules): handles chunks split
 * mid-line, \n / \r\n / \r line endings (including a \r\n split across chunks),
 * comment lines (heartbeats), and multi-line data fields.
 */
export class SSEParser {
  private buffer = ''
  private pendingCR = false
  private eventName = ''
  private dataLines: string[] = []
  private hasData = false
  private lastId: string | undefined

  push(chunk: string): SSEMessage[] {
    const out: SSEMessage[] = []
    let text = chunk
    if (this.pendingCR) {
      this.pendingCR = false
      if (text.startsWith('\n')) text = text.slice(1)
    }
    this.buffer += text
    let start = 0
    for (let i = 0; i < this.buffer.length; i++) {
      const ch = this.buffer[i]
      if (ch !== '\n' && ch !== '\r') continue
      const line = this.buffer.slice(start, i)
      if (ch === '\r') {
        if (i + 1 < this.buffer.length) {
          if (this.buffer[i + 1] === '\n') i++
        } else {
          this.pendingCR = true
        }
      }
      start = i + 1
      const msg = this.processLine(line)
      if (msg) out.push(msg)
    }
    this.buffer = this.buffer.slice(start)
    return out
  }

  /** Dispatches a trailing event that was not terminated by a blank line (stream end). */
  flush(): SSEMessage[] {
    const out: SSEMessage[] = []
    if (this.buffer) {
      const msg = this.processLine(this.buffer)
      this.buffer = ''
      if (msg) out.push(msg)
    }
    const last = this.processLine('')
    if (last) out.push(last)
    return out
  }

  private processLine(line: string): SSEMessage | null {
    if (line === '') {
      if (!this.hasData) {
        this.eventName = ''
        return null
      }
      const msg: SSEMessage = { event: this.eventName || 'message', data: this.dataLines.join('\n') }
      if (this.lastId !== undefined) msg.id = this.lastId
      this.eventName = ''
      this.dataLines = []
      this.hasData = false
      return msg
    }
    if (line.startsWith(':')) return null // comment / heartbeat
    const colon = line.indexOf(':')
    let field = line
    let value = ''
    if (colon >= 0) {
      field = line.slice(0, colon)
      value = line.slice(colon + 1)
      if (value.startsWith(' ')) value = value.slice(1)
    }
    switch (field) {
      case 'event':
        this.eventName = value
        break
      case 'data':
        this.dataLines.push(value)
        this.hasData = true
        break
      case 'id':
        this.lastId = value
        break
      default:
        break
    }
    return null
  }
}

/** Parses one SSE message into a RoutingEvent; returns null for other events or bad JSON. */
export function parseRoutingMessage(msg: SSEMessage): RoutingEvent | null {
  if (msg.event !== 'routing') return null
  try {
    const parsed = JSON.parse(msg.data) as RoutingEvent
    if (!parsed || typeof parsed !== 'object' || typeof parsed.type !== 'string') return null
    return parsed
  } catch {
    return null
  }
}

export type TopologyStreamStatus = 'connecting' | 'open' | 'reconnecting' | 'closed'

export interface TopologyStreamOptions {
  onEvent: (ev: RoutingEvent) => void
  onStatus?: (status: TopologyStreamStatus) => void
  /** Injected for tests. */
  fetchImpl?: typeof fetch
  minBackoffMs?: number
  maxBackoffMs?: number
}

export interface TopologyStreamHandle {
  close: () => void
}

/** Backoff for the n-th consecutive failure (0-based): min * 2^n capped at max. */
export function backoffDelay(failures: number, minMs = 1000, maxMs = 30000): number {
  return Math.min(maxMs, minMs * Math.pow(2, Math.max(0, failures)))
}

/**
 * Opens the routing event stream and keeps it open: reconnects with exponential
 * backoff after errors or server-side close, refreshes the access token once on
 * 401, and stops for good when close() is called.
 */
export function openTopologyEventStream(opts: TopologyStreamOptions): TopologyStreamHandle {
  const fetchImpl = opts.fetchImpl ?? ((...args: Parameters<typeof fetch>) => fetch(...args))
  const minMs = opts.minBackoffMs ?? 1000
  const maxMs = opts.maxBackoffMs ?? 30000
  let closed = false
  let controller: AbortController | null = null
  let timer: ReturnType<typeof setTimeout> | null = null
  let failures = 0
  let refreshedOnce = false

  const setStatus = (s: TopologyStreamStatus) => opts.onStatus?.(s)

  const scheduleReconnect = () => {
    if (closed) return
    setStatus('reconnecting')
    const delay = backoffDelay(failures, minMs, maxMs)
    failures++
    timer = setTimeout(() => {
      timer = null
      void connect()
    }, delay)
  }

  const connect = async () => {
    if (closed) return
    controller = new AbortController()
    if (failures === 0) setStatus('connecting')
    try {
      const token = localStorage.getItem('auth_token')
      const headers: Record<string, string> = {
        Accept: 'text/event-stream',
        'Accept-Language': getLocale(),
        [ADMIN_UI_REQUEST_HEADER]: '1'
      }
      if (token) headers.Authorization = `Bearer ${token}`
      const res = await fetchImpl(buildApiUrl('/admin/topology/events'), {
        method: 'GET',
        headers,
        credentials: 'include',
        signal: controller.signal
      })
      if (res.status === 401 && !refreshedOnce && localStorage.getItem('refresh_token')) {
        refreshedOnce = true
        await refreshAuthTokens({ failedAccessToken: token })
        return connect()
      }
      if (!res.ok || !res.body) throw new Error(`topology events: HTTP ${res.status}`)
      refreshedOnce = false
      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      const parser = new SSEParser()
      let opened = false
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        if (!opened) {
          opened = true
          failures = 0
          setStatus('open')
        }
        for (const msg of parser.push(decoder.decode(value, { stream: true }))) {
          const ev = parseRoutingMessage(msg)
          if (ev) opts.onEvent(ev)
        }
      }
      for (const msg of parser.flush()) {
        const ev = parseRoutingMessage(msg)
        if (ev) opts.onEvent(ev)
      }
      if (!opened) failures = Math.max(failures, 1)
      scheduleReconnect()
    } catch (err) {
      if (closed || (err instanceof DOMException && err.name === 'AbortError')) return
      scheduleReconnect()
    }
  }

  void connect()

  return {
    close() {
      if (closed) return
      closed = true
      if (timer) clearTimeout(timer)
      controller?.abort()
      setStatus('closed')
    }
  }
}

export default { getTopologySnapshot, openTopologyEventStream }

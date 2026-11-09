/**
 * 调度拓扑的纯数据整理：密钥折叠排序、额度条格式化、账号状态映射、关卡定义、连线几何。
 * 不依赖 DOM / Vue，便于单测；文案通过传入的 t() 取。
 */

import type {
  TopologyAccount,
  TopologyGroup,
  TopologyKey,
  TopologyQuotaWindow
} from '@/api/admin/topology'

export type Translate = (key: string, params?: Record<string, unknown>) => string

export const I18N = 'admin.topology.'

// ==================== Gates ====================

export const GATE_IDS = [
  'auth',
  'group',
  'sticky',
  'platform',
  'status',
  'quota',
  'model',
  'capacity',
  'dispatch'
] as const
export type GateId = (typeof GATE_IDS)[number]
export const LAST_GATE_INDEX = GATE_IDS.length - 1

/**
 * 拦下原因码 → 关卡。原因码沿用调度器的过滤统计 / 选号失败分类；
 * 不认识的原因落到最后一关（挑号发出），文案直接显示原始码。
 */
const REASON_GATE: Record<string, GateId> = {
  invalid_api_key: 'auth',
  api_key_disabled: 'auth',
  api_key_expired: 'auth',
  insufficient_balance: 'auth',
  group_not_found: 'group',
  no_group: 'group',
  group_disabled: 'group',
  platform_mismatch: 'platform',
  platform_filtered: 'platform',
  channel_restricted: 'platform',
  channel_upstream_restricted: 'platform',
  transport_incompatible: 'platform',
  not_schedulable: 'status',
  unschedulable: 'status',
  runtime_blocked: 'status',
  excluded: 'status',
  shadow_parent_unhealthy: 'status',
  privacy_not_set: 'status',
  ineligible: 'status',
  model_rate_limited: 'quota',
  quota_exhausted: 'quota',
  quota: 'quota',
  quota_unknown: 'quota',
  model_not_supported: 'model',
  model_unsupported: 'model',
  compact_unsupported: 'model',
  capacity: 'capacity',
  concurrency_full: 'capacity',
  queue_full: 'capacity',
  full: 'capacity',
  no_available_accounts: 'dispatch'
}

export const KNOWN_REASONS = Object.keys(REASON_GATE)

export function gateIndexForReason(reason: string | undefined | null): number {
  const gate = reason ? REASON_GATE[reason] : undefined
  return gate ? GATE_IDS.indexOf(gate) : LAST_GATE_INDEX
}

export function reasonLabel(reason: string, t: Translate): string {
  if (!reason) return t(I18N + 'reason.unknown')
  if (reason in REASON_GATE) return t(I18N + 'reason.' + reason)
  return reason
}

export interface GateBlockSummary {
  total: number
  top: { reason: string; count: number } | null
}

export function summarizeGateBlocks(blocks: Record<string, number> | null | undefined): GateBlockSummary {
  let total = 0
  let top: GateBlockSummary['top'] = null
  for (const [reason, raw] of Object.entries(blocks ?? {})) {
    const count = Number(raw) || 0
    if (count <= 0) continue
    total += count
    if (!top || count > top.count || (count === top.count && reason < top.reason)) {
      top = { reason, count }
    }
  }
  return { total, top }
}

// ==================== Keys ====================

export function isKeyActive(k: TopologyKey): boolean {
  return k.in_flight > 0 || k.queued > 0 || k.requests_5m > 0
}

function keyGroupLabel(k: TopologyKey): string {
  return k.group_name || (k.group_id != null ? `#${k.group_id}` : '')
}

function compareKeyActivity(a: TopologyKey, b: TopologyKey): number {
  return (
    b.in_flight - a.in_flight ||
    b.queued - a.queued ||
    b.requests_5m - a.requests_5m ||
    a.name.localeCompare(b.name) ||
    a.id - b.id
  )
}

/**
 * 按分组聚在一起（分组按总活跃度降序），组内按活跃度降序。
 */
export function sortKeys(keys: TopologyKey[]): TopologyKey[] {
  const groupScore = new Map<string, { inFlight: number; queued: number; req: number }>()
  for (const k of keys) {
    const g = keyGroupLabel(k)
    const s = groupScore.get(g) ?? { inFlight: 0, queued: 0, req: 0 }
    s.inFlight += k.in_flight
    s.queued += k.queued
    s.req += k.requests_5m
    groupScore.set(g, s)
  }
  return [...keys].sort((a, b) => {
    const ga = keyGroupLabel(a)
    const gb = keyGroupLabel(b)
    if (ga !== gb) {
      const sa = groupScore.get(ga)!
      const sb = groupScore.get(gb)!
      return (
        sb.inFlight - sa.inFlight ||
        sb.queued - sa.queued ||
        sb.req - sa.req ||
        ga.localeCompare(gb)
      )
    }
    return compareKeyActivity(a, b)
  })
}

export interface FoldedKeys {
  visible: TopologyKey[]
  folded: TopologyKey[]
}

/** 活跃密钥（正在请求 / 排队 / 近 5 分钟有请求）单独显示，其余折叠。 */
export function foldKeys(keys: TopologyKey[]): FoldedKeys {
  const visible: TopologyKey[] = []
  const folded: TopologyKey[] = []
  for (const k of keys) (isKeyActive(k) ? visible : folded).push(k)
  return {
    visible: sortKeys(visible),
    folded: [...folded].sort((a, b) => a.name.localeCompare(b.name) || a.id - b.id)
  }
}

export type KeyLoadKind = 'active' | 'queued' | 'idle'

export function keyLoadKind(k: TopologyKey): KeyLoadKind {
  if (k.in_flight > 0) return 'active'
  if (k.queued > 0) return 'queued'
  return 'idle'
}

// ==================== Quota ====================

export type QuotaTone = 'ok' | 'warn' | 'full'

export interface QuotaBar {
  window: '5h' | '7d'
  percent: number
  tone: QuotaTone
  text: string
}

export type QuotaView =
  | { mode: 'payg' }
  | { mode: 'bars'; bars: QuotaBar[] }

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

export function formatClock(d: Date): string {
  return `${d.getMonth() + 1}/${d.getDate()} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

export function parseTime(value: string | null | undefined): number | null {
  if (!value) return null
  const ms = Date.parse(value)
  return Number.isFinite(ms) ? ms : null
}

/** "3 分钟后" / "2 小时后" / "5 天后"（向上取整，最少 1）。 */
export function formatRelativeFuture(targetMs: number, nowMs: number, t: Translate): string {
  const diff = Math.max(0, targetMs - nowMs)
  const minutes = Math.max(1, Math.ceil(diff / 60000))
  if (minutes < 60) return t(I18N + 'time.minutesLater', { n: minutes })
  const hours = Math.ceil(diff / 3600000)
  if (hours < 24) return t(I18N + 'time.hoursLater', { n: hours })
  return t(I18N + 'time.daysLater', { n: Math.ceil(diff / 86400000) })
}

export function quotaTone(percent: number): QuotaTone {
  if (percent >= 100) return 'full'
  if (percent >= 80) return 'warn'
  return 'ok'
}

export function formatQuotaBar(
  window: '5h' | '7d',
  q: TopologyQuotaWindow,
  nowMs: number,
  t: Translate
): QuotaBar {
  const raw = Number(q.used_percent)
  const percent = Math.max(0, Math.min(100, Number.isFinite(raw) ? Math.round(raw) : 0))
  const tone = quotaTone(percent)
  const resetMs = parseTime(q.reset_at)
  let text: string
  if (tone === 'full') {
    text = resetMs != null
      ? t(I18N + 'quota.exhaustedUntil', { time: formatClock(new Date(resetMs)) })
      : t(I18N + 'quota.exhausted')
  } else if (resetMs != null) {
    text = t(I18N + 'quota.usedReset', { percent, when: formatRelativeFuture(resetMs, nowMs, t) })
  } else {
    text = t(I18N + 'quota.used', { percent })
  }
  return { window, percent, tone, text }
}

/** 按量付费不显示额度条；为 null 的窗口不显示。 */
export function accountQuotaView(a: TopologyAccount, nowMs: number, t: Translate): QuotaView {
  if (a.billing === 'pay_as_you_go') return { mode: 'payg' }
  const bars: QuotaBar[] = []
  if (a.quota_5h) bars.push(formatQuotaBar('5h', a.quota_5h, nowMs, t))
  if (a.quota_7d) bars.push(formatQuotaBar('7d', a.quota_7d, nowMs, t))
  return { mode: 'bars', bars }
}

// ==================== Account status ====================

export type AccountTone = 'ok' | 'warn' | 'error' | 'disabled'

export interface AccountStatusView {
  tone: AccountTone
  /** 可以接新请求 */
  available: boolean
  /** 右上角文字；ok 时为空（显示并发 x/y） */
  label: string
  /** 例如 "11/11 17:36 恢复" */
  recoverText: string
  /** 授权失效：给"去重新授权"链接 */
  reauth: boolean
  title: string
}

export function accountStatusView(a: TopologyAccount, nowMs: number, t: Translate): AccountStatusView {
  const recoverMs = parseTime(a.recover_at)
  const recoverText = recoverMs != null && recoverMs > nowMs
    ? t(I18N + 'account.recoverAt', { time: formatClock(new Date(recoverMs)) })
    : ''
  const title = a.status_text || ''
  switch (a.status) {
    case 'ok':
      return { tone: 'ok', available: true, label: '', recoverText: '', reauth: false, title }
    case 'rate_limited':
      return { tone: 'warn', available: false, label: t(I18N + 'account.rateLimited'), recoverText, reauth: false, title }
    case 'cooldown':
      return { tone: 'warn', available: false, label: t(I18N + 'account.cooldown'), recoverText, reauth: false, title }
    case 'error':
      return { tone: 'error', available: false, label: t(I18N + 'account.error'), recoverText: '', reauth: true, title }
    case 'disabled':
      return { tone: 'disabled', available: false, label: t(I18N + 'account.disabled'), recoverText: '', reauth: false, title }
    default:
      return { tone: 'disabled', available: false, label: a.status_text || String(a.status), recoverText: '', reauth: false, title }
  }
}

/** 可用的在前，同类里并发占用多的在前。 */
export function sortAccounts(accounts: TopologyAccount[]): TopologyAccount[] {
  const rank = (a: TopologyAccount) =>
    a.status === 'ok' ? 0 : a.status === 'cooldown' || a.status === 'rate_limited' ? 1 : a.status === 'error' ? 2 : 3
  return [...accounts].sort(
    (a, b) =>
      rank(a) - rank(b) ||
      b.concurrency_current - a.concurrency_current ||
      a.name.localeCompare(b.name) ||
      a.id - b.id
  )
}

export function strategyLabel(groups: TopologyGroup[], t: Translate): string {
  const set = new Set(groups.map(g => g.strategy).filter(Boolean))
  if (set.size === 0) return ''
  if (set.size > 1) return t(I18N + 'strategy.mixed')
  const only = [...set][0]
  return only === 'load_balance' || only === 'sticky' || only === 'priority' || only === 'round_robin'
    ? t(I18N + 'strategy.' + only)
    : only
}

// ==================== Geometry ====================

export interface Point {
  x: number
  y: number
}

export interface Curve {
  from: Point
  c1: Point
  c2: Point
  to: Point
}

/** 水平方向的 S 形三次贝塞尔：控制点在两端中点的 x 上。 */
export function makeCurve(from: Point, to: Point): Curve {
  const mx = (from.x + to.x) / 2
  return { from, c1: { x: mx, y: from.y }, c2: { x: mx, y: to.y }, to }
}

export function curvePath(c: Curve): string {
  const r = (n: number) => Math.round(n * 10) / 10
  return `M ${r(c.from.x)} ${r(c.from.y)} C ${r(c.c1.x)} ${r(c.c1.y)}, ${r(c.c2.x)} ${r(c.c2.y)}, ${r(c.to.x)} ${r(c.to.y)}`
}

export function pointOnCurve(c: Curve, tRaw: number): Point {
  const t = Math.max(0, Math.min(1, tRaw))
  const u = 1 - t
  const a = u * u * u
  const b = 3 * u * u * t
  const d = 3 * u * t * t
  const e = t * t * t
  return {
    x: a * c.from.x + b * c.c1.x + d * c.c2.x + e * c.to.x,
    y: a * c.from.y + b * c.c1.y + d * c.c2.y + e * c.to.y
  }
}

/**
 * 平台卡片左边缘上，n 条入线的落点 y：围绕 centerY 均匀排开，间距不超过 maxGap，
 * 也不超出 [top+pad, bottom-pad]。
 */
export function entryYs(n: number, top: number, bottom: number, maxGap = 32, pad = 40): number[] {
  if (n <= 0) return []
  const center = (top + bottom) / 2
  const room = Math.max(0, bottom - top - pad * 2)
  const gap = n > 1 ? Math.min(maxGap, room / (n - 1)) : 0
  const first = center - (gap * (n - 1)) / 2
  return Array.from({ length: n }, (_, i) => first + gap * i)
}

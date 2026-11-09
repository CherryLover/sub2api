/**
 * 实时路由事件 → 动画状态。纯函数，时间全部由调用方传入（客户端收到事件的时刻），
 * 不读服务器的 at 字段（避免时钟偏差）。
 *
 * 每个请求有一个 readyAt 游标：它的下一个画面要等上一个画面播完才开始，
 * 这样一次请求里几毫秒内连发的事件也能按顺序看清楚。
 */

import type { RoutingEvent } from '@/api/admin/topology'
import { I18N, LAST_GATE_INDEX, gateIndexForReason, type Translate } from './topologyModel'

export const TIMING = {
  dot: 650,
  gateStep: 70,
  gateLit: 420,
  attemptGlow: 1500,
  failFlash: 900,
  retryRing: 1600,
  switchFlash: 1100,
  doneTag: 2200,
  keyErrorTag: 2800,
  blockedChip: 3200,
  /** 超过这个时间没新事件的请求轨迹直接丢掉 */
  trackTtl: 10 * 60 * 1000
} as const

export const MAX_DOTS = 30
const MAX_FX = 240

export type DotLine =
  | { kind: 'key'; keyId: number }
  | { kind: 'account'; accountId: number }

export interface Dot {
  id: number
  line: DotLine
  /** forward: 沿线从左往右；back: 从右往左 */
  dir: 'forward' | 'back'
  tone: 'ok' | 'err'
  startAt: number
  endAt: number
}

export type FxTarget = 'account' | 'key' | 'exit'
export type FxKind = 'active' | 'ok' | 'failed' | 'retry' | 'switch' | 'giveup' | 'error'

export interface Fx {
  id: number
  target: FxTarget
  /** account / key 的 id；exit 用 0 */
  targetId: number
  kind: FxKind
  /** 标签文字的语义；由视图翻译 */
  tag?: TagSpec
  startAt: number
  endAt: number
}

export type TagSpec =
  | { code: 'success'; attempt: number }
  | { code: 'upstream'; status: number }
  | { code: 'retrySame'; attempt: number }
  | { code: 'switchNext'; attempt: number }
  | { code: 'giveUp' }
  | { code: 'returned'; status: number }

export interface GateRun {
  requestId: string
  startAt: number
  /** 拦下时停在哪一关（含）；null 表示一路通过 */
  stopIndex: number | null
  reason?: string
}

export interface GateRetry {
  requestId: string
  startAt: number
  endAt: number
  attempt: number
}

export interface RequestTrack {
  keyId: number
  accountId: number
  attempt: number
  readyAt: number
  lastAt: number
  /** 最后一次失败后已经把点送回出口 */
  returnedToExit: boolean
  failed: boolean
}

export interface Cooldown {
  startAt: number
  until: number
}

export interface AnimState {
  seq: number
  requests: Record<string, RequestTrack>
  dots: Dot[]
  fx: Fx[]
  gateRun: GateRun | null
  gateRetry: GateRetry | null
  blocked: { reason: string; startAt: number; endAt: number } | null
  cooldowns: Record<number, Cooldown>
}

export interface ReduceOptions {
  reducedMotion?: boolean
}

export function createAnimState(): AnimState {
  return {
    seq: 0,
    requests: {},
    dots: [],
    fx: [],
    gateRun: null,
    gateRetry: null,
    blocked: null,
    cooldowns: {}
  }
}

/** 9 道关卡逐个亮完所需的时长（最后一关亮起的时刻 + 一个 step） */
export function gateSweepMs(stopIndex: number = LAST_GATE_INDEX): number {
  return (stopIndex + 1) * TIMING.gateStep
}

function liveDotCount(s: AnimState, now: number): number {
  let n = 0
  for (const d of s.dots) if (d.endAt > now) n++
  return n
}

function addDot(s: AnimState, now: number, opts: ReduceOptions, line: DotLine, dir: Dot['dir'], tone: Dot['tone'], startAt: number): number {
  const duration = opts.reducedMotion ? 0 : TIMING.dot
  if (!opts.reducedMotion && liveDotCount(s, now) < MAX_DOTS) {
    s.dots.push({ id: ++s.seq, line, dir, tone, startAt, endAt: startAt + duration })
  }
  return startAt + duration
}

function addFx(s: AnimState, fx: Omit<Fx, 'id'>) {
  s.fx.push({ ...fx, id: ++s.seq })
  if (s.fx.length > MAX_FX) s.fx.splice(0, s.fx.length - MAX_FX)
}

function ensureTrack(s: AnimState, ev: RoutingEvent, now: number): RequestTrack {
  let tr = s.requests[ev.request_id]
  if (!tr) {
    tr = {
      keyId: ev.api_key_id ?? 0,
      accountId: 0,
      attempt: 0,
      readyAt: now,
      lastAt: now,
      returnedToExit: false,
      failed: false
    }
    s.requests[ev.request_id] = tr
  }
  if (ev.api_key_id) tr.keyId = ev.api_key_id
  tr.lastAt = now
  return tr
}

function clone(s: AnimState): AnimState {
  return {
    seq: s.seq,
    requests: Object.fromEntries(Object.entries(s.requests).map(([k, v]) => [k, { ...v }])),
    dots: [...s.dots],
    fx: [...s.fx],
    gateRun: s.gateRun,
    gateRetry: s.gateRetry,
    blocked: s.blocked,
    cooldowns: { ...s.cooldowns }
  }
}

/**
 * 应用一个路由事件，返回新的状态（不修改入参）。
 */
export function applyRoutingEvent(prev: AnimState, ev: RoutingEvent, now: number, opts: ReduceOptions = {}): AnimState {
  if (!ev || !ev.request_id) return prev
  const s = clone(prev)
  const motion = !opts.reducedMotion

  switch (ev.type) {
    case 'request_start': {
      const tr = ensureTrack(s, ev, now)
      const start = Math.max(now, tr.readyAt)
      const arrive = tr.keyId ? addDot(s, now, opts, { kind: 'key', keyId: tr.keyId }, 'forward', 'ok', start) : start
      if (tr.keyId) {
        addFx(s, { target: 'key', targetId: tr.keyId, kind: 'active', startAt: start, endAt: Math.max(arrive, start + 400) + 200 })
      }
      // 关卡只给最新的请求亮，避免并发时满屏闪
      if (motion) s.gateRun = { requestId: ev.request_id, startAt: arrive, stopIndex: null }
      tr.readyAt = arrive + (motion ? gateSweepMs() : 0)
      break
    }

    case 'blocked': {
      const tr = ensureTrack(s, ev, now)
      const reason = ev.reason || ''
      const stop = gateIndexForReason(reason)
      // 若 request_start 已经安排了关卡动画，就让这次关卡动画改为停在拦下的那关；
      // 此时 readyAt 指向整轮关卡扫完，不能拿来等，否则红色原因会比关卡晚出现。
      const run = s.gateRun && s.gateRun.requestId === ev.request_id ? s.gateRun : null
      const start = run ? now : Math.max(now, tr.readyAt)
      const sweepStart = run ? run.startAt : start
      if (motion) s.gateRun = { requestId: ev.request_id, startAt: sweepStart, stopIndex: stop, reason }
      const chipAt = motion ? Math.max(start, sweepStart + stop * TIMING.gateStep) : start
      s.blocked = { reason, startAt: chipAt, endAt: chipAt + TIMING.blockedChip }
      tr.readyAt = chipAt + (motion ? TIMING.gateStep * 2 : 0)
      tr.failed = true
      tr.returnedToExit = true // 点停在平台，不必再从账号往回走
      break
    }

    case 'attempt': {
      const tr = ensureTrack(s, ev, now)
      const accountId = ev.account_id ?? 0
      tr.attempt = ev.attempt ?? tr.attempt + 1
      tr.accountId = accountId
      tr.returnedToExit = false
      tr.failed = false
      const start = Math.max(now, tr.readyAt)
      const arrive = accountId ? addDot(s, now, opts, { kind: 'account', accountId }, 'forward', 'ok', start) : start
      if (accountId) {
        addFx(s, { target: 'account', targetId: accountId, kind: 'active', startAt: arrive, endAt: arrive + TIMING.attemptGlow })
      }
      tr.readyAt = arrive
      break
    }

    case 'attempt_failed': {
      const tr = ensureTrack(s, ev, now)
      const accountId = ev.account_id ?? tr.accountId
      if (ev.attempt) tr.attempt = ev.attempt
      tr.accountId = accountId
      tr.failed = true
      const status = ev.status_code ?? 0
      const start = Math.max(now, tr.readyAt)
      if (accountId) {
        addFx(s, {
          target: 'account', targetId: accountId, kind: 'failed',
          tag: { code: 'upstream', status }, startAt: start, endAt: start + TIMING.failFlash
        })
      }
      if (ev.cooldown_until && accountId) {
        const until = Date.parse(ev.cooldown_until)
        if (Number.isFinite(until) && until > now) {
          s.cooldowns[accountId] = { startAt: start, until }
        }
      }
      let cursor = start + (motion ? TIMING.failFlash : 0)
      const next = ev.next
      if (next === 'same_account') {
        if (accountId) {
          addFx(s, {
            target: 'account', targetId: accountId, kind: 'retry',
            tag: { code: 'retrySame', attempt: tr.attempt + 1 }, startAt: cursor, endAt: cursor + TIMING.retryRing
          })
        }
        cursor += motion ? TIMING.retryRing : 0
        tr.returnedToExit = false
      } else if (next === 'switch' || next === 'give_up') {
        if (accountId) cursor = addDot(s, now, opts, { kind: 'account', accountId }, 'back', 'err', cursor)
        tr.returnedToExit = true
        if (next === 'switch') {
          if (motion) s.gateRetry = { requestId: ev.request_id, startAt: cursor, endAt: cursor + TIMING.switchFlash, attempt: tr.attempt + 1 }
          addFx(s, {
            target: 'exit', targetId: 0, kind: 'switch',
            tag: { code: 'switchNext', attempt: tr.attempt + 1 }, startAt: cursor, endAt: cursor + TIMING.switchFlash
          })
          cursor += motion ? TIMING.switchFlash * 0.6 : 0
        } else {
          addFx(s, {
            target: 'exit', targetId: 0, kind: 'giveup',
            tag: { code: 'giveUp' }, startAt: cursor, endAt: cursor + TIMING.switchFlash
          })
          cursor += motion ? TIMING.switchFlash * 0.6 : 0
        }
      }
      tr.readyAt = cursor
      break
    }

    case 'request_done': {
      const tr = s.requests[ev.request_id]
      const status = ev.status_code ?? 0
      const start = Math.max(now, tr?.readyAt ?? now)
      const keyId = ev.api_key_id || tr?.keyId || 0
      if (status > 0 && status < 400) {
        if (tr?.accountId) {
          addFx(s, {
            target: 'account', targetId: tr.accountId, kind: 'ok',
            tag: { code: 'success', attempt: Math.max(1, tr.attempt) }, startAt: start, endAt: start + TIMING.doneTag
          })
        }
      } else {
        let cursor = start
        // 上游直接把错误透传回来（没有 attempt_failed）：先让点从账号退回出口
        if (tr?.accountId && !tr.returnedToExit) {
          addFx(s, {
            target: 'account', targetId: tr.accountId, kind: 'failed',
            tag: { code: 'upstream', status }, startAt: cursor, endAt: cursor + TIMING.failFlash
          })
          cursor = addDot(s, now, opts, { kind: 'account', accountId: tr.accountId }, 'back', 'err', cursor)
        }
        if (keyId) {
          cursor = addDot(s, now, opts, { kind: 'key', keyId }, 'back', 'err', cursor)
          addFx(s, {
            target: 'key', targetId: keyId, kind: 'error',
            tag: { code: 'returned', status }, startAt: cursor, endAt: cursor + TIMING.keyErrorTag
          })
        }
      }
      delete s.requests[ev.request_id]
      break
    }

    default:
      return prev
  }
  return s
}

/** 清理播完的点、过期的特效 / 冷却 / 长时间没动静的请求轨迹。 */
export function pruneAnimState(prev: AnimState, now: number): AnimState {
  const dots = prev.dots.filter(d => d.endAt > now)
  const fx = prev.fx.filter(f => f.endAt > now)
  const cooldowns: Record<number, Cooldown> = {}
  let cooldownChanged = false
  for (const [id, c] of Object.entries(prev.cooldowns)) {
    if (c.until > now) cooldowns[Number(id)] = c
    else cooldownChanged = true
  }
  const requests: Record<string, RequestTrack> = {}
  let reqChanged = false
  for (const [id, tr] of Object.entries(prev.requests)) {
    if (now - tr.lastAt < TIMING.trackTtl) requests[id] = tr
    else reqChanged = true
  }
  const gateRun = prev.gateRun && now - prev.gateRun.startAt > gateSweepMs() + TIMING.gateLit + TIMING.blockedChip ? null : prev.gateRun
  const gateRetry = prev.gateRetry && prev.gateRetry.endAt <= now ? null : prev.gateRetry
  const blocked = prev.blocked && prev.blocked.endAt <= now ? null : prev.blocked
  if (
    dots.length === prev.dots.length &&
    fx.length === prev.fx.length &&
    !cooldownChanged &&
    !reqChanged &&
    gateRun === prev.gateRun &&
    gateRetry === prev.gateRetry &&
    blocked === prev.blocked
  ) {
    return prev
  }
  return { ...prev, dots, fx, cooldowns: cooldownChanged ? cooldowns : prev.cooldowns, requests: reqChanged ? requests : prev.requests, gateRun, gateRetry, blocked }
}

/** 还有东西在播（或即将播）→ 需要继续逐帧刷新。 */
export function hasPendingAnimation(s: AnimState, now: number): boolean {
  return (
    s.dots.some(d => d.endAt > now) ||
    s.fx.some(f => f.endAt > now) ||
    Object.values(s.cooldowns).some(c => c.until > now) ||
    s.gateRun != null ||
    s.gateRetry != null ||
    s.blocked != null
  )
}

// ==================== Selectors ====================

export interface DotFrame {
  dot: Dot
  /** 0..1，已经按方向换算过：0 是线的左端，1 是右端 */
  pos: number
}

export function easeInOut(t: number): number {
  const x = Math.max(0, Math.min(1, t))
  return x < 0.5 ? 2 * x * x : 1 - Math.pow(-2 * x + 2, 2) / 2
}

export function activeDots(s: AnimState, now: number): DotFrame[] {
  const out: DotFrame[] = []
  for (const dot of s.dots) {
    if (dot.startAt > now || dot.endAt <= now) continue
    const p = easeInOut((now - dot.startAt) / Math.max(1, dot.endAt - dot.startAt))
    out.push({ dot, pos: dot.dir === 'forward' ? p : 1 - p })
  }
  return out
}

/** 目标此刻生效的特效（多个重叠时取最晚开始的那个）。 */
export function fxFor(s: AnimState, target: FxTarget, targetId: number, now: number): Fx | null {
  let best: Fx | null = null
  for (const f of s.fx) {
    if (f.target !== target || f.targetId !== targetId) continue
    if (f.startAt > now || f.endAt <= now) continue
    if (!best || f.startAt >= best.startAt) best = f
  }
  return best
}

export type GateVisual = 'idle' | 'lit' | 'blocked' | 'retry'

export function gateVisuals(s: AnimState, now: number, count = LAST_GATE_INDEX + 1): GateVisual[] {
  const out: GateVisual[] = Array.from({ length: count }, () => 'idle')
  const run = s.gateRun
  if (run) {
    const last = run.stopIndex ?? count - 1
    for (let i = 0; i <= last && i < count; i++) {
      const on = run.startAt + i * TIMING.gateStep
      if (run.stopIndex != null && i === run.stopIndex) {
        if (now >= on && now < on + TIMING.blockedChip) out[i] = 'blocked'
      } else if (now >= on && now < on + TIMING.gateLit) {
        out[i] = 'lit'
      }
    }
  }
  const retry = s.gateRetry
  if (retry && now >= retry.startAt && now < retry.endAt) out[count - 1] = 'retry'
  return out
}

export function cooldownLeftSeconds(s: AnimState, accountId: number, now: number): number {
  const c = s.cooldowns[accountId]
  if (!c || now < c.startAt || c.until <= now) return 0
  return Math.ceil((c.until - now) / 1000)
}

export function tagText(tag: TagSpec, t: Translate): string {
  switch (tag.code) {
    case 'success':
      return tag.attempt > 1 ? t(I18N + 'tag.successNth', { n: tag.attempt }) : t(I18N + 'tag.success')
    case 'upstream':
      if (!tag.status) return t(I18N + 'tag.upstreamNoResponse')
      if (tag.status === 429) return t(I18N + 'tag.upstreamRateLimited')
      return t(I18N + 'tag.upstream', { code: tag.status })
    case 'retrySame':
      return t(I18N + 'tag.retrySame', { n: tag.attempt })
    case 'switchNext':
      return t(I18N + 'tag.switchNext', { n: tag.attempt })
    case 'giveUp':
      return t(I18N + 'tag.giveUp')
    case 'returned':
      return t(I18N + 'tag.returned', { code: tag.status || '—' })
  }
}

export function activeBlocked(s: AnimState, now: number): { reason: string; gateIndex: number } | null {
  const b = s.blocked
  if (!b || now < b.startAt || now >= b.endAt) return null
  return { reason: b.reason, gateIndex: gateIndexForReason(b.reason) }
}

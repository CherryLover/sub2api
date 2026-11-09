import { describe, expect, it } from 'vitest'
import type { RoutingEvent } from '@/api/admin/topology'
import {
  MAX_DOTS,
  TIMING,
  activeBlocked,
  activeDots,
  applyRoutingEvent,
  cooldownLeftSeconds,
  createAnimState,
  easeInOut,
  fxFor,
  gateSweepMs,
  gateVisuals,
  hasPendingAnimation,
  pruneAnimState,
  tagText,
  type AnimState
} from '../topologyAnimation'
import { GATE_IDS } from '../topologyModel'
import { zhT } from './zhTranslate'

const T0 = 1_000_000

function ev(type: RoutingEvent['type'], over: Partial<RoutingEvent> = {}): RoutingEvent {
  return { type, request_id: 'r1', api_key_id: 7, at: '2026-11-09T00:00:00Z', ...over }
}

function run(events: Array<[number, RoutingEvent]>, opts = {}): AnimState {
  return events.reduce((s, [at, e]) => applyRoutingEvent(s, e, at, opts), createAnimState())
}

describe('request_start', () => {
  it('sends a dot from the key to the platform, highlights the key and sweeps the gates after arrival', () => {
    const s = run([[T0, ev('request_start')]])
    expect(s.dots).toHaveLength(1)
    expect(s.dots[0]).toMatchObject({ line: { kind: 'key', keyId: 7 }, dir: 'forward', tone: 'ok', startAt: T0, endAt: T0 + TIMING.dot })
    expect(fxFor(s, 'key', 7, T0 + 10)?.kind).toBe('active')
    expect(s.gateRun).toMatchObject({ requestId: 'r1', startAt: T0 + TIMING.dot, stopIndex: null })
    expect(s.requests.r1.readyAt).toBe(T0 + TIMING.dot + gateSweepMs())

    // gates light up top → bottom
    const arrive = T0 + TIMING.dot
    expect(gateVisuals(s, arrive - 1).every(g => g === 'idle')).toBe(true)
    expect(gateVisuals(s, arrive + 1)[0]).toBe('lit')
    expect(gateVisuals(s, arrive + 1)[8]).toBe('idle')
    expect(gateVisuals(s, arrive + 8 * TIMING.gateStep + 1)[8]).toBe('lit')
  })

  it('only animates the gates for the most recent request', () => {
    const s = run([
      [T0, ev('request_start', { request_id: 'a' })],
      [T0 + 100, ev('request_start', { request_id: 'b', api_key_id: 8 })]
    ])
    expect(s.gateRun?.requestId).toBe('b')
    expect(s.dots).toHaveLength(2)
  })

  it('does not mutate the previous state', () => {
    const s0 = createAnimState()
    const s1 = applyRoutingEvent(s0, ev('request_start'), T0)
    expect(s0.dots).toHaveLength(0)
    expect(s0.requests).toEqual({})
    expect(s1).not.toBe(s0)
  })
})

describe('attempt', () => {
  it('waits for the gate sweep, then moves a dot from the exit to the account and highlights it', () => {
    const s = run([
      [T0, ev('request_start')],
      [T0 + 5, ev('attempt', { account_id: 3, attempt: 1 })]
    ])
    const ready = T0 + TIMING.dot + gateSweepMs()
    const dot = s.dots[1]
    expect(dot).toMatchObject({ line: { kind: 'account', accountId: 3 }, dir: 'forward', startAt: ready })
    expect(fxFor(s, 'account', 3, dot.endAt + 1)?.kind).toBe('active')
    expect(fxFor(s, 'account', 3, dot.startAt)).toBeNull()
  })
})

describe('attempt_failed', () => {
  const base: Array<[number, RoutingEvent]> = [
    [T0, ev('request_start')],
    [T0, ev('attempt', { account_id: 3, attempt: 1 })]
  ]

  it('next=same_account: red flash with the upstream code, then an amber retry ring on the same account', () => {
    const s = run([...base, [T0 + 10, ev('attempt_failed', { account_id: 3, attempt: 1, status_code: 502, next: 'same_account' })]])
    const failFx = s.fx.find(f => f.kind === 'failed')!
    expect(failFx).toMatchObject({ target: 'account', targetId: 3, tag: { code: 'upstream', status: 502 } })
    expect(tagText(failFx.tag!, zhT)).toBe('上游 502')
    const retry = s.fx.find(f => f.kind === 'retry')!
    expect(retry.startAt).toBe(failFx.startAt + TIMING.failFlash)
    expect(tagText(retry.tag!, zhT)).toBe('原账号重试 · 第 2 次')
    expect(fxFor(s, 'account', 3, retry.startAt + 1)?.kind).toBe('retry')
    // no dot goes back for an in-place retry
    expect(s.dots.filter(d => d.dir === 'back')).toHaveLength(0)
  })

  it('next=switch: red dot back to the exit, last gate amber with "换下一个 · 第 N 次", next attempt continues after', () => {
    let s = run([...base, [T0 + 10, ev('attempt_failed', { account_id: 3, attempt: 1, status_code: 502, next: 'switch' })]])
    const back = s.dots.find(d => d.dir === 'back')!
    expect(back).toMatchObject({ line: { kind: 'account', accountId: 3 }, tone: 'err' })
    expect(s.gateRetry).toMatchObject({ startAt: back.endAt, attempt: 2 })
    expect(gateVisuals(s, back.endAt + 1)[GATE_IDS.length - 1]).toBe('retry')
    const exitFx = fxFor(s, 'exit', 0, back.endAt + 1)!
    expect(tagText(exitFx.tag!, zhT)).toBe('换下一个 · 第 2 次')

    s = applyRoutingEvent(s, ev('attempt', { account_id: 4, attempt: 2 }), T0 + 20)
    const next = s.dots.at(-1)!
    expect(next).toMatchObject({ line: { kind: 'account', accountId: 4 }, dir: 'forward' })
    expect(next.startAt).toBeGreaterThan(back.endAt)
  })

  it('next=give_up: dot back to the exit with "能换的都试过了"', () => {
    const s = run([...base, [T0 + 10, ev('attempt_failed', { account_id: 3, attempt: 2, status_code: 502, next: 'give_up' })]])
    const back = s.dots.find(d => d.dir === 'back')!
    const exitFx = fxFor(s, 'exit', 0, back.endAt + 1)!
    expect(exitFx.kind).toBe('giveup')
    expect(tagText(exitFx.tag!, zhT)).toBe('能换的都试过了')
    expect(s.gateRetry).toBeNull()
  })

  it('labels 429 and network errors specially', () => {
    expect(tagText({ code: 'upstream', status: 429 }, zhT)).toBe('上游 429 · 被限流')
    expect(tagText({ code: 'upstream', status: 0 }, zhT)).toBe('上游无响应')
  })

  it('cooldown_until starts a countdown that expires and is pruned', () => {
    const until = new Date(T0 + 60_000).toISOString()
    let s = run([...base, [T0 + 10, ev('attempt_failed', { account_id: 3, status_code: 429, next: 'switch', cooldown_until: until })]])
    const startAt = s.cooldowns[3].startAt
    expect(cooldownLeftSeconds(s, 3, startAt)).toBe(Math.ceil((T0 + 60_000 - startAt) / 1000))
    expect(cooldownLeftSeconds(s, 3, T0 + 59_001)).toBe(1)
    expect(cooldownLeftSeconds(s, 3, T0 + 60_000)).toBe(0)
    expect(hasPendingAnimation(s, T0 + 30_000)).toBe(true)
    s = pruneAnimState(s, T0 + 60_001)
    expect(s.cooldowns[3]).toBeUndefined()
  })

  it('ignores a cooldown that is already over', () => {
    const s = run([...base, [T0 + 10, ev('attempt_failed', { account_id: 3, next: 'switch', cooldown_until: new Date(T0 - 1).toISOString() })]])
    expect(s.cooldowns[3]).toBeUndefined()
  })
})

describe('request_done', () => {
  it('success: account turns green with "成功" / "第 N 次成功" and the track is dropped', () => {
    const s = run([
      [T0, ev('request_start')],
      [T0, ev('attempt', { account_id: 3, attempt: 1 })],
      [T0, ev('attempt_failed', { account_id: 3, attempt: 1, status_code: 502, next: 'switch' })],
      [T0, ev('attempt', { account_id: 4, attempt: 2 })],
      [T0, ev('request_done', { status_code: 200 })]
    ])
    const ok = s.fx.find(f => f.kind === 'ok')!
    expect(ok).toMatchObject({ target: 'account', targetId: 4 })
    expect(tagText(ok.tag!, zhT)).toBe('第 2 次成功')
    expect(s.requests.r1).toBeUndefined()
    expect(tagText({ code: 'success', attempt: 1 }, zhT)).toBe('成功')
  })

  it('failure after attempts: red dot back to the key and a "返回 <code>" tag on the key', () => {
    const s = run([
      [T0, ev('request_start')],
      [T0, ev('attempt', { account_id: 3, attempt: 1 })],
      [T0, ev('attempt_failed', { account_id: 3, attempt: 1, status_code: 502, next: 'give_up' })],
      [T0, ev('request_done', { status_code: 502 })]
    ])
    const keyBack = s.dots.find(d => d.line.kind === 'key' && d.dir === 'back')!
    expect(keyBack.tone).toBe('err')
    const keyFx = fxFor(s, 'key', 7, keyBack.endAt + 1)!
    expect(keyFx.kind).toBe('error')
    expect(tagText(keyFx.tag!, zhT)).toBe('返回 502')
    // the account→exit return already happened on give_up; no second one
    expect(s.dots.filter(d => d.line.kind === 'account' && d.dir === 'back')).toHaveLength(1)
  })

  it('an error passed straight through (no attempt_failed) first walks back from the account', () => {
    const s = run([
      [T0, ev('request_start')],
      [T0, ev('attempt', { account_id: 3, attempt: 1 })],
      [T0, ev('request_done', { status_code: 400 })]
    ])
    expect(s.dots.filter(d => d.dir === 'back').map(d => d.line.kind)).toEqual(['account', 'key'])
  })

  it('works for a done event without a known track', () => {
    const s = run([[T0, ev('request_done', { request_id: 'ghost', status_code: 503 })]])
    expect(fxFor(s, 'key', 7, T0 + TIMING.dot + 1)?.kind).toBe('error')
  })
})

describe('blocked', () => {
  it('stops the dot at the platform, sweeps down to the blocking gate and shows the reason', () => {
    const s = run([
      [T0, ev('request_start')],
      [T0 + 5, ev('blocked', { reason: 'model_not_supported' })]
    ])
    const modelIdx = GATE_IDS.indexOf('model')
    expect(s.gateRun).toMatchObject({ stopIndex: modelIdx, reason: 'model_not_supported' })
    const blockAt = T0 + TIMING.dot + modelIdx * TIMING.gateStep
    const vis = gateVisuals(s, blockAt + 1)
    expect(vis[modelIdx]).toBe('blocked')
    expect(vis.slice(modelIdx + 1).every(v => v === 'idle')).toBe(true)
    expect(activeBlocked(s, blockAt + 1)).toEqual({ reason: 'model_not_supported', gateIndex: modelIdx })
    expect(activeBlocked(s, blockAt + TIMING.blockedChip + 1)).toBeNull()
    // only the key → platform dot; nothing towards accounts
    expect(s.dots.every(d => d.line.kind === 'key')).toBe(true)
  })

  it('then request_done sends a red dot back to the key without an account step', () => {
    const s = run([
      [T0, ev('request_start')],
      [T0, ev('blocked', { reason: 'capacity' })],
      [T0, ev('request_done', { status_code: 503 })]
    ])
    expect(s.dots.filter(d => d.dir === 'back').map(d => d.line.kind)).toEqual(['key'])
    expect(s.fx.find(f => f.target === 'key' && f.kind === 'error')?.tag).toEqual({ code: 'returned', status: 503 })
  })
})

describe('limits, pruning and reduced motion', () => {
  it(`caps live dots at ${MAX_DOTS}`, () => {
    let s = createAnimState()
    for (let i = 0; i < MAX_DOTS + 10; i++) {
      s = applyRoutingEvent(s, ev('request_start', { request_id: `r${i}` }), T0)
    }
    expect(s.dots).toHaveLength(MAX_DOTS)
  })

  it('prunes finished dots and effects, returning the same object when nothing changed', () => {
    const s = run([[T0, ev('request_start')]])
    expect(pruneAnimState(s, T0 + 1)).toBe(s)
    const later = pruneAnimState(s, T0 + 60_000)
    expect(later.dots).toHaveLength(0)
    expect(later.fx).toHaveLength(0)
    expect(later.gateRun).toBeNull()
    expect(hasPendingAnimation(later, T0 + 60_000)).toBe(false)
  })

  it('drops request tracks that went quiet for too long', () => {
    const s = run([[T0, ev('request_start')]])
    expect(pruneAnimState(s, T0 + TIMING.trackTtl + 1).requests.r1).toBeUndefined()
  })

  it('reduced motion: no dots or gate sweep, but state highlights still apply', () => {
    const s = run(
      [
        [T0, ev('request_start')],
        [T0, ev('attempt', { account_id: 3 })],
        [T0, ev('attempt_failed', { account_id: 3, status_code: 502, next: 'switch' })]
      ],
      { reducedMotion: true }
    )
    expect(s.dots).toHaveLength(0)
    expect(s.gateRun).toBeNull()
    expect(s.gateRetry).toBeNull()
    expect(fxFor(s, 'account', 3, T0)?.kind).toBe('failed')
  })

  it('positions dots along their line with easing, reversed for back dots', () => {
    const s = run([
      [T0, ev('request_start')],
      [T0, ev('attempt', { account_id: 3 })],
      [T0, ev('attempt_failed', { account_id: 3, status_code: 500, next: 'give_up' })]
    ])
    const fwd = activeDots(s, T0 + TIMING.dot / 2)
    expect(fwd).toHaveLength(1)
    expect(fwd[0].pos).toBeCloseTo(0.5)
    const back = s.dots.find(d => d.dir === 'back')!
    const frame = activeDots(s, back.startAt + 1).find(f => f.dot.id === back.id)!
    expect(frame.pos).toBeGreaterThan(0.99)
    expect(easeInOut(0)).toBe(0)
    expect(easeInOut(1)).toBe(1)
  })

  it('ignores events without a request id or with an unknown type', () => {
    const s0 = createAnimState()
    expect(applyRoutingEvent(s0, { ...ev('attempt'), request_id: '' }, T0)).toBe(s0)
    expect(applyRoutingEvent(s0, { ...ev('attempt'), type: 'other' as RoutingEvent['type'] }, T0)).toBe(s0)
  })
})

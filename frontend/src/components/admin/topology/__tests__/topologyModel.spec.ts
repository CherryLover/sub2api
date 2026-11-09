import { describe, expect, it } from 'vitest'
import type { TopologyAccount, TopologyGroup, TopologyKey } from '@/api/admin/topology'
import {
  GATE_IDS,
  KNOWN_REASONS,
  accountQuotaView,
  accountStatusView,
  curvePath,
  entryYs,
  foldKeys,
  formatQuotaBar,
  gateIndexForReason,
  keyLoadKind,
  makeCurve,
  pointOnCurve,
  reasonLabel,
  sortAccounts,
  strategyLabel,
  summarizeGateBlocks
} from '../topologyModel'
import { zhT } from './zhTranslate'

const NOW = new Date(2026, 10, 9, 12, 0, 0).getTime()
const inMs = (ms: number) => new Date(NOW + ms).toISOString()
const HOUR = 3600_000

function key(id: number, over: Partial<TopologyKey> = {}): TopologyKey {
  return { id, name: `k${id}`, group_id: 1, group_name: 'G', in_flight: 0, queued: 0, requests_5m: 0, failed_5m: 0, ...over }
}

function account(id: number, over: Partial<TopologyAccount> = {}): TopologyAccount {
  return {
    id,
    name: `a${id}`,
    platform: 'OpenAI',
    status: 'ok',
    status_text: '',
    recover_at: null,
    concurrency_current: 0,
    concurrency_max: 10,
    quota_5h: null,
    quota_7d: null,
    billing: 'subscription',
    ...over
  }
}

describe('foldKeys', () => {
  it('shows keys with in-flight, queued or recent requests and folds the rest', () => {
    const keys = [
      key(1, { name: 'idle-b' }),
      key(2, { in_flight: 1 }),
      key(3, { requests_5m: 4 }),
      key(4, { queued: 2 }),
      key(5, { name: 'idle-a' })
    ]
    const { visible, folded } = foldKeys(keys)
    expect(visible.map(k => k.id).sort()).toEqual([2, 3, 4])
    expect(folded.map(k => k.name)).toEqual(['idle-a', 'idle-b'])
  })

  it('groups keys together (busiest group first) and sorts by activity inside a group', () => {
    const keys = [
      key(1, { group_name: 'OpenAI', group_id: 2, requests_5m: 50 }),
      key(2, { group_name: 'Composite', group_id: 1, in_flight: 1 }),
      key(3, { group_name: 'OpenAI', group_id: 2, in_flight: 1 }),
      key(4, { group_name: 'Composite', group_id: 1, in_flight: 3 }),
      key(5, { group_name: 'ds', group_id: 3, requests_5m: 1 })
    ]
    const { visible } = foldKeys(keys)
    expect(visible.map(k => k.id)).toEqual([4, 2, 3, 1, 5])
  })

  it('handles an empty list', () => {
    expect(foldKeys([])).toEqual({ visible: [], folded: [] })
  })
})

describe('keyLoadKind', () => {
  it('maps counters to active / queued / idle', () => {
    expect(keyLoadKind(key(1, { in_flight: 2, queued: 1 }))).toBe('active')
    expect(keyLoadKind(key(1, { queued: 1 }))).toBe('queued')
    expect(keyLoadKind(key(1, { requests_5m: 9 }))).toBe('idle')
  })
})

describe('quota formatting', () => {
  it('formats used percent with a relative reset time', () => {
    expect(formatQuotaBar('5h', { used_percent: 12.4, reset_at: inMs(3 * HOUR - 60_000) }, NOW, zhT)).toEqual({
      window: '5h', percent: 12, tone: 'ok', text: '12% · 3 小时后重置'
    })
    expect(formatQuotaBar('7d', { used_percent: 87, reset_at: inMs(2 * 24 * HOUR) }, NOW, zhT)).toMatchObject({
      tone: 'warn', text: '87% · 2 天后重置'
    })
    expect(formatQuotaBar('5h', { used_percent: 3, reset_at: inMs(10 * 60_000) }, NOW, zhT).text).toBe('3% · 10 分钟后重置')
  })

  it('shows an exhausted window with its recovery clock and clamps the bar', () => {
    const reset = new Date(2026, 10, 11, 17, 36).toISOString()
    expect(formatQuotaBar('7d', { used_percent: 130, reset_at: reset }, NOW, zhT)).toEqual({
      window: '7d', percent: 100, tone: 'full', text: '用完 · 11/11 17:36 恢复'
    })
    expect(formatQuotaBar('7d', { used_percent: 100, reset_at: null }, NOW, zhT).text).toBe('用完')
  })

  it('omits the reset hint when reset_at is missing and survives bad numbers', () => {
    expect(formatQuotaBar('5h', { used_percent: 40, reset_at: null }, NOW, zhT).text).toBe('40%')
    expect(formatQuotaBar('5h', { used_percent: Number.NaN, reset_at: null }, NOW, zhT).percent).toBe(0)
  })

  it('hides null windows and returns pay-as-you-go mode', () => {
    expect(accountQuotaView(account(1), NOW, zhT)).toEqual({ mode: 'bars', bars: [] })
    const only7d = accountQuotaView(account(1, { quota_7d: { used_percent: 2, reset_at: null } }), NOW, zhT)
    expect(only7d.mode === 'bars' && only7d.bars.map(b => b.window)).toEqual(['7d'])
    const both = accountQuotaView(
      account(1, { quota_5h: { used_percent: 1, reset_at: null }, quota_7d: { used_percent: 2, reset_at: null } }),
      NOW,
      zhT
    )
    expect(both.mode === 'bars' && both.bars.map(b => b.window)).toEqual(['5h', '7d'])
    expect(
      accountQuotaView(account(1, { billing: 'pay_as_you_go', quota_5h: { used_percent: 50, reset_at: null } }), NOW, zhT)
    ).toEqual({ mode: 'payg' })
  })
})

describe('accountStatusView', () => {
  it('treats ok accounts as available without a label', () => {
    expect(accountStatusView(account(1), NOW, zhT)).toMatchObject({ tone: 'ok', available: true, label: '', reauth: false })
  })

  it('shows rate-limited / cooldown in amber with the recovery time', () => {
    const recover = new Date(2026, 10, 11, 16, 26).toISOString()
    expect(accountStatusView(account(1, { status: 'rate_limited', recover_at: recover }), NOW, zhT)).toMatchObject({
      tone: 'warn', available: false, label: '限流中', recoverText: '11/11 16:26 恢复'
    })
    expect(accountStatusView(account(1, { status: 'cooldown' }), NOW, zhT)).toMatchObject({
      tone: 'warn', label: '冷却中', recoverText: ''
    })
    // a recovery time in the past is not shown
    expect(accountStatusView(account(1, { status: 'cooldown', recover_at: inMs(-1000) }), NOW, zhT).recoverText).toBe('')
  })

  it('marks error accounts red with a re-authorize link and disabled ones grey', () => {
    expect(accountStatusView(account(1, { status: 'error', status_text: 'token expired' }), NOW, zhT)).toMatchObject({
      tone: 'error', available: false, label: '授权失效', reauth: true, title: 'token expired'
    })
    expect(accountStatusView(account(1, { status: 'disabled' }), NOW, zhT)).toMatchObject({
      tone: 'disabled', available: false, label: '已停用', reauth: false
    })
  })

  it('falls back to status_text for unknown statuses', () => {
    const odd = account(1, { status: 'weird' as TopologyAccount['status'], status_text: '奇怪' })
    expect(accountStatusView(odd, NOW, zhT)).toMatchObject({ tone: 'disabled', available: false, label: '奇怪' })
  })

  it('sorts available accounts first, busiest first', () => {
    const list = [
      account(1, { status: 'disabled' }),
      account(2, { concurrency_current: 1 }),
      account(3, { status: 'error' }),
      account(4, { status: 'rate_limited' }),
      account(5, { concurrency_current: 4 })
    ]
    expect(sortAccounts(list).map(a => a.id)).toEqual([5, 2, 4, 3, 1])
  })
})

describe('gates and block reasons', () => {
  it('has the nine gates from the design, each with zh copy', () => {
    expect(GATE_IDS).toHaveLength(9)
    for (const id of GATE_IDS) {
      expect(zhT(`admin.topology.gates.${id}.title`)).toBeTruthy()
      expect(zhT(`admin.topology.gates.${id}.desc`)).toBeTruthy()
    }
    expect(zhT('admin.topology.gates.dispatch.title')).toBe('挑最闲的发出去')
  })

  it('maps reasons to the gate that blocks them, unknown → last gate', () => {
    expect(GATE_IDS[gateIndexForReason('model_not_supported')]).toBe('model')
    expect(GATE_IDS[gateIndexForReason('platform_mismatch')]).toBe('platform')
    expect(GATE_IDS[gateIndexForReason('quota_exhausted')]).toBe('quota')
    expect(GATE_IDS[gateIndexForReason('concurrency_full')]).toBe('capacity')
    expect(GATE_IDS[gateIndexForReason('something_new')]).toBe('dispatch')
    expect(GATE_IDS[gateIndexForReason(undefined)]).toBe('dispatch')
  })

  it('labels every known reason in zh and passes unknown codes through', () => {
    for (const r of KNOWN_REASONS) expect(reasonLabel(r, zhT)).not.toBe(r)
    expect(reasonLabel('something_new', zhT)).toBe('something_new')
    expect(reasonLabel('', zhT)).toBe('原因未知')
  })

  it('summarizes 5-minute gate blocks', () => {
    expect(summarizeGateBlocks({})).toEqual({ total: 0, top: null })
    expect(summarizeGateBlocks(null)).toEqual({ total: 0, top: null })
    expect(summarizeGateBlocks({ capacity: 2, model_not_supported: 5, zero: 0 })).toEqual({
      total: 7,
      top: { reason: 'model_not_supported', count: 5 }
    })
  })
})

describe('strategyLabel', () => {
  const g = (strategy: string): TopologyGroup => ({ id: 1, name: 'g', platform: 'openai', strategy, in_flight: 0, requests_5m: 0, failed_5m: 0, account_ids: [] })
  it('shows the shared strategy, a mixed label or nothing', () => {
    expect(strategyLabel([g('load_balance'), g('load_balance')], zhT)).toBe('负载均衡')
    expect(strategyLabel([g('load_balance'), g('sticky')], zhT)).toBe('多种策略')
    expect(strategyLabel([], zhT)).toBe('')
    expect(strategyLabel([g('custom')], zhT)).toBe('custom')
  })
})

describe('geometry', () => {
  it('builds a horizontal S curve and samples its end points', () => {
    const c = makeCurve({ x: 296, y: 228 }, { x: 488, y: 404 })
    expect(curvePath(c)).toBe('M 296 228 C 392 228, 392 404, 488 404')
    expect(pointOnCurve(c, 0)).toEqual({ x: 296, y: 228 })
    expect(pointOnCurve(c, 1)).toEqual({ x: 488, y: 404 })
    const mid = pointOnCurve(c, 0.5)
    expect(mid.x).toBeCloseTo(392)
    expect(mid.y).toBeCloseTo(316)
  })

  it('spreads entry points around the platform centre and squeezes them to fit', () => {
    expect(entryYs(0, 0, 100)).toEqual([])
    expect(entryYs(1, 100, 900)).toEqual([500])
    expect(entryYs(3, 100, 900)).toEqual([468, 500, 532])
    const tight = entryYs(11, 0, 280, 32, 40) // room 200 → gap 20
    expect(tight[0]).toBe(40)
    expect(tight[10]).toBe(240)
  })
})

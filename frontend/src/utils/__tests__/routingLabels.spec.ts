import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'

import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import type { RoutingInfo } from '@/types'
import { diagnosisFixture } from '@/components/admin/usage/__tests__/routingFixtures'
import {
  ROUTING_REASON_CODES,
  candidateReasonLabel,
  candidatesSummaryLabel,
  countSwitches,
  diagnosisFixHint,
  diagnosisFunnel,
  dominantReason,
  gateState,
  routingChip,
  routingReasonLabel,
  routingWhy,
  sortCandidatesByLoad
} from '../routingLabels'

const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh, en } })
const t = i18n.global.t as unknown as (key: string, params?: Record<string, unknown>) => string

describe('routing reason labels', () => {
  it('maps every known reason code to a zh and en label', () => {
    for (const code of ROUTING_REASON_CODES) {
      i18n.global.locale.value = 'zh'
      const zhLabel = routingReasonLabel(t, code)
      expect(zhLabel, code).not.toBe(code)
      expect(zhLabel, code).not.toContain('admin.routing')
      i18n.global.locale.value = 'en'
      expect(routingReasonLabel(t, code), code).not.toContain('admin.routing')
      expect(candidateReasonLabel(t, code, 'm'), code).not.toContain('admin.routing')
    }
    i18n.global.locale.value = 'zh'
  })

  it('uses the Chinese wording from the design', () => {
    expect(routingReasonLabel(t, 'model_not_supported')).toBe('没有账号支持该模型')
    expect(routingReasonLabel(t, 'not_schedulable')).toBe('账号都不可调度')
    expect(routingReasonLabel(t, 'empty_pool')).toBe('分组里没有账号')
    expect(routingReasonLabel(t, 'excluded')).toBe('账号都已试过失败')
    expect(candidateReasonLabel(t, 'model_not_supported', 'gpt-6.1-sol')).toBe('模型列表里没有 gpt-6.1-sol')
  })

  it('handles quota windows, unknown codes and empty input', () => {
    expect(routingReasonLabel(t, 'quota_auto_pause_7d')).toBe('额度快用完自动暂停（7d）')
    expect(candidateReasonLabel(t, 'quota_auto_pause_5h')).toBe('5h 额度快用完，自动暂停')
    expect(routingReasonLabel(t, 'brand_new_code')).toBe('brand_new_code')
    expect(routingReasonLabel(t, '')).toBe('')
    expect(routingReasonLabel(t, null)).toBe('')
  })
})

describe('diagnosis helpers', () => {
  it('counts candidates per gate for the funnel, skipping gates never evaluated', () => {
    expect(diagnosisFunnel(diagnosisFixture)).toEqual([
      { gate: 'platform', count: 3 },
      { gate: 'schedulable', count: 2 },
      { gate: 'quota', count: 2 },
      { gate: 'model', count: 0 }
    ])
  })

  it('reports pass / fail / not evaluated per gate', () => {
    const c = diagnosisFixture.candidates![2]
    expect(gateState(c, 'platform')).toBe('pass')
    expect(gateState(c, 'schedulable')).toBe('fail')
    expect(gateState(c, 'model')).toBe('skip')
  })

  it('summarises candidates as usable or tried', () => {
    expect(candidatesSummaryLabel(t, diagnosisFixture)).toBe('4 个候选 · 0 个可用')
    expect(candidatesSummaryLabel(t, { pool_size: 0, candidates: [] })).toBe('0 个候选')
    expect(candidatesSummaryLabel(t, {
      pool_size: 2,
      candidates: [
        { account_id: 1, account_name: 'a', reason: 'excluded', load_current: 0, load_max: 0 },
        { account_id: 2, account_name: 'b', reason: 'excluded', load_current: 0, load_max: 0 }
      ],
      attempts: [
        { attempt: 1, account_id: 1, account_name: 'a', status_code: 502, duration_ms: 3100, next: 'switch' },
        { attempt: 2, account_id: 2, account_name: 'b', status_code: 502, duration_ms: 2400, next: 'give_up' }
      ]
    })).toBe('2 个候选 · 2 个试过')
  })

  it('derives the dominant reason when the diagnosis has none', () => {
    expect(dominantReason({ ...diagnosisFixture, reason: undefined })).toBe('model_not_supported')
    expect(dominantReason({ pool_size: 0, candidates: [] })).toBe('empty_pool')
  })

  it('gives a fix hint per dominant reason', () => {
    const fmt = () => '11/11 17:36'
    expect(diagnosisFixHint(t, diagnosisFixture, fmt)).toMatchObject({ action: 'accounts' })
    expect(diagnosisFixHint(t, diagnosisFixture, fmt)!.text).toContain('gpt-6.1-sol')
    expect(diagnosisFixHint(t, { pool_size: 0, candidates: [], reason: 'empty_pool' }, fmt)).toEqual({
      text: '分组里一个账号都没有，给分组加账号。',
      action: 'groups'
    })
    const hint = diagnosisFixHint(t, { ...diagnosisFixture, reason: 'not_schedulable' }, fmt)!
    expect(hint.text).toContain('11/11 17:36')
    expect(diagnosisFixHint(t, null, fmt)).toBeNull()
  })
})

describe('routing chip', () => {
  const base: RoutingInfo = { method: 'load_balance', candidate_count: 3 }

  it('labels each method', () => {
    expect(routingChip(t, base)!.label).toBe('负载均衡 · 3 选 1')
    expect(routingChip(t, { method: 'sticky', candidate_count: 3 })!.label).toBe('沿用上次账号')
    expect(routingChip(t, { method: 'single', candidate_count: 1 })!.label).toBe('只有 1 个候选')
    expect(routingChip(t, { ...base, queue_wait_ms: 2100 })!.label).toBe('负载均衡 · 排队 2.1s')
    expect(routingChip(t, {
      ...base,
      attempts: [
        { attempt: 1, account_id: 1, account_name: 'a', status_code: 502, duration_ms: 8200, next: 'switch' },
        { attempt: 2, account_id: 2, account_name: 'b', status_code: 200, duration_ms: 2000 }
      ]
    })!.label).toBe('切换过 1 次账号')
    expect(routingChip(t, {
      ...base,
      attempts: [
        { attempt: 1, account_id: 1, account_name: 'a', status_code: 429, duration_ms: 100, next: 'same_account' },
        { attempt: 2, account_id: 1, account_name: 'a', status_code: 200, duration_ms: 2000 }
      ]
    })!.label).toBe('重试过 1 次')
    expect(routingChip(t, null)).toBeNull()
  })

  it('counts account switches between consecutive attempts', () => {
    expect(countSwitches([])).toBe(0)
    expect(countSwitches([
      { attempt: 1, account_id: 1, account_name: 'a', status_code: 502, duration_ms: 1 },
      { attempt: 2, account_id: 1, account_name: 'a', status_code: 502, duration_ms: 1 },
      { attempt: 3, account_id: 2, account_name: 'b', status_code: 200, duration_ms: 1 }
    ])).toBe(1)
  })

  it('sorts candidates chosen first, then by load, skipped last', () => {
    const sorted = sortCandidatesByLoad([
      { account_id: 1, account_name: 'busy', load_current: 6, load_max: 10 },
      { account_id: 2, account_name: 'skip', reason: 'not_schedulable', load_current: 0, load_max: 10 },
      { account_id: 3, account_name: 'chosen', chosen: true, load_current: 3, load_max: 10 }
    ])
    expect(sorted.map((c) => c.account_name)).toEqual(['chosen', 'busy', 'skip'])
  })

  it('explains why the account was chosen', () => {
    const why = routingWhy(t, {
      method: 'load_balance',
      candidate_count: 3,
      candidates: [
        { account_id: 3, account_name: 'g1.jacobs', chosen: true, load_current: 3, load_max: 10 },
        { account_id: 1, account_name: 'busy', load_current: 6, load_max: 10 },
        { account_id: 2, account_name: 'skip', reason: 'not_schedulable', load_current: 0, load_max: 10 }
      ]
    }, '')
    expect(why).toBe('3 个候选里 2 个能用，g1.jacobs 当时最闲（3/10），所以分给了它。')
    expect(routingWhy(t, { method: 'sticky', candidate_count: 2 }, 'ds-api')).toContain('ds-api')
  })
})

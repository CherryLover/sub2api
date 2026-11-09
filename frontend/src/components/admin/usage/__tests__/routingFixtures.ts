// 调度可视化单测共用的诊断 / 选号样例
import type { RoutingDiagnosis, RoutingInfo } from '@/types'

export const diagnosisFixture: RoutingDiagnosis = {
  group_id: 3,
  group_name: 'Composite',
  model: 'gpt-6.1-sol',
  pool_size: 4,
  reason: 'model_not_supported',
  raw: 'no available OpenAI accounts supporting model: gpt-6.1-sol (pool=4, filtered: model_not_supported=2 not_schedulable=1 platform_mismatch=1)',
  candidates: [
    { account_id: 1, account_name: 'g1.jacobs', checks: { platform: true, schedulable: true, quota: true, model: false }, reason: 'model_not_supported', load_current: 3, load_max: 10 },
    { account_id: 2, account_name: 'biz-jianghongbo', checks: { platform: true, schedulable: true, quota: true, model: false }, reason: 'model_not_supported', load_current: 6, load_max: 10 },
    { account_id: 3, account_name: 'biz-jacobs', checks: { platform: true, schedulable: false }, reason: 'not_schedulable', recover_at: '2026-11-11T09:36:00Z', load_current: 0, load_max: 10 },
    { account_id: 4, account_name: 'ds-api', checks: { platform: false }, reason: 'platform_mismatch', load_current: 1, load_max: 10 }
  ]
}

export const attemptsDiagnosisFixture: RoutingDiagnosis = {
  group_name: 'OpenAI',
  model: 'gpt-5.6-luna',
  pool_size: 2,
  reason: 'excluded',
  candidates: [
    { account_id: 11, account_name: 'biz-jianghongbo', checks: { platform: true, schedulable: true }, reason: 'excluded', load_current: 0, load_max: 10 },
    { account_id: 12, account_name: 'g1.jacobs', checks: { platform: true, schedulable: true }, reason: 'excluded', load_current: 0, load_max: 10 }
  ],
  attempts: [
    { attempt: 1, account_id: 11, account_name: 'biz-jianghongbo', status_code: 502, duration_ms: 3100, next: 'switch' },
    { attempt: 2, account_id: 12, account_name: 'g1.jacobs', status_code: 502, duration_ms: 2400, next: 'give_up' }
  ]
}

export const loadBalanceInfoFixture: RoutingInfo = {
  method: 'load_balance',
  candidate_count: 3,
  candidates: [
    { account_id: 2, account_name: 'biz-jianghongbo', load_current: 6, load_max: 10 },
    { account_id: 3, account_name: 'biz-jacobs', reason: 'not_schedulable', load_current: 0, load_max: 10 },
    { account_id: 1, account_name: 'g1.jacobs', chosen: true, load_current: 3, load_max: 10 }
  ]
}

export const switchedInfoFixture: RoutingInfo = {
  method: 'load_balance',
  candidate_count: 2,
  attempts: [
    { attempt: 1, account_id: 1, account_name: 'g1.jacobs', status_code: 502, duration_ms: 8200, next: 'switch' },
    { attempt: 2, account_id: 2, account_name: 'biz-jianghongbo', status_code: 200, duration_ms: 2650 }
  ]
}

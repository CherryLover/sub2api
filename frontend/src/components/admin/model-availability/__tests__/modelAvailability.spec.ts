import { describe, expect, it } from 'vitest'

import type { Account, AccountDiagnosis, AccountSchedulingBlock } from '@/types'

import {
  buildModelAvailabilityMatrix,
  filterIssuesOnly,
  modelLabelKey,
  resolveReason,
  type ModelAvailabilityInput
} from '../modelAvailability'

const NOW = Date.parse('2026-09-11T10:00:00Z')
const FUTURE = '2026-09-15T07:04:37Z'
const PAST = '2026-09-01T07:04:37Z'

function makeAccount(overrides: Partial<Account> = {}): Account {
  return {
    id: 1,
    name: 'acct-1',
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 1,
    priority: 0,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides
  } as Account
}

function makeDiagnosis(blocks: AccountSchedulingBlock[], schedulable = true): AccountDiagnosis {
  return { scheduling: { schedulable, blocks }, recent_errors: null }
}

function cellOf(
  matrix: ReturnType<typeof buildModelAvailabilityMatrix>,
  accountId: number,
  model: string
) {
  const row = matrix.rows.find((item) => item.id === accountId)
  expect(row, `row ${accountId} must exist`).toBeTruthy()
  const index = matrix.columns.findIndex((column) => column.model === model)
  expect(index, `column ${model} must exist`).toBeGreaterThanOrEqual(0)
  return row!.cells[index]
}

describe('buildModelAvailabilityMatrix', () => {
  it('renders one row per account and one column per model', () => {
    const inputs: ModelAvailabilityInput[] = [
      { account: makeAccount({ id: 1, name: 'alpha' }), models: ['gpt-5.3-codex', 'gpt-5.3-codex-spark'] },
      { account: makeAccount({ id: 2, name: 'bravo' }), models: ['gpt-5.3-codex'] }
    ]

    const matrix = buildModelAvailabilityMatrix(inputs, NOW)

    expect(matrix.rows.map((row) => row.name)).toEqual(['alpha', 'bravo'])
    expect(matrix.columns.map((column) => column.model)).toEqual(['gpt-5.3-codex', 'gpt-5.3-codex-spark'])
    expect(cellOf(matrix, 1, 'gpt-5.3-codex').state).toBe('ok')
    // bravo 没映射 spark：不是「可用」，也不是「限流」
    expect(cellOf(matrix, 2, 'gpt-5.3-codex-spark').state).toBe('unsupported')
    expect(matrix.summary).toEqual({
      accountCount: 2,
      modelCount: 2,
      blockedAccountCount: 0,
      limitedCellCount: 0
    })
  })

  it('marks model-level rate limits with reset time and raw reason', () => {
    const account = makeAccount({
      id: 7,
      name: 'spark',
      extra: {
        model_rate_limits: {
          'gpt-5.3-codex-spark': {
            rate_limited_at: '2026-09-11T09:00:00Z',
            rate_limit_reset_at: FUTURE,
            reason: 'openai_dedicated_quota_pool_rate_limited'
          }
        }
      }
    })

    const matrix = buildModelAvailabilityMatrix(
      [{ account, models: ['gpt-5.3-codex', 'gpt-5.3-codex-spark'] }],
      NOW
    )

    const cell = cellOf(matrix, 7, 'gpt-5.3-codex-spark')
    expect(cell.state).toBe('limited')
    expect(cell.until).toBe(FUTURE)
    expect(cell.details).toEqual([
      { source: 'model_rate_limit', until: FUTURE, reason: 'openai_dedicated_quota_pool_rate_limited' }
    ])
    expect(cellOf(matrix, 7, 'gpt-5.3-codex').state).toBe('ok')
    expect(matrix.summary.limitedCellCount).toBe(1)
  })

  it('ignores model rate limits whose reset time already passed', () => {
    const account = makeAccount({
      id: 8,
      name: 'stale',
      extra: {
        model_rate_limits: {
          'gpt-5.3-codex-spark': { rate_limited_at: PAST, rate_limit_reset_at: PAST, reason: 'whatever' }
        }
      }
    })

    const matrix = buildModelAvailabilityMatrix(
      [{ account, models: ['gpt-5.3-codex-spark'] }],
      NOW
    )

    expect(cellOf(matrix, 8, 'gpt-5.3-codex-spark').state).toBe('ok')
    expect(matrix.summary.limitedCellCount).toBe(0)
    expect(matrix.columns[0].limitedCount).toBe(0)
  })

  it('picks up in-process model blocks and drops expired ones', () => {
    const account = makeAccount({ id: 9, name: 'grok', platform: 'grok' })
    const diagnosis = makeDiagnosis([
      { source: 'grok_model_quota', model: 'grok-4', until: FUTURE },
      { source: 'model_runtime_block', model: 'grok-4-fast', until: PAST }
    ])

    const matrix = buildModelAvailabilityMatrix(
      [{ account, diagnosis, models: ['grok-4', 'grok-4-fast'] }],
      NOW
    )

    expect(cellOf(matrix, 9, 'grok-4').state).toBe('limited')
    expect(cellOf(matrix, 9, 'grok-4').details[0].source).toBe('grok_model_quota')
    expect(cellOf(matrix, 9, 'grok-4-fast').state).toBe('ok')
  })

  it('flags account-wide blocks separately from model-level ones', () => {
    const blocked = makeAccount({ id: 3, name: 'blocked', schedulable: false })
    const healthy = makeAccount({ id: 4, name: 'healthy' })
    const diagnosis = makeDiagnosis(
      [{ source: 'manual_unschedulable' }, { source: 'runtime_block', until: FUTURE }],
      false
    )

    const matrix = buildModelAvailabilityMatrix(
      [
        { account: healthy, models: ['m1'] },
        { account: blocked, diagnosis, models: ['m1'] }
      ],
      NOW
    )

    // 整号不可调度排最前，避免被淹没在格子里
    expect(matrix.rows[0].id).toBe(3)
    expect(matrix.rows[0].accountBlocked).toBe(true)
    expect(matrix.rows[0].accountBlocks.map((block) => block.source).sort()).toEqual([
      'manual_unschedulable',
      'runtime_block'
    ])
    expect(matrix.rows[1].accountBlocked).toBe(false)
    expect(matrix.summary.blockedAccountCount).toBe(1)
  })

  it('falls back to account fields when diagnostics are unavailable', () => {
    const matrix = buildModelAvailabilityMatrix(
      [
        { account: makeAccount({ id: 11, name: 'rate', rate_limit_reset_at: FUTURE }), models: ['m1'] },
        { account: makeAccount({ id: 12, name: 'stopped', status: 'error' }), models: ['m1'] },
        {
          account: makeAccount({ id: 13, name: 'expired', auto_pause_on_expired: true, expires_at: 1000 }),
          models: ['m1']
        },
        {
          // 没开「过期自动暂停」的账号，凭证过期不等于停调，不能误判
          account: makeAccount({ id: 14, name: 'lazy-refresh', expires_at: 1000 }),
          models: ['m1']
        }
      ],
      NOW
    )

    const byId = new Map(matrix.rows.map((row) => [row.id, row]))
    expect(byId.get(11)!.accountBlocks[0].source).toBe('rate_limited')
    expect(byId.get(12)!.accountBlocks[0].source).toBe('status')
    expect(byId.get(13)!.accountBlocks[0].source).toBe('expired')
    expect(byId.get(14)!.accountBlocked).toBe(false)
    expect(matrix.summary.blockedAccountCount).toBe(3)
  })

  it('ignores expired account-level blocks reported by diagnostics', () => {
    const diagnosis = makeDiagnosis([{ source: 'rate_limited', until: PAST }], true)
    const matrix = buildModelAvailabilityMatrix(
      [{ account: makeAccount({ id: 15, name: 'recovered' }), diagnosis, models: ['m1'] }],
      NOW
    )

    expect(matrix.rows[0].accountBlocked).toBe(false)
    expect(matrix.summary.blockedAccountCount).toBe(0)
  })

  it('keeps model cells unknown when the account model list could not be fetched', () => {
    const matrix = buildModelAvailabilityMatrix(
      [
        { account: makeAccount({ id: 21, name: 'known' }), models: ['m1'] },
        { account: makeAccount({ id: 22, name: 'unknown' }) }
      ],
      NOW
    )

    expect(cellOf(matrix, 22, 'm1').state).toBe('unknown')
    expect(matrix.rows.find((row) => row.id === 22)!.modelsUnavailable).toBe(true)
    expect(matrix.rows.find((row) => row.id === 21)!.modelsUnavailable).toBe(false)
  })

  it('adds rate-limited scopes as columns even when no account maps them', () => {
    const account = makeAccount({
      id: 31,
      name: 'credits',
      platform: 'antigravity',
      extra: {
        model_rate_limits: {
          AICredits: { rate_limited_at: PAST, rate_limit_reset_at: FUTURE }
        }
      }
    })

    const matrix = buildModelAvailabilityMatrix([{ account, models: ['gemini-3-pro'] }], NOW)

    expect(matrix.columns.map((column) => column.model)).toContain('AICredits')
    // 有问题的模型排在前面
    expect(matrix.columns[0].model).toBe('AICredits')
    expect(matrix.columns[0].labelKey).toBe('admin.accounts.modelAvailability.pseudoModels.aiCredits')
    expect(cellOf(matrix, 31, 'AICredits').state).toBe('limited')
  })

  it('counts limited combos across accounts for each column', () => {
    const limited = (id: number, name: string) =>
      makeAccount({
        id,
        name,
        extra: {
          model_rate_limits: { shared: { rate_limited_at: PAST, rate_limit_reset_at: FUTURE } }
        }
      })

    const matrix = buildModelAvailabilityMatrix(
      [
        { account: limited(41, 'a'), models: ['shared'] },
        { account: limited(42, 'b'), models: ['shared'] },
        { account: makeAccount({ id: 43, name: 'c' }), models: ['shared'] }
      ],
      NOW
    )

    expect(matrix.columns[0].limitedCount).toBe(2)
    expect(matrix.summary.limitedCellCount).toBe(2)
    expect(matrix.summary.accountCount).toBe(3)
  })
})

describe('filterIssuesOnly', () => {
  it('keeps only problematic rows/columns but preserves the summary', () => {
    const limitedAccount = makeAccount({
      id: 51,
      name: 'limited',
      extra: { model_rate_limits: { m2: { rate_limited_at: PAST, rate_limit_reset_at: FUTURE } } }
    })
    const matrix = buildModelAvailabilityMatrix(
      [
        { account: limitedAccount, models: ['m1', 'm2'] },
        { account: makeAccount({ id: 52, name: 'fine' }), models: ['m1', 'm2'] }
      ],
      NOW
    )

    const filtered = filterIssuesOnly(matrix)

    expect(filtered.rows.map((row) => row.id)).toEqual([51])
    expect(filtered.columns.map((column) => column.model)).toEqual(['m2'])
    expect(filtered.rows[0].cells).toHaveLength(1)
    expect(filtered.rows[0].cells[0].state).toBe('limited')
    // 汇总条描述的是整体情况，不随视图收窄而变小
    expect(filtered.summary).toEqual(matrix.summary)
  })
})

describe('resolveReason', () => {
  it('translates known reasons', () => {
    expect(resolveReason('openai_dedicated_quota_pool_rate_limited')).toEqual({
      i18nKey: 'admin.accounts.modelAvailability.reasons.openaiDedicatedQuotaPool',
      raw: null
    })
    expect(resolveReason('upstream_400_codex_plan_gated_model').i18nKey).toBe(
      'admin.accounts.modelAvailability.reasons.codexPlanGatedModel'
    )
    expect(resolveReason('upstream_404_model_not_found').i18nKey).toBe(
      'admin.accounts.modelAvailability.reasons.upstreamModelNotFound'
    )
    expect(resolveReason('upstream_model_not_found').i18nKey).toBe(
      'admin.accounts.modelAvailability.reasons.upstreamModelNotFound'
    )
  })

  it('shows unknown reasons verbatim', () => {
    expect(resolveReason('some_brand_new_backend_reason')).toEqual({
      i18nKey: null,
      raw: 'some_brand_new_backend_reason'
    })
  })

  it('extracts error_message from JSON reasons written by temp-unschedulable rules', () => {
    const payload = JSON.stringify({ until_unix: 1, status_code: 429, error_message: 'usage limit reached' })
    expect(resolveReason(payload)).toEqual({ i18nKey: null, raw: 'usage limit reached' })
  })

  it('keeps malformed JSON reasons verbatim instead of swallowing them', () => {
    expect(resolveReason('{not json').raw).toBe('{not json')
    expect(resolveReason('  ').raw).toBeNull()
    expect(resolveReason(null)).toEqual({ i18nKey: null, raw: null })
  })
})

describe('modelLabelKey', () => {
  it('maps pseudo scopes to human labels and leaves real models alone', () => {
    expect(modelLabelKey('openai:image_generation')).toBe(
      'admin.accounts.modelAvailability.pseudoModels.imageGeneration'
    )
    expect(modelLabelKey('gpt-5.3-codex')).toBeNull()
  })
})

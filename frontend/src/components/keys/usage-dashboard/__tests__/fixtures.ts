import type {
  KeyUsageDashboardKey,
  KeyUsageDashboardResponse,
  KeyUsageMetric
} from '@/api/keyUsageDashboard'

export function metric(cost = 0, requests = 0, tokens = 0): KeyUsageMetric {
  return { cost, requests, tokens }
}

export function createDashboardKey(
  id: number,
  overrides: Partial<KeyUsageDashboardKey> = {}
): KeyUsageDashboardKey {
  const totals = overrides.totals || metric(id / 10, id, id * 100)
  return {
    api_key_id: id,
    name: `key-${id}`,
    group_name: 'default',
    status: 'active',
    last_used_at: null,
    totals,
    cost_per_request: totals.requests > 0 ? totals.cost / totals.requests : 0,
    cache_hit_rate: 0.4,
    avg_first_token_ms: 800,
    error_count: 0,
    error_rate: 0,
    peak_rpm: 4,
    hour_of_day: Array.from({ length: 24 }, (_, hour) => metric(
      hour === 11 ? totals.cost : totals.cost / 24,
      hour === 11 ? totals.requests : Math.ceil(totals.requests / 24),
      hour === 11 ? totals.tokens : Math.ceil(totals.tokens / 24)
    )),
    series: Array.from({ length: 7 }, () => metric(
      totals.cost / 7,
      Math.ceil(totals.requests / 7),
      Math.ceil(totals.tokens / 7)
    )),
    ...overrides
  }
}

export function createDashboardFixture(
  overrides: Partial<KeyUsageDashboardResponse> = {}
): KeyUsageDashboardResponse {
  const keys = overrides.keys || [
    createDashboardKey(1),
    createDashboardKey(2),
    createDashboardKey(3)
  ]
  const total = keys.reduce(
    (sum, key) => metric(
      sum.cost + key.totals.cost,
      sum.requests + key.totals.requests,
      sum.tokens + key.totals.tokens
    ),
    metric()
  )

  return {
    range: '7d',
    start: '2026-10-31T00:00:00+08:00',
    end: '2026-11-06T16:00:00+08:00',
    totals: total,
    prev_totals: metric(total.cost * 0.8, total.requests * 0.9, total.tokens * 0.85),
    active_keys: keys.filter((key) => key.totals.requests > 0).length,
    total_keys: keys.length,
    avg_cost_per_request: total.requests > 0 ? total.cost / total.requests : 0,
    prev_avg_cost_per_request: 0.12,
    cache_hit_rate: 0.5,
    prev_cache_hit_rate: 0.45,
    error_rate: 0.03,
    prev_error_rate: 0.02,
    peak_hours: [
      { hour: 11, requests: 40 },
      { hour: 15, requests: 25 },
      { hour: 9, requests: 20 }
    ],
    alerts: [],
    keys,
    series_labels: ['10/31', '11/01', '11/02', '11/03', '11/04', '11/05', '11/06'],
    week: Array.from({ length: 7 }, (_, day) =>
      Array.from({ length: 24 }, (_, hour) => metric(day, day + hour + 1, day * 100))
    ),
    models: {
      names: ['gpt-5', 'claude-sonnet'],
      keys: keys.map((key) => ({
        api_key_id: key.api_key_id,
        cells: [
          metric(key.totals.cost * 0.7, key.totals.requests * 0.7, key.totals.tokens * 0.7),
          metric(key.totals.cost * 0.3, key.totals.requests * 0.3, key.totals.tokens * 0.3)
        ]
      }))
    },
    ...overrides
  }
}

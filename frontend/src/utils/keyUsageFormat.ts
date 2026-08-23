/**
 * Formatting helpers shared by the public Key Usage page and its sub-components.
 * Kept deliberately locale-independent (plain `toLocaleString` / fixed 2 decimals)
 * so the numbers match the rest of that page.
 */

import type { KeyUsageMetric, KeyUsageRankingEntry } from '@/api/keyUsage'

export function formatUsd(value: number | null | undefined): string {
  if (value == null || value < 0) return '-'
  return '$' + Number(value).toFixed(2)
}

export function formatCount(value: number | null | undefined): string {
  if (value == null) return '-'
  return Number(value).toLocaleString()
}

/** The value a ranking row is currently sorted by. */
export function metricValueOf(
  entry: Pick<KeyUsageRankingEntry, 'requests' | 'tokens' | 'cost_usd'> | null | undefined,
  metric: KeyUsageMetric
): string {
  if (!entry) return '-'
  if (metric === 'requests') return formatCount(entry.requests)
  if (metric === 'tokens') return formatCount(entry.tokens)
  return formatUsd(entry.cost_usd)
}

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

/**
 * Counts (tokens and requests) only start being abbreviated at a million.
 * Below that the raw grouped number is short enough to read and strictly more useful.
 */
const COUNT_ABBREVIATION_FLOOR = 1_000_000

/**
 * Largest unit first. `M`/`B` only — deliberately not `K`: five-digit and six-digit
 * counts are perfectly readable, and abbreviating them just throws away precision.
 */
const COUNT_UNITS: ReadonlyArray<{ limit: number; suffix: string }> = [
  { limit: 1_000_000_000, suffix: 'B' },
  { limit: 1_000_000, suffix: 'M' },
]

/**
 * Three significant digits, so the rendered width stays put as the magnitude grows:
 * `1.23M` / `12.3M` / `123M`.
 */
function abbreviationDecimals(magnitude: number): number {
  if (magnitude >= 100) return 0
  if (magnitude >= 10) return 1
  return 2
}

/**
 * Display form of a count: `1.23B` / `12.3M` / `123M`, or the plain grouped number
 * below a million. Negatives and zero keep their exact form.
 *
 * The exact value is never lost — call `formatCountExact` for the `title` attribute
 * so hovering an abbreviated number reveals every digit.
 */
export function formatCount(value: number | null | undefined): string {
  if (value == null) return '-'
  const num = Number(value)
  if (!Number.isFinite(num)) return '-'

  const magnitude = Math.abs(num)
  if (magnitude >= COUNT_ABBREVIATION_FLOOR) {
    for (const unit of COUNT_UNITS) {
      const scaled = num / unit.limit
      const text = scaled.toFixed(abbreviationDecimals(Math.abs(scaled)))
      // `>= 1` both selects the right unit (0.99M rounds out of the B bucket) and
      // promotes on rounding overflow: 999,999,999 renders as `1.00B`, never `1000M`.
      if (Math.abs(Number(text)) >= 1) return text + unit.suffix
    }
  }
  return num.toLocaleString()
}

/**
 * The full grouped number, always — what goes into `title` next to an abbreviated
 * count. The station owner asked for the digits to remain reachable, so every
 * abbreviated figure on the page must be hoverable back to its exact value.
 */
export function formatCountExact(value: number | null | undefined): string {
  if (value == null) return '-'
  const num = Number(value)
  if (!Number.isFinite(num)) return '-'
  return num.toLocaleString()
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

/**
 * Tooltip counterpart of `metricValueOf`. Cost is already exact on screen
 * (`$x.xx`), so only the two count metrics get an exact-value tooltip.
 */
export function metricTitleOf(
  entry: Pick<KeyUsageRankingEntry, 'requests' | 'tokens' | 'cost_usd'> | null | undefined,
  metric: KeyUsageMetric
): string | undefined {
  if (!entry) return undefined
  if (metric === 'requests') return formatCountExact(entry.requests)
  if (metric === 'tokens') return formatCountExact(entry.tokens)
  return undefined
}

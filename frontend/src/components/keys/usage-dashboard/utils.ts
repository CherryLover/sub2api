import type {
  KeyUsageDashboardMetric,
  KeyUsageMetric
} from '@/api/keyUsageDashboard'

export const KEY_USAGE_SERIES_COLORS = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#8b5cf6',
  '#ec4899'
] as const

export const KEY_USAGE_OTHER_COLOR = '#94a3b8'

export const HEATMAP_COLORS = [
  'bg-gray-100 dark:bg-dark-700',
  'bg-primary-100 dark:bg-primary-950',
  'bg-primary-200 dark:bg-primary-900',
  'bg-primary-300 dark:bg-primary-800',
  'bg-primary-400 dark:bg-primary-700',
  'bg-primary-500 dark:bg-primary-600',
  'bg-primary-600 dark:bg-primary-500',
  'bg-primary-700 dark:bg-primary-400',
  'bg-primary-800 dark:bg-primary-300'
] as const

export const MODEL_BAR_COLORS = [
  'bg-[#3b82f6]',
  'bg-[#10b981]',
  'bg-[#f59e0b]',
  'bg-[#ef4444]',
  'bg-[#8b5cf6]',
  'bg-[#ec4899]'
] as const

export const METRIC_LABEL_KEYS: Record<KeyUsageDashboardMetric, string> = {
  cost: 'keyUsageDashboard.metrics.cost',
  requests: 'keyUsageDashboard.metrics.requests',
  tokens: 'keyUsageDashboard.metrics.tokens'
}

export function toFiniteNumber(value: unknown): number {
  const numberValue = Number(value)
  return Number.isFinite(numberValue) ? numberValue : 0
}

export function metricValue(
  metric: KeyUsageMetric | null | undefined,
  selectedMetric: KeyUsageDashboardMetric
): number {
  if (!metric) return 0
  return toFiniteNumber(metric[selectedMetric])
}

export function formatCost(value: number): string {
  const safeValue = toFiniteNumber(value)
  if (safeValue >= 1_000) return `$${(safeValue / 1_000).toFixed(2)}K`
  if (safeValue >= 1) return `$${safeValue.toFixed(2)}`
  if (safeValue >= 0.01) return `$${safeValue.toFixed(3)}`
  return `$${safeValue.toFixed(4)}`
}

export function formatCompactNumber(value: number): string {
  const safeValue = toFiniteNumber(value)
  if (safeValue >= 1_000_000_000) return `${(safeValue / 1_000_000_000).toFixed(2)}B`
  if (safeValue >= 1_000_000) return `${(safeValue / 1_000_000).toFixed(2)}M`
  if (safeValue >= 1_000) return `${(safeValue / 1_000).toFixed(1)}K`
  return safeValue.toLocaleString()
}

export function formatPercent(value: number, digits = 1): string {
  return `${(toFiniteNumber(value) * 100).toFixed(digits)}%`
}

export function formatLatency(value: number): string {
  const safeValue = toFiniteNumber(value)
  if (safeValue <= 0) return '—'
  if (safeValue >= 1_000) return `${(safeValue / 1_000).toFixed(safeValue >= 10_000 ? 0 : 1)}s`
  return `${Math.round(safeValue)}ms`
}

export function formatBytes(value: number): string {
  const safeValue = toFiniteNumber(value)
  if (safeValue >= 1024 * 1024) return `${(safeValue / (1024 * 1024)).toFixed(1)} MB`
  if (safeValue >= 1024) return `${(safeValue / 1024).toFixed(1)} KB`
  return `${Math.round(safeValue)} B`
}

export function heatmapColorIndex(value: number, maxValue: number): number {
  const safeValue = toFiniteNumber(value)
  const safeMax = toFiniteNumber(maxValue)
  if (safeValue <= 0 || safeMax <= 0) return 0
  return Math.min(8, Math.max(1, Math.ceil((safeValue / safeMax) * 8)))
}

export function heatmapClass(value: number, maxValue: number): string {
  return HEATMAP_COLORS[heatmapColorIndex(value, maxValue)]
}

export function metricDisplay(value: number, metric: KeyUsageDashboardMetric): string {
  if (metric === 'cost') return formatCost(value)
  if (metric === 'tokens') return formatCompactNumber(value)
  return Math.round(value).toLocaleString()
}

import type { Account } from '@/types'

export type CodexCreditsMode = 'on' | 'first' | 'last' | 'off'

export interface OpenAICreditsInfo {
  hasData: boolean
  hasCredits: boolean
  unlimited: boolean
  overageReached: boolean
  balance: number
  peakBalance: number
  spent: number
  remainingPct: number
  spentPct: number
  mode: CodexCreditsMode
  updatedAt: string | null
  usable: boolean
}

export function normalizeCodexCreditsMode(raw: unknown): CodexCreditsMode {
  if (typeof raw !== 'string') return 'on'
  switch (raw.trim().toLowerCase()) {
    case 'first':
      return 'first'
    case 'last':
      return 'last'
    case 'off':
      return 'off'
    default:
      return 'on'
  }
}

function readExtraBool(extra: Record<string, unknown> | undefined, key: string): boolean {
  if (!extra) return false
  const val = extra[key]
  if (typeof val === 'boolean') return val
  if (typeof val === 'string') return val.trim().toLowerCase() === 'true'
  if (typeof val === 'number') return val !== 0
  return false
}

function readExtraNumber(extra: Record<string, unknown> | undefined, key: string): number | null {
  if (!extra) return null
  const val = extra[key]
  if (typeof val === 'number' && Number.isFinite(val)) return val
  if (typeof val === 'string' && val.trim() !== '') {
    const parsed = Number(val.trim())
    if (Number.isFinite(parsed)) return parsed
  }
  return null
}

/**
 * Extract OpenAI Codex credits balance, peak/spent tracking, and routing policy from account.extra.
 */
export function getOpenAICreditsInfo(account: Pick<Account, 'platform' | 'type' | 'extra'>): OpenAICreditsInfo {
  const extra = account.extra as Record<string, unknown> | undefined
  const mode = normalizeCodexCreditsMode(extra?.codex_credits_mode)
  const hasCredits = readExtraBool(extra, 'codex_credits_has')
  const unlimited = readExtraBool(extra, 'codex_credits_unlimited')
  const overageReached = readExtraBool(extra, 'codex_credits_overage_reached')
  const rawBalance = readExtraNumber(extra, 'codex_credits_balance')
  const rawPeak = readExtraNumber(extra, 'codex_credits_peak_balance')
  const updatedAt = typeof extra?.codex_credits_updated_at === 'string' ? extra.codex_credits_updated_at : null

  const hasData =
    extra?.codex_credits_has !== undefined ||
    extra?.codex_credits_unlimited !== undefined ||
    rawBalance !== null

  const balance = Math.max(0, rawBalance ?? 0)
  const peakBalance = Math.max(balance, rawPeak ?? balance)
  const spent = Math.max(0, peakBalance - balance)

  let remainingPct = 0
  let spentPct = 0
  if (unlimited) {
    remainingPct = 100
    spentPct = 0
  } else if (peakBalance > 0) {
    remainingPct = Math.min(100, Math.max(0, (balance / peakBalance) * 100))
    spentPct = Math.min(100, Math.max(0, (spent / peakBalance) * 100))
  }

  const usable = unlimited || (hasCredits && balance > 0)

  return {
    hasData,
    hasCredits,
    unlimited,
    overageReached,
    balance,
    peakBalance,
    spent,
    remainingPct,
    spentPct,
    mode,
    updatedAt,
    usable,
  }
}

export function formatCreditsAmount(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0'
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 10_000) return `${(value / 1_000).toFixed(2)}K`
  if (value >= 100) return value.toFixed(1).replace(/\.0$/, '')
  return value.toFixed(2).replace(/\.?0+$/, '')
}

export function getCodexCreditsModeBadgeClass(mode: CodexCreditsMode): string {
  switch (mode) {
    case 'first':
      return 'bg-emerald-500/10 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
    case 'last':
      return 'bg-amber-500/10 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
    case 'off':
      return 'bg-rose-500/10 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300'
    default:
      return 'bg-teal-500/10 text-teal-700 dark:bg-teal-500/15 dark:text-teal-300'
  }
}

// 调度可视化的纯函数：原因代码 → 中文文案、漏斗计数、选号 chip 文案、怎么修提示。
// 组件只负责渲染，所有判断集中在这里，便于单测。
import type {
  RoutingAttempt,
  RoutingCandidate,
  RoutingDiagnosis,
  RoutingGate,
  RoutingInfo
} from '@/types'

type Translate = (key: string, params?: Record<string, unknown>) => string

/** 所有已知原因代码（后端 filterStats.exclude(...) 与 OpsNoAccountReason*） */
export const ROUTING_REASON_CODES = [
  'model_not_supported',
  'model_rate_limited',
  'excluded',
  'not_schedulable',
  'empty_pool',
  'unknown',
  'platform_mismatch',
  'runtime_blocked',
  'privacy_not_set',
  'transport_incompatible',
  'ineligible',
  'channel_restricted',
  'channel_upstream_restricted',
  'compact_unsupported',
  'shadow_parent_unhealthy',
  'account_nil',
  'quota_auto_pause',
  'capability_mismatch',
  'proxy_stream_quarantined',
  'profit_threshold',
  'profit_invalid_account_rate',
  'grok_free_quota_soft_gate',
  'grok_team_model_rate_limit',
  'grok_model_quota_block',
  'selection_order_empty',
  'selection_order_exhausted'
] as const

const KNOWN_REASONS = new Set<string>(ROUTING_REASON_CODES)

/** 关卡检查顺序（与后端关卡顺序一致） */
export const ROUTING_GATES: RoutingGate[] = ['platform', 'schedulable', 'quota', 'model', 'capacity']

/** 这些原因靠等待不会好，需要改配置 */
const CONFIG_REASONS = new Set<string>([
  'model_not_supported',
  'platform_mismatch',
  'privacy_not_set',
  'transport_incompatible',
  'channel_restricted',
  'channel_upstream_restricted',
  'compact_unsupported',
  'capability_mismatch'
])

function reasonKey(
  t: Translate,
  ns: 'reasons' | 'candidateReasons',
  code: string | null | undefined,
  params: Record<string, unknown> = {}
): string {
  const raw = String(code || '').trim()
  if (!raw) return ''
  const quotaWindow = /^quota_auto_pause_(.+)$/.exec(raw)
  if (quotaWindow) {
    return t(`admin.routing.${ns}.quota_auto_pause_window`, { ...params, window: quotaWindow[1] })
  }
  if (!KNOWN_REASONS.has(raw)) return raw
  return t(`admin.routing.${ns}.${raw}`, params)
}

/** 整条请求的原因文案（原因列 chip） */
export function routingReasonLabel(t: Translate, code: string | null | undefined): string {
  return reasonKey(t, 'reasons', code)
}

/** 单个账号「为什么没选它」 */
export function candidateReasonLabel(
  t: Translate,
  code: string | null | undefined,
  model?: string | null
): string {
  return reasonKey(t, 'candidateReasons', code, { model: model || '-' })
}

/** 原因 chip 配色：配置类偏琥珀，暂时性偏橙，其它偏红 */
export function routingReasonClass(code: string | null | undefined): string {
  const raw = String(code || '')
  if (CONFIG_REASONS.has(raw) || raw === 'empty_pool') {
    return 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200'
  }
  if (raw === 'not_schedulable' || raw === 'model_rate_limited' || raw.startsWith('quota_auto_pause')) {
    return 'bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-200'
  }
  return 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-200'
}

export function isConfigReason(code: string | null | undefined): boolean {
  return CONFIG_REASONS.has(String(code || ''))
}

export function diagnosisCandidates(diag: RoutingDiagnosis | null | undefined): RoutingCandidate[] {
  return Array.isArray(diag?.candidates) ? diag!.candidates! : []
}

export function diagnosisAttempts(diag: { attempts?: RoutingAttempt[] | null } | null | undefined): RoutingAttempt[] {
  return Array.isArray(diag?.attempts) ? diag!.attempts! : []
}

/** 候选账号列：「N 个候选 · M 个可用」或有尝试时「N 个候选 · M 个试过」 */
export function candidatesSummaryLabel(t: Translate, diag: RoutingDiagnosis | null | undefined): string {
  if (!diag) return ''
  const candidates = diagnosisCandidates(diag)
  const total = candidates.length || diag.pool_size || 0
  if (total === 0) return t('admin.routing.candidatesNone')
  const attempts = diagnosisAttempts(diag)
  if (attempts.length > 0) {
    const tried = new Set(attempts.map((a) => a.account_id)).size
    return t('admin.routing.candidatesTried', { total, tried })
  }
  const usable = candidates.filter((c) => !c.reason).length
  return t('admin.routing.candidatesSummary', { total, usable })
}

export interface FunnelStep {
  gate: RoutingGate
  count: number
}

/**
 * 漏斗：每一关通过的账号数。只列出至少有一个账号被检查过的关卡；
 * 某关被检查过意味着前面的关都通过了，所以直接数 checks[gate] === true。
 */
export function diagnosisFunnel(diag: RoutingDiagnosis | null | undefined): FunnelStep[] {
  const candidates = diagnosisCandidates(diag)
  const steps: FunnelStep[] = []
  for (const gate of ROUTING_GATES) {
    const evaluated = candidates.some((c) => c.checks && gate in c.checks)
    if (!evaluated) continue
    steps.push({ gate, count: candidates.filter((c) => c.checks?.[gate] === true).length })
  }
  return steps
}

/** 关卡格子：'pass' ✓ / 'fail' ✕ / 'skip' —（没检查到） */
export function gateState(candidate: RoutingCandidate, gate: RoutingGate): 'pass' | 'fail' | 'skip' {
  const v = candidate.checks?.[gate]
  if (v === true) return 'pass'
  if (v === false) return 'fail'
  return 'skip'
}

/** 出现最多的候选原因；诊断自带 reason 时以它为准 */
export function dominantReason(diag: RoutingDiagnosis | null | undefined): string {
  if (!diag) return ''
  if (diag.reason) return diag.reason
  const candidates = diagnosisCandidates(diag)
  if (candidates.length === 0) return 'empty_pool'
  const counts = new Map<string, number>()
  for (const c of candidates) {
    if (!c.reason) continue
    counts.set(c.reason, (counts.get(c.reason) ?? 0) + 1)
  }
  let best = ''
  let bestCount = 0
  for (const [reason, count] of counts) {
    if (count > bestCount) {
      best = reason
      bestCount = count
    }
  }
  return best
}

/** 候选里最早的恢复时间 */
export function earliestRecoverAt(diag: RoutingDiagnosis | null | undefined): string | null {
  let best: string | null = null
  let bestMs = Infinity
  for (const c of diagnosisCandidates(diag)) {
    if (!c.recover_at) continue
    const ms = Date.parse(c.recover_at)
    if (Number.isFinite(ms) && ms < bestMs) {
      bestMs = ms
      best = c.recover_at
    }
  }
  return best
}

export type FixAction = 'accounts' | 'groups'

export interface FixHint {
  text: string
  action: FixAction
}

/** 根据主要原因给出「怎么修」 */
export function diagnosisFixHint(
  t: Translate,
  diag: RoutingDiagnosis | null | undefined,
  formatTime: (iso: string) => string
): FixHint | null {
  if (!diag) return null
  const reason = dominantReason(diag)
  const model = diag.model || '-'
  if (reason === 'model_not_supported') {
    return { text: t('admin.routing.fix.model_not_supported', { model }), action: 'accounts' }
  }
  if (reason === 'empty_pool' || diagnosisCandidates(diag).length === 0) {
    return { text: t('admin.routing.fix.empty_pool'), action: 'groups' }
  }
  if (reason === 'not_schedulable') {
    const recover = earliestRecoverAt(diag)
    if (recover) {
      return { text: t('admin.routing.fix.not_schedulable_recover', { time: formatTime(recover) }), action: 'groups' }
    }
    return { text: t('admin.routing.fix.not_schedulable'), action: 'accounts' }
  }
  if (reason === 'platform_mismatch') return { text: t('admin.routing.fix.platform_mismatch'), action: 'groups' }
  if (reason === 'model_rate_limited') return { text: t('admin.routing.fix.model_rate_limited'), action: 'groups' }
  if (reason.startsWith('quota_auto_pause')) return { text: t('admin.routing.fix.quota_auto_pause'), action: 'groups' }
  if (reason === 'capability_mismatch') return { text: t('admin.routing.fix.capability_mismatch'), action: 'accounts' }
  if (reason === 'profit_threshold' || reason === 'profit_invalid_account_rate') {
    return { text: t('admin.routing.fix.profit_control'), action: 'groups' }
  }
  if (reason === 'grok_free_quota_soft_gate' || reason === 'grok_model_quota_block') {
    return { text: t('admin.routing.fix.grok_quota'), action: 'groups' }
  }
  if (reason === 'grok_team_model_rate_limit') {
    return { text: t('admin.routing.fix.grok_team_rate_limit'), action: 'groups' }
  }
  if (reason === 'selection_order_empty' || reason === 'selection_order_exhausted') {
    return { text: t('admin.routing.fix.selection_order'), action: 'accounts' }
  }
  if (reason === 'excluded') return { text: t('admin.routing.fix.excluded'), action: 'accounts' }
  return { text: t('admin.routing.fix.default'), action: 'accounts' }
}

// ==================== 成功请求：选号 chip ====================

/** 账号切换次数：相邻两次尝试的账号不同记一次 */
export function countSwitches(attempts: RoutingAttempt[]): number {
  let n = 0
  for (let i = 1; i < attempts.length; i++) {
    if (attempts[i].account_id !== attempts[i - 1].account_id) n++
  }
  return n
}

export function formatSeconds(ms: number | null | undefined): string {
  return ((ms ?? 0) / 1000).toFixed(1)
}

/** 恢复时间：MM/DD HH:mm（本地时区） */
export function formatRecoverTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}/${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

/** 尝试耗时：不到 1 秒显示毫秒，否则显示一位小数的秒 */
export function formatAttemptDuration(ms: number | null | undefined): string {
  const v = ms ?? 0
  return v < 1000 ? `${v}ms` : `${formatSeconds(v)}s`
}

export type RoutingChipKind = 'load_balance' | 'sticky' | 'single' | 'switched' | 'retried' | 'queued'

export interface RoutingChip {
  kind: RoutingChipKind
  label: string
  className: string
}

const CHIP_CLASS: Record<RoutingChipKind, string> = {
  load_balance: 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300',
  single: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
  sticky: 'bg-sky-50 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
  switched: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  retried: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  queued: 'bg-violet-50 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300'
}

/** 选号列 chip：切换/重试优先，其次排队，最后按选号方式 */
export function routingChip(t: Translate, info: RoutingInfo | null | undefined): RoutingChip | null {
  if (!info) return null
  const attempts = diagnosisAttempts(info)
  const switches = countSwitches(attempts)
  if (switches > 0) {
    return { kind: 'switched', label: t('admin.routing.usage.chip.switched', { count: switches }), className: CHIP_CLASS.switched }
  }
  if (attempts.length > 1) {
    return { kind: 'retried', label: t('admin.routing.usage.chip.retried', { count: attempts.length - 1 }), className: CHIP_CLASS.retried }
  }
  const count = info.candidate_count || (Array.isArray(info.candidates) ? info.candidates.length : 0)
  let base: string
  let kind: RoutingChipKind
  if (info.method === 'sticky') {
    kind = 'sticky'
    base = t('admin.routing.usage.chip.sticky')
  } else if (info.method === 'single' || count <= 1) {
    kind = 'single'
    base = t('admin.routing.usage.chip.single')
  } else {
    kind = 'load_balance'
    base = t('admin.routing.usage.chip.load_balance', { count })
  }
  if ((info.queue_wait_ms ?? 0) > 0) {
    const queued = t('admin.routing.usage.chip.queued', { seconds: formatSeconds(info.queue_wait_ms) })
    // 排队时按选号方式取前缀：负载均衡下即使只剩一个能接的账号也写「负载均衡」
    const prefix = info.method === 'load_balance' ? t('admin.routing.methodShort.load_balance') : base
    return { kind: 'queued', label: `${prefix} · ${queued}`, className: CHIP_CLASS.queued }
  }
  return { kind, label: base, className: CHIP_CLASS[kind] }
}

/** 候选按负载从低到高；选中的排最前，被跳过的排最后 */
export function sortCandidatesByLoad(candidates: RoutingCandidate[] | null | undefined): RoutingCandidate[] {
  const list = Array.isArray(candidates) ? [...candidates] : []
  const ratio = (c: RoutingCandidate) => (c.load_max > 0 ? c.load_current / c.load_max : c.load_current)
  const rank = (c: RoutingCandidate) => (c.chosen ? 0 : c.reason ? 2 : 1)
  return list.sort((a, b) => rank(a) - rank(b) || ratio(a) - ratio(b))
}

export function formatLoad(c: RoutingCandidate): string {
  return c.load_max > 0 ? `${c.load_current}/${c.load_max}` : String(c.load_current ?? 0)
}

/** 「为什么是它」一句话 */
export function routingWhy(t: Translate, info: RoutingInfo | null | undefined, chosenName: string): string {
  if (!info) return ''
  const attempts = diagnosisAttempts(info)
  const candidates = Array.isArray(info.candidates) ? info.candidates : []
  const chosen = candidates.find((c) => c.chosen)
  const name = chosen?.account_name || chosenName || '-'
  const parts: string[] = []
  if (countSwitches(attempts) > 0) {
    parts.push(t('admin.routing.usage.whySwitched', { name: attempts[attempts.length - 1].account_name || name }))
  } else if (info.method === 'sticky') {
    parts.push(t('admin.routing.usage.whySticky', { name }))
  } else if (info.method === 'single' || (info.candidate_count || candidates.length) <= 1) {
    parts.push(t('admin.routing.usage.whySingle', { name }))
  } else {
    const count = info.candidate_count || candidates.length
    if (chosen && candidates.length > 0) {
      const usable = candidates.filter((c) => !c.reason).length
      parts.push(t('admin.routing.usage.whyLoadBalance', { count, usable, name, load: formatLoad(chosen) }))
    } else {
      parts.push(t('admin.routing.usage.whyLoadBalanceNoLoad', { count, name }))
    }
  }
  if ((info.queue_wait_ms ?? 0) > 0) {
    parts.push(t('admin.routing.usage.whyQueued', { seconds: formatSeconds(info.queue_wait_ms) }))
  }
  return parts.join(' ')
}

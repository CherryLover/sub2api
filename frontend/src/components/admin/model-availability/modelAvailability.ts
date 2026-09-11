/**
 * 「账号 × 模型」可用性矩阵的纯计算逻辑。
 *
 * 之所以把它单独抽成一个模块：矩阵的判定规则（限流是否已过期、整号不可调度是否
 * 压过单模型限流、原因怎么翻译）是这个页面最容易出错的部分，必须能脱离 DOM 单测。
 *
 * 数据来源（全部是现成接口，本页不新增后端）：
 *   1. GET  /admin/accounts                      → extra.model_rate_limits（落库的模型级限流）
 *   2. POST /admin/accounts/diagnostics/batch    → scheduling.blocks（进程内封锁，含模型级）
 *   3. GET  /admin/accounts/:id/models           → 每个账号按 model_mapping 支持的模型清单
 */
import type {
  Account,
  AccountDiagnosis,
  AccountSchedulingBlock,
  AccountSchedulingBlockSource
} from '@/types'

/** 诊断结果里「只影响单个模型」的封锁来源，其余来源都按整号不可调度处理。 */
export const MODEL_LEVEL_BLOCK_SOURCES: ReadonlySet<AccountSchedulingBlockSource> = new Set([
  'model_runtime_block',
  'grok_model_quota',
  'grok_team_rate_limit'
])

/** 库里存的 reason 原始值 → i18n key。命中不了的 reason 一律原样显示，不吞。 */
export const REASON_I18N_KEYS: Readonly<Record<string, string>> = {
  // internal/service/ratelimit_service.go
  openai_dedicated_quota_pool_rate_limited:
    'admin.accounts.modelAvailability.reasons.openaiDedicatedQuotaPool',
  upstream_400_codex_plan_gated_model: 'admin.accounts.modelAvailability.reasons.codexPlanGatedModel',
  upstream_404_model_not_found: 'admin.accounts.modelAvailability.reasons.upstreamModelNotFound',
  // 后端常量是 upstream_404_model_not_found，历史数据里也可能是不带状态码的写法。
  upstream_model_not_found: 'admin.accounts.modelAvailability.reasons.upstreamModelNotFound',
  openai_image_rate_limited: 'admin.accounts.modelAvailability.reasons.openaiImageRateLimited',
  openai_image_capability_lost: 'admin.accounts.modelAvailability.reasons.openaiImageCapabilityLost',
  // internal/service/openai_images_responses.go
  openai_images_oauth_tool_unavailable:
    'admin.accounts.modelAvailability.reasons.openaiImagesToolUnavailable',
  anthropic_7d_oi_window_exhausted: 'admin.accounts.modelAvailability.reasons.anthropicWindowExhausted',
  anthropic_fable_credits_required: 'admin.accounts.modelAvailability.reasons.anthropicFableCredits'
}

/**
 * model_rate_limits 里并不全是真模型名，还有几个「伪模型」作用域键。
 * 直接把 `openai:image_generation` 甩到列头上没人看得懂，这里给人话标签。
 */
export const PSEUDO_MODEL_I18N_KEYS: Readonly<Record<string, string>> = {
  AICredits: 'admin.accounts.modelAvailability.pseudoModels.aiCredits',
  'openai:image_generation': 'admin.accounts.modelAvailability.pseudoModels.imageGeneration'
}

/** 整号级封锁来源 → i18n key（模型级来源不会走到这里）。 */
export const ACCOUNT_BLOCK_I18N_KEYS: Readonly<Record<string, string>> = {
  status: 'admin.accounts.modelAvailability.accountBlocks.status',
  manual_unschedulable: 'admin.accounts.modelAvailability.accountBlocks.manualUnschedulable',
  expired: 'admin.accounts.modelAvailability.accountBlocks.expired',
  overloaded: 'admin.accounts.modelAvailability.accountBlocks.overloaded',
  rate_limited: 'admin.accounts.modelAvailability.accountBlocks.rateLimited',
  temp_unschedulable: 'admin.accounts.modelAvailability.accountBlocks.tempUnschedulable',
  quota_exceeded: 'admin.accounts.modelAvailability.accountBlocks.quotaExceeded',
  runtime_block: 'admin.accounts.modelAvailability.accountBlocks.runtimeBlock',
  proxy_quarantine: 'admin.accounts.modelAvailability.accountBlocks.proxyQuarantine',
  quota_auto_pause: 'admin.accounts.modelAvailability.accountBlocks.quotaAutoPause'
}

/** 模型级封锁来源 → i18n key。 */
export const MODEL_BLOCK_I18N_KEYS: Readonly<Record<string, string>> = {
  model_rate_limit: 'admin.accounts.modelAvailability.modelBlocks.modelRateLimit',
  model_runtime_block: 'admin.accounts.modelAvailability.modelBlocks.modelRuntimeBlock',
  grok_model_quota: 'admin.accounts.modelAvailability.modelBlocks.grokModelQuota',
  grok_team_rate_limit: 'admin.accounts.modelAvailability.modelBlocks.grokTeamRateLimit'
}

export type ModelBlockSource = 'model_rate_limit' | AccountSchedulingBlockSource

export interface ModelBlockDetail {
  source: ModelBlockSource
  /** null 表示没有解除时间（封锁持续到人工干预 / 下次探测） */
  until: string | null
  /** 库里存的原始 reason，未做翻译 */
  reason: string | null
}

export interface AccountBlockDetail {
  source: AccountSchedulingBlockSource
  until: string | null
}

export type MatrixCellState = 'ok' | 'limited' | 'unsupported' | 'unknown'

export interface MatrixCell {
  model: string
  state: MatrixCellState
  /** limited 时取所有生效封锁里最晚的解除时间 */
  until: string | null
  details: ModelBlockDetail[]
}

export interface MatrixRow {
  id: number
  name: string
  platform: string
  /** 整号不可调度：比单模型限流严重，页面上必须整行标出来 */
  accountBlocked: boolean
  accountBlocks: AccountBlockDetail[]
  /** 该账号被限流的模型数 */
  limitedModels: number
  /** 该账号的模型清单没取到（接口失败），单元格只能显示「未知」 */
  modelsUnavailable: boolean
  cells: MatrixCell[]
}

export interface MatrixColumn {
  model: string
  /** 伪模型作用域的人话标签 key；真模型为 null，直接显示模型名 */
  labelKey: string | null
  /** 有多少个账号在这个模型上被限流 */
  limitedCount: number
}

export interface MatrixSummary {
  accountCount: number
  modelCount: number
  blockedAccountCount: number
  limitedCellCount: number
}

export interface ModelAvailabilityMatrix {
  columns: MatrixColumn[]
  rows: MatrixRow[]
  summary: MatrixSummary
}

export interface ModelAvailabilityInput {
  account: Account
  diagnosis?: AccountDiagnosis | null
  /** GET /admin/accounts/:id/models 的结果；undefined 表示没取到 */
  models?: string[]
}

interface RawModelRateLimitEntry {
  rate_limited_at?: string
  rate_limit_reset_at?: string
  reason?: string
}

/** 目标时间是否仍在未来；空值 / 非法时间一律视为「已过期」。 */
export function isStillFuture(value: string | null | undefined, nowMs: number): boolean {
  if (!value) return false
  const ts = Date.parse(value)
  return Number.isFinite(ts) && ts > nowMs
}

export interface ResolvedReason {
  /** 命中翻译表时的 i18n key */
  i18nKey: string | null
  /** 没命中翻译表时要原样显示的文本 */
  raw: string | null
}

const EMPTY_REASON: ResolvedReason = { i18nKey: null, raw: null }

/**
 * 把库里的 reason 解析成「可翻译的 key」或「原样文本」。
 *
 * 临时停调 / 阈值暂停写进 reason 的是一整段 JSON（见 service.TempUnschedState），
 * 直接甩给运维看就是一坨转义字符串，这里抽出其中的 error_message；
 * 其余未知 reason 原样返回，绝不吞掉。
 */
export function resolveReason(raw: string | null | undefined): ResolvedReason {
  const value = (raw ?? '').trim()
  if (!value) return EMPTY_REASON

  const mapped = REASON_I18N_KEYS[value]
  if (mapped) return { i18nKey: mapped, raw: null }

  if (value.startsWith('{')) {
    try {
      const parsed = JSON.parse(value) as Record<string, unknown>
      const message = typeof parsed.error_message === 'string' ? parsed.error_message.trim() : ''
      if (message) return { i18nKey: null, raw: message }
    } catch {
      // 不是合法 JSON 就按普通文本处理
    }
  }

  return { i18nKey: null, raw: value }
}

/** 伪模型作用域 → 人话标签 key；真模型返回 null。 */
export function modelLabelKey(model: string): string | null {
  return PSEUDO_MODEL_I18N_KEYS[model] ?? null
}

function readModelRateLimits(account: Account): Record<string, RawModelRateLimitEntry> {
  const extra = account.extra as Record<string, unknown> | undefined
  const limits = extra?.model_rate_limits
  if (!limits || typeof limits !== 'object' || Array.isArray(limits)) return {}
  return limits as Record<string, RawModelRateLimitEntry>
}

function blocksOf(diagnosis: AccountDiagnosis | null | undefined): AccountSchedulingBlock[] {
  const blocks = diagnosis?.scheduling?.blocks
  return Array.isArray(blocks) ? blocks : []
}

/** 封锁是否仍然生效：没有 until 视为「一直有效」，有 until 就必须还没到期。 */
function blockActive(block: AccountSchedulingBlock, nowMs: number): boolean {
  return !block.until || isStillFuture(block.until, nowMs)
}

/**
 * 整号级不可调度的判定。
 *
 * 诊断接口不可用时（后端报错 / 老版本）退回账号字段自查，否则整页会把一堆
 * 已经停调的账号画成绿色。
 */
function collectAccountBlocks(
  account: Account,
  diagnosis: AccountDiagnosis | null | undefined,
  nowMs: number
): AccountBlockDetail[] {
  const found = new Map<AccountSchedulingBlockSource, AccountBlockDetail>()

  const push = (source: AccountSchedulingBlockSource, until: string | null) => {
    const existing = found.get(source)
    if (existing) {
      // 同一来源取更晚的解除时间，避免展示成马上就恢复
      if (until && (!existing.until || Date.parse(until) > Date.parse(existing.until))) {
        existing.until = until
      }
      return
    }
    found.set(source, { source, until })
  }

  for (const block of blocksOf(diagnosis)) {
    if (MODEL_LEVEL_BLOCK_SOURCES.has(block.source)) continue
    if (!blockActive(block, nowMs)) continue
    push(block.source, block.until ?? null)
  }

  // 账号字段兜底（与诊断结果求并集，按 source 去重）
  if (account.status !== 'active') push('status', null)
  if (account.schedulable === false) push('manual_unschedulable', null)
  if (isStillFuture(account.rate_limit_reset_at, nowMs)) push('rate_limited', account.rate_limit_reset_at)
  if (isStillFuture(account.overload_until, nowMs)) push('overloaded', account.overload_until)
  if (isStillFuture(account.temp_unschedulable_until, nowMs)) {
    push('temp_unschedulable', account.temp_unschedulable_until)
  }
  // 与后端 diagnoseAccountPersistedSchedulingBlocks 对齐：只有开了「过期自动暂停」的账号
  // 才会因为凭证过期停调。普通账号的 expires_at 经常是过去时间（等下次刷新令牌），
  // 按过期判死会造出一片假的「不可调度」。
  if (
    account.auto_pause_on_expired &&
    typeof account.expires_at === 'number' &&
    account.expires_at > 0 &&
    account.expires_at * 1000 <= nowMs
  ) {
    push('expired', null)
  }

  return [...found.values()]
}

/** 收集该账号当前生效的「模型级」封锁：落库限流 + 进程内模型封锁。 */
function collectModelBlocks(
  account: Account,
  diagnosis: AccountDiagnosis | null | undefined,
  nowMs: number
): Map<string, ModelBlockDetail[]> {
  const byModel = new Map<string, ModelBlockDetail[]>()

  const push = (model: string, detail: ModelBlockDetail) => {
    const key = model.trim()
    if (!key) return
    const list = byModel.get(key)
    if (list) list.push(detail)
    else byModel.set(key, [detail])
  }

  for (const [model, entry] of Object.entries(readModelRateLimits(account))) {
    if (!entry || typeof entry !== 'object') continue
    // 已过期的限流记录不能显示成「限流中」
    if (!isStillFuture(entry.rate_limit_reset_at, nowMs)) continue
    push(model, {
      source: 'model_rate_limit',
      until: entry.rate_limit_reset_at ?? null,
      reason: typeof entry.reason === 'string' ? entry.reason : null
    })
  }

  for (const block of blocksOf(diagnosis)) {
    if (!MODEL_LEVEL_BLOCK_SOURCES.has(block.source)) continue
    if (!blockActive(block, nowMs)) continue
    push(block.model ?? '', { source: block.source, until: block.until ?? null, reason: null })
  }

  return byModel
}

function latestUntil(details: ModelBlockDetail[]): string | null {
  let best: string | null = null
  let bestTs = Number.NEGATIVE_INFINITY
  for (const detail of details) {
    if (!detail.until) continue
    const ts = Date.parse(detail.until)
    if (!Number.isFinite(ts)) continue
    if (ts > bestTs) {
      bestTs = ts
      best = detail.until
    }
  }
  return best
}

/**
 * 构建矩阵。
 *
 * @param inputs 每个账号的账号数据 + 诊断 + 模型清单
 * @param nowMs  判定「是否仍在限流中」的基准时间
 */
export function buildModelAvailabilityMatrix(
  inputs: ModelAvailabilityInput[],
  nowMs: number = Date.now()
): ModelAvailabilityMatrix {
  const prepared = inputs.map((input) => {
    const supported = new Set((input.models ?? []).map((m) => m.trim()).filter(Boolean))
    const modelBlocks = collectModelBlocks(input.account, input.diagnosis, nowMs)
    return {
      input,
      supported,
      modelBlocks,
      accountBlocks: collectAccountBlocks(input.account, input.diagnosis, nowMs)
    }
  })

  // 列 = 所有账号支持的模型 ∪ 所有当前生效的模型级封锁键
  const limitedCounts = new Map<string, number>()
  const allModels = new Set<string>()
  for (const item of prepared) {
    for (const model of item.supported) allModels.add(model)
    for (const model of item.modelBlocks.keys()) {
      allModels.add(model)
      limitedCounts.set(model, (limitedCounts.get(model) ?? 0) + 1)
    }
  }

  const columns: MatrixColumn[] = [...allModels]
    .map((model) => ({
      model,
      labelKey: modelLabelKey(model),
      limitedCount: limitedCounts.get(model) ?? 0
    }))
    // 有问题的模型排前面，方便一眼看到是哪几个模型在限流
    .sort((a, b) => {
      if ((a.limitedCount > 0) !== (b.limitedCount > 0)) return a.limitedCount > 0 ? -1 : 1
      return a.model.localeCompare(b.model)
    })

  let limitedCellCount = 0
  let blockedAccountCount = 0

  const rows: MatrixRow[] = prepared.map((item) => {
    const modelsUnavailable = item.input.models === undefined
    const cells: MatrixCell[] = columns.map((column) => {
      const details = item.modelBlocks.get(column.model)
      if (details && details.length > 0) {
        return {
          model: column.model,
          state: 'limited',
          until: latestUntil(details),
          details
        }
      }
      if (item.supported.has(column.model)) {
        return { model: column.model, state: 'ok', until: null, details: [] }
      }
      return {
        model: column.model,
        state: modelsUnavailable ? 'unknown' : 'unsupported',
        until: null,
        details: []
      }
    })

    const limitedModels = cells.filter((cell) => cell.state === 'limited').length
    limitedCellCount += limitedModels
    const accountBlocked = item.accountBlocks.length > 0
    if (accountBlocked) blockedAccountCount += 1

    return {
      id: item.input.account.id,
      name: item.input.account.name,
      platform: item.input.account.platform,
      accountBlocked,
      accountBlocks: item.accountBlocks,
      limitedModels,
      modelsUnavailable,
      cells
    }
  })

  // 整号不可调度排最前（最严重），其次按被限流模型数降序，最后按名字
  rows.sort((a, b) => {
    if (a.accountBlocked !== b.accountBlocked) return a.accountBlocked ? -1 : 1
    if (a.limitedModels !== b.limitedModels) return b.limitedModels - a.limitedModels
    return a.name.localeCompare(b.name)
  })

  return {
    columns,
    rows,
    summary: {
      accountCount: rows.length,
      modelCount: columns.length,
      blockedAccountCount,
      limitedCellCount
    }
  }
}

/**
 * 只保留有问题的行与列（整号不可调度 / 有模型被限流）。
 * summary 保持不变：汇总条描述的是当前范围内的整体情况，不随视图收窄而变小。
 */
export function filterIssuesOnly(matrix: ModelAvailabilityMatrix): ModelAvailabilityMatrix {
  const keptColumnIndexes: number[] = []
  matrix.columns.forEach((column, index) => {
    if (column.limitedCount > 0) keptColumnIndexes.push(index)
  })

  const columns = keptColumnIndexes.map((index) => matrix.columns[index])
  const rows = matrix.rows
    .filter((row) => row.accountBlocked || row.limitedModels > 0)
    .map((row) => ({ ...row, cells: keptColumnIndexes.map((index) => row.cells[index]) }))

  return { columns, rows, summary: matrix.summary }
}

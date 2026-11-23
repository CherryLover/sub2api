export type ModelFamilyKey =
  | 'deepseek'
  | 'openai'
  | 'claude'
  | 'gemini'
  | 'grok'
  | 'qwen'
  | 'glm'
  | 'kimi'
  | 'other'

export interface ModelFamilyMeta {
  key: ModelFamilyKey
  label: string
  color: string
  dotClass: string
  badgeClass: string
  barClass: string
}

export interface ModelUsageInput {
  model: string
  requests: number
  tokens: number
  cost: number
}

export interface ModelFamilyAggregate {
  family: ModelFamilyKey
  label: string
  color: string
  dotClass: string
  badgeClass: string
  barClass: string
  requests: number
  tokens: number
  cost: number
  sharePct: number
  modelsCount: number
}

export const MODEL_FAMILY_META: Record<ModelFamilyKey, ModelFamilyMeta> = {
  deepseek: {
    key: 'deepseek',
    label: 'DeepSeek',
    color: '#14b8a6',
    dotClass: 'bg-teal-500',
    badgeClass: 'bg-teal-500/10 text-teal-700 dark:bg-teal-500/15 dark:text-teal-300',
    barClass: 'bg-teal-500',
  },
  openai: {
    key: 'openai',
    label: 'GPT / Codex',
    color: '#10b981',
    dotClass: 'bg-emerald-500',
    badgeClass: 'bg-emerald-500/10 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300',
    barClass: 'bg-emerald-500',
  },
  claude: {
    key: 'claude',
    label: 'Claude',
    color: '#f59e0b',
    dotClass: 'bg-amber-500',
    badgeClass: 'bg-amber-500/10 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300',
    barClass: 'bg-amber-500',
  },
  gemini: {
    key: 'gemini',
    label: 'Gemini',
    color: '#3b82f6',
    dotClass: 'bg-blue-500',
    badgeClass: 'bg-blue-500/10 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300',
    barClass: 'bg-blue-500',
  },
  grok: {
    key: 'grok',
    label: 'Grok',
    color: '#64748b',
    dotClass: 'bg-slate-500',
    badgeClass: 'bg-slate-500/10 text-slate-700 dark:bg-slate-500/20 dark:text-slate-300',
    barClass: 'bg-slate-500',
  },
  qwen: {
    key: 'qwen',
    label: 'Qwen',
    color: '#8b5cf6',
    dotClass: 'bg-violet-500',
    badgeClass: 'bg-violet-500/10 text-violet-700 dark:bg-violet-500/15 dark:text-violet-300',
    barClass: 'bg-violet-500',
  },
  glm: {
    key: 'glm',
    label: 'GLM',
    color: '#6366f1',
    dotClass: 'bg-indigo-500',
    badgeClass: 'bg-indigo-500/10 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300',
    barClass: 'bg-indigo-500',
  },
  kimi: {
    key: 'kimi',
    label: 'Kimi',
    color: '#06b6d4',
    dotClass: 'bg-cyan-500',
    badgeClass: 'bg-cyan-500/10 text-cyan-700 dark:bg-cyan-500/15 dark:text-cyan-300',
    barClass: 'bg-cyan-500',
  },
  other: {
    key: 'other',
    label: 'Other',
    color: '#9ca3af',
    dotClass: 'bg-gray-400',
    badgeClass: 'bg-gray-500/10 text-gray-600 dark:bg-gray-500/20 dark:text-gray-300',
    barClass: 'bg-gray-400',
  },
}

/**
 * Classify an upstream model name into a display family (DeepSeek / GPT / Claude / Gemini / ...).
 * Strips optional provider prefixes such as "openai/" or "deepseek/".
 */
export function classifyModelFamily(model: string): ModelFamilyKey {
  const raw = (model || '').trim().toLowerCase()
  if (!raw) return 'other'

  // Check explicit provider prefix first (e.g. "deepseek/deepseek-chat", "anthropic/claude-...")
  const slashIdx = raw.indexOf('/')
  const prefix = slashIdx > 0 ? raw.slice(0, slashIdx) : ''
  const name = slashIdx > 0 ? raw.slice(slashIdx + 1) : raw

  if (prefix === 'deepseek' || name.includes('deepseek')) return 'deepseek'
  if (prefix === 'anthropic' || name.includes('claude')) return 'claude'
  if (
    prefix === 'google' ||
    name.includes('gemini') ||
    name.includes('gemma') ||
    name.includes('learnlm') ||
    name.includes('imagen-') ||
    name.includes('veo-')
  ) {
    return 'gemini'
  }
  if (
    prefix === 'openai' ||
    name.startsWith('gpt') ||
    name.includes('codex') ||
    name.startsWith('o1') ||
    name.startsWith('o3') ||
    name.startsWith('o4') ||
    name.includes('chatgpt') ||
    name.includes('dall-e') ||
    name.includes('whisper') ||
    name.includes('tts-1') ||
    name.includes('text-embedding-') ||
    name.includes('text-moderation') ||
    name.includes('davinci') ||
    name.includes('babbage')
  ) {
    return 'openai'
  }
  if (prefix === 'xai' || prefix === 'x-ai' || name.includes('grok')) return 'grok'
  if (prefix === 'qwen' || name.includes('qwen') || name.includes('qwq')) return 'qwen'
  if (
    prefix === 'zhipu' ||
    prefix === 'zai' ||
    name.includes('glm') ||
    name.includes('chatglm') ||
    name.includes('cogview') ||
    name.includes('cogvideo')
  ) {
    return 'glm'
  }
  if (prefix === 'moonshot' || name.includes('kimi') || name.includes('moonshot')) return 'kimi'

  return 'other'
}

export function getModelFamilyMeta(model: string): ModelFamilyMeta {
  return MODEL_FAMILY_META[classifyModelFamily(model)]
}

/**
 * Aggregate per-model usage stats into model families, sorted by cost desc (then tokens, requests).
 */
export function aggregateModelFamilies(models: ModelUsageInput[]): ModelFamilyAggregate[] {
  if (!Array.isArray(models) || models.length === 0) return []

  const buckets = new Map<
    ModelFamilyKey,
    { requests: number; tokens: number; cost: number; distinctModels: Set<string> }
  >()

  let totalCost = 0
  let totalTokens = 0
  let totalRequests = 0

  for (const item of models) {
    if (!item) continue
    const family = classifyModelFamily(item.model)
    const requests = Number(item.requests) || 0
    const tokens = Number(item.tokens) || 0
    const cost = Number(item.cost) || 0

    totalCost += cost
    totalTokens += tokens
    totalRequests += requests

    let bucket = buckets.get(family)
    if (!bucket) {
      bucket = { requests: 0, tokens: 0, cost: 0, distinctModels: new Set<string>() }
      buckets.set(family, bucket)
    }
    bucket.requests += requests
    bucket.tokens += tokens
    bucket.cost += cost
    if (item.model) {
      bucket.distinctModels.add(item.model)
    }
  }

  const results: ModelFamilyAggregate[] = []
  for (const [family, bucket] of buckets.entries()) {
    const meta = MODEL_FAMILY_META[family]
    let sharePct = 0
    if (totalCost > 0) {
      sharePct = (bucket.cost / totalCost) * 100
    } else if (totalTokens > 0) {
      sharePct = (bucket.tokens / totalTokens) * 100
    } else if (totalRequests > 0) {
      sharePct = (bucket.requests / totalRequests) * 100
    }

    results.push({
      family,
      label: meta.label,
      color: meta.color,
      dotClass: meta.dotClass,
      badgeClass: meta.badgeClass,
      barClass: meta.barClass,
      requests: bucket.requests,
      tokens: bucket.tokens,
      cost: bucket.cost,
      sharePct,
      modelsCount: bucket.distinctModels.size,
    })
  }

  results.sort((a, b) => {
    if (b.cost !== a.cost) return b.cost - a.cost
    if (b.tokens !== a.tokens) return b.tokens - a.tokens
    return b.requests - a.requests
  })

  return results
}

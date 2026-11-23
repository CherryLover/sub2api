<template>
  <div
    data-test="apikey-usage-expand-panel"
    class="border-t border-primary-100/80 bg-gradient-to-b from-gray-50/90 to-white px-5 py-5 dark:border-dark-700 dark:from-dark-800/70 dark:to-dark-900/90 sm:px-7 sm:py-6"
    @click.stop
  >
    <!-- Header: Key info + Window selector + Actions -->
    <div class="flex flex-col gap-3 border-b border-gray-200/80 pb-4 dark:border-dark-700/80 lg:flex-row lg:items-center lg:justify-between">
      <div class="min-w-0 space-y-1">
        <div class="flex flex-wrap items-center gap-2">
          <h4 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ apiKey.name }}
          </h4>
          <code class="code text-xs">{{ apiKey.key }}</code>
          <span class="rounded-full bg-teal-500/10 px-2.5 py-0.5 text-[11px] font-medium text-teal-700 dark:bg-teal-500/15 dark:text-teal-300">
            {{ t('admin.apiKeys.expand.upstreamBasis') }}
          </span>
        </div>
        <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
          <span v-if="apiKey.user?.email">
            {{ t('admin.apiKeys.columns.user') }}: <strong class="font-medium text-gray-700 dark:text-dark-200">{{ apiKey.user.email }}</strong>
          </span>
          <span v-if="apiKey.group?.name">
            {{ t('admin.apiKeys.columns.group') }}: <strong class="font-medium text-gray-700 dark:text-dark-200">{{ apiKey.group.name }}</strong>
          </span>
          <span>
            {{ t('admin.apiKeys.today') }}: <strong class="font-medium tabular-nums text-gray-700 dark:text-dark-200">${{ (usageStat?.today_actual_cost ?? 0).toFixed(4) }}</strong>
          </span>
          <span>
            {{ t('admin.apiKeys.spent') }}: <strong class="font-medium tabular-nums text-gray-700 dark:text-dark-200">${{ (apiKey.quota_used ?? 0).toFixed(4) }}</strong>
          </span>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <!-- Window tabs -->
        <div
          class="inline-flex rounded-lg border border-gray-200 bg-white p-0.5 dark:border-dark-700 dark:bg-dark-950"
          role="group"
          data-test="expand-window-tabs"
        >
          <button
            v-for="opt in windowOptions"
            :key="opt.value"
            type="button"
            :data-test="`expand-window-${opt.value}`"
            :aria-pressed="activeWindow === opt.value"
            class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
            :class="
              activeWindow === opt.value
                ? 'bg-primary-500 text-white shadow-sm'
                : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-800'
            "
            @click="selectWindow(opt.value)"
          >
            {{ opt.label }}
          </button>
        </div>

        <button
          type="button"
          class="inline-flex items-center gap-1 rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:border-gray-300 hover:text-gray-900 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-300 dark:hover:text-white"
          :disabled="loading"
          :title="t('common.refresh')"
          data-test="expand-refresh"
          @click="loadModelStats(true)"
        >
          <Icon name="refresh" size="xs" :class="loading ? 'animate-spin' : ''" />
        </button>

        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-lg border border-primary-200 bg-primary-50/70 px-3 py-1.5 text-xs font-medium text-primary-700 transition-colors hover:bg-primary-100 dark:border-primary-500/30 dark:bg-primary-500/10 dark:text-primary-300 dark:hover:bg-primary-500/20"
          data-test="expand-open-usage"
          @click="emit('openUsage', apiKey)"
        >
          <Icon name="chartBar" size="xs" />
          <span>{{ t('admin.apiKeys.expand.openFullLogs') }}</span>
        </button>
      </div>
    </div>

    <!-- Loading state -->
    <div v-if="loading && models.length === 0" data-test="expand-loading" class="mt-4 space-y-4">
      <div class="grid gap-3 sm:grid-cols-4">
        <div v-for="n in 4" :key="n" class="h-16 animate-pulse rounded-xl bg-gray-200/70 dark:bg-dark-700/60" />
      </div>
      <div class="h-24 animate-pulse rounded-xl bg-gray-200/70 dark:bg-dark-700/60" />
    </div>

    <!-- Error state -->
    <div
      v-else-if="errorMessage"
      data-test="expand-error"
      class="mt-4 flex items-center justify-between gap-3 rounded-xl border border-rose-200 bg-rose-50/70 px-4 py-3 text-xs text-rose-700 dark:border-rose-500/30 dark:bg-rose-500/10 dark:text-rose-300"
    >
      <span>{{ errorMessage }}</span>
      <button
        type="button"
        class="rounded-md border border-rose-300 px-2.5 py-1 font-medium hover:bg-rose-100 dark:border-rose-500/40 dark:hover:bg-rose-500/20"
        @click="loadModelStats(true)"
      >
        {{ t('admin.apiKeys.expand.retry') }}
      </button>
    </div>

    <!-- Content -->
    <div v-else class="mt-5 space-y-5" :class="{ 'opacity-60 transition-opacity': loading }">
      <!-- Window Summary KPIs -->
      <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div class="rounded-xl border border-gray-200/80 bg-white px-4 py-3 dark:border-dark-700/80 dark:bg-dark-900/70">
          <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.apiKeys.expand.requests') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white" :title="formatCountExact(totals.requests)">
            {{ formatCount(totals.requests) }}
          </div>
        </div>
        <div class="rounded-xl border border-gray-200/80 bg-white px-4 py-3 dark:border-dark-700/80 dark:bg-dark-900/70">
          <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.apiKeys.expand.totalTokens') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white" :title="formatCountExact(totals.tokens)">
            {{ formatCount(totals.tokens) }}
          </div>
        </div>
        <div class="rounded-xl border border-gray-200/80 bg-white px-4 py-3 dark:border-dark-700/80 dark:bg-dark-900/70">
          <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.apiKeys.expand.actualCost') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">
            {{ formatUsd(totals.cost) }}
          </div>
        </div>
        <div class="rounded-xl border border-gray-200/80 bg-white px-4 py-3 dark:border-dark-700/80 dark:bg-dark-900/70">
          <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.apiKeys.expand.activeModels') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">
            {{ sortedModels.length }}
          </div>
        </div>
      </div>

      <!-- Empty state -->
      <div
        v-if="sortedModels.length === 0"
        data-test="expand-empty"
        class="rounded-xl border border-dashed border-gray-200 bg-white/70 px-6 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900/50 dark:text-dark-400"
      >
        {{ t('admin.apiKeys.expand.noModelUsage') }}
      </div>

      <template v-else>
        <!-- Family Summary (GPT / DeepSeek / Claude / ...) -->
        <ModelFamilySummary
          :models="familyInputs"
          :title="t('admin.apiKeys.expand.familyDistribution')"
          :basis-note="t('admin.apiKeys.expand.familyHint')"
          :other-label="t('admin.apiKeys.expand.otherFamily')"
          :requests-unit="t('admin.apiKeys.expand.requestsShort')"
        />

        <!-- Concrete Upstream Model Breakdown Table -->
        <div class="overflow-hidden rounded-xl border border-gray-200/80 bg-white dark:border-dark-700/80 dark:bg-dark-900/70">
          <div class="flex items-center justify-between border-b border-gray-200/80 px-4 py-3 dark:border-dark-700/80">
            <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
              {{ t('admin.apiKeys.expand.modelBreakdown') }}
            </span>
            <span class="text-[11px] text-gray-400 dark:text-dark-400">
              {{ t('admin.apiKeys.expand.modelCount', { count: sortedModels.length }) }}
            </span>
          </div>
          <div class="overflow-x-auto">
            <table class="w-full min-w-[40rem]" data-test="expand-model-table">
              <thead>
                <tr class="border-b border-gray-100 bg-gray-50/80 text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:border-dark-800 dark:bg-dark-950/60 dark:text-dark-400">
                  <th class="px-4 py-2.5 text-left">{{ t('admin.apiKeys.expand.modelColumn') }}</th>
                  <th class="px-4 py-2.5 text-right">{{ t('admin.apiKeys.expand.requests') }}</th>
                  <th class="px-4 py-2.5 text-right">{{ t('admin.apiKeys.expand.inputTokens') }}</th>
                  <th class="px-4 py-2.5 text-right">{{ t('admin.apiKeys.expand.outputTokens') }}</th>
                  <th class="px-4 py-2.5 text-right">{{ t('admin.apiKeys.expand.totalTokens') }}</th>
                  <th class="px-4 py-2.5 text-right">{{ t('admin.apiKeys.expand.actualCost') }}</th>
                  <th class="px-4 py-2.5 text-right">{{ t('admin.apiKeys.expand.share') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                <tr
                  v-for="row in sortedModels"
                  :key="row.model"
                  data-test="expand-model-row"
                  class="hover:bg-gray-50/70 dark:hover:bg-dark-800/40"
                >
                  <td class="px-4 py-2.5 text-sm">
                    <div class="flex items-center gap-2">
                      <span
                        :class="[
                          'inline-flex shrink-0 items-center rounded-md px-2 py-0.5 text-[11px] font-medium',
                          row.familyMeta.badgeClass
                        ]"
                      >
                        {{ row.familyMeta.key === 'other' ? t('admin.apiKeys.expand.otherFamily') : row.familyMeta.label }}
                      </span>
                      <span class="truncate font-mono text-xs font-medium text-gray-900 dark:text-white" :title="row.model">
                        {{ row.model || '-' }}
                      </span>
                    </div>
                  </td>
                  <td class="px-4 py-2.5 text-right text-xs tabular-nums text-gray-700 dark:text-dark-200" :title="formatCountExact(row.requests)">
                    {{ formatCount(row.requests) }}
                  </td>
                  <td class="px-4 py-2.5 text-right text-xs tabular-nums text-gray-600 dark:text-dark-300" :title="formatCountExact(row.input_tokens)">
                    {{ formatCount(row.input_tokens) }}
                  </td>
                  <td class="px-4 py-2.5 text-right text-xs tabular-nums text-gray-600 dark:text-dark-300" :title="formatCountExact(row.output_tokens)">
                    {{ formatCount(row.output_tokens) }}
                  </td>
                  <td class="px-4 py-2.5 text-right text-xs tabular-nums text-gray-700 dark:text-dark-200" :title="formatCountExact(row.total_tokens)">
                    {{ formatCount(row.total_tokens) }}
                  </td>
                  <td class="px-4 py-2.5 text-right text-xs font-semibold tabular-nums text-gray-900 dark:text-white">
                    {{ formatUsd(row.displayCost) }}
                  </td>
                  <td class="px-4 py-2.5 text-right text-xs tabular-nums text-gray-600 dark:text-dark-300">
                    <div class="flex items-center justify-end gap-2">
                      <div class="h-1.5 w-16 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                        <div
                          :class="['h-full rounded-full', row.familyMeta.barClass]"
                          :style="{ width: `${Math.min(Math.max(row.sharePct, 0), 100).toFixed(1)}%` }"
                        />
                      </div>
                      <span class="w-11 text-right">{{ row.sharePct.toFixed(1) }}%</span>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { BatchApiKeyUsageStats } from '@/api/admin/dashboard'
import type { ApiKey, ModelStat } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import ModelFamilySummary from '@/components/common/ModelFamilySummary.vue'
import { getModelFamilyMeta, type ModelUsageInput } from '@/utils/modelFamily'
import { formatCount, formatCountExact, formatUsd } from '@/utils/keyUsageFormat'

export type ExpandUsageWindow = 'today' | '7d' | '30d' | 'all'

const props = defineProps<{
  apiKey: ApiKey
  usageStat?: BatchApiKeyUsageStats
}>()

const emit = defineEmits<{
  openUsage: [apiKey: ApiKey]
}>()

const { t } = useI18n()

const activeWindow = ref<ExpandUsageWindow>('today')
const loading = ref(false)
const errorMessage = ref('')
const models = ref<ModelStat[]>([])
const cache = new Map<ExpandUsageWindow, ModelStat[]>()

let abortController: AbortController | null = null

const windowOptions = computed<Array<{ value: ExpandUsageWindow; label: string }>>(() => [
  { value: 'today', label: t('admin.apiKeys.expand.windowToday') },
  { value: '7d', label: t('admin.apiKeys.expand.window7d') },
  { value: '30d', label: t('admin.apiKeys.expand.window30d') },
  { value: 'all', label: t('admin.apiKeys.expand.windowAll') },
])

const formatLocalDate = (date: Date): string => {
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

const getBrowserTimezone = (): string | undefined => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || undefined
  } catch {
    return undefined
  }
}

const resolveDateRange = (win: ExpandUsageWindow): { start_date: string; end_date: string } => {
  const now = new Date()
  const end = formatLocalDate(now)
  if (win === 'today') {
    return { start_date: end, end_date: end }
  }
  if (win === '7d') {
    const start = new Date(now)
    start.setDate(start.getDate() - 6)
    return { start_date: formatLocalDate(start), end_date: end }
  }
  if (win === '30d') {
    const start = new Date(now)
    start.setDate(start.getDate() - 29)
    return { start_date: formatLocalDate(start), end_date: end }
  }
  return { start_date: '2020-01-01', end_date: end }
}

const isAbortError = (error: unknown): boolean => {
  if (!error || typeof error !== 'object') return false
  const { name, code } = error as { name?: string; code?: string }
  return name === 'AbortError' || code === 'ERR_CANCELED'
}

const loadModelStats = async (force = false) => {
  const win = activeWindow.value
  if (!force && cache.has(win)) {
    models.value = cache.get(win) ?? []
    errorMessage.value = ''
    return
  }

  abortController?.abort()
  const controller = new AbortController()
  abortController = controller

  loading.value = true
  errorMessage.value = ''

  try {
    const { start_date, end_date } = resolveDateRange(win)
    const response = await adminAPI.dashboard.getModelStats(
      {
        api_key_id: props.apiKey.id,
        start_date,
        end_date,
        model_source: 'upstream',
        timezone: getBrowserTimezone(),
      },
      { signal: controller.signal }
    )
    if (controller.signal.aborted) return
    const list = Array.isArray(response?.models) ? response.models : []
    cache.set(win, list)
    models.value = list
  } catch (err) {
    if (isAbortError(err)) return
    errorMessage.value = t('admin.apiKeys.expand.loadFailed')
  } finally {
    if (abortController === controller) {
      loading.value = false
    }
  }
}

const selectWindow = (win: ExpandUsageWindow) => {
  if (activeWindow.value === win) return
  activeWindow.value = win
  loadModelStats(false)
}

const totals = computed(() => {
  let requests = 0
  let tokens = 0
  let cost = 0
  for (const m of models.value) {
    requests += Number(m.requests) || 0
    tokens += Number(m.total_tokens) || 0
    cost += Number(m.actual_cost ?? m.cost) || 0
  }
  return { requests, tokens, cost }
})

const familyInputs = computed<ModelUsageInput[]>(() =>
  models.value.map((m) => ({
    model: m.model,
    requests: Number(m.requests) || 0,
    tokens: Number(m.total_tokens) || 0,
    cost: Number(m.actual_cost ?? m.cost) || 0,
  }))
)

const sortedModels = computed(() => {
  const totalCost = totals.value.cost
  const totalTokens = totals.value.tokens
  const totalRequests = totals.value.requests

  return [...models.value]
    .map((m) => {
      const displayCost = Number(m.actual_cost ?? m.cost) || 0
      const tok = Number(m.total_tokens) || 0
      const req = Number(m.requests) || 0
      let sharePct = 0
      if (totalCost > 0) {
        sharePct = (displayCost / totalCost) * 100
      } else if (totalTokens > 0) {
        sharePct = (tok / totalTokens) * 100
      } else if (totalRequests > 0) {
        sharePct = (req / totalRequests) * 100
      }
      return {
        ...m,
        displayCost,
        sharePct,
        familyMeta: getModelFamilyMeta(m.model),
      }
    })
    .sort((a, b) => {
      if (b.displayCost !== a.displayCost) return b.displayCost - a.displayCost
      if (b.total_tokens !== a.total_tokens) return b.total_tokens - a.total_tokens
      return b.requests - a.requests
    })
})

watch(
  () => props.apiKey.id,
  () => {
    cache.clear()
    activeWindow.value = 'today'
    loadModelStats(true)
  }
)

onMounted(() => {
  loadModelStats(false)
})

onUnmounted(() => {
  abortController?.abort()
})
</script>

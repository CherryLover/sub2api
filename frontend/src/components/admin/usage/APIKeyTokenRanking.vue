<template>
  <div>
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700/50 sm:px-6">
      <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('admin.usage.keyRanking.subtitle') }}</p>
      <div class="flex items-center gap-3">
        <span v-if="!loading && items.length > 0" class="text-xs text-gray-400 dark:text-gray-500">
          {{ t('admin.usage.keyRanking.keyCount', { count: items.length }) }}
        </span>
        <div class="w-28">
          <Select v-model="limit" :options="limitOptions" @change="load" />
        </div>
      </div>
    </div>

    <div class="overflow-x-auto">
      <table class="w-full min-w-max divide-y divide-gray-200 dark:divide-dark-700">
        <thead class="bg-gray-50 dark:bg-dark-800">
          <tr>
            <th class="w-12 px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400 sm:px-6">#</th>
            <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">
              {{ t('admin.usage.keyRanking.columns.key') }}
            </th>
            <th
              v-for="col in sortableColumns"
              :key="col.key"
              class="cursor-pointer select-none whitespace-nowrap px-4 py-3 text-right text-xs font-medium uppercase tracking-wider transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
              :class="sortBy === col.key ? 'text-primary-600 dark:text-primary-400' : 'text-gray-500 dark:text-dark-400'"
              @click="setSort(col.key)"
            >
              {{ t(col.label) }}
              <span v-if="sortBy === col.key" aria-hidden="true">↓</span>
            </th>
            <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400 sm:pr-6">
              {{ t('admin.usage.keyRanking.columns.share') }}
            </th>
          </tr>
        </thead>
        <tbody v-if="loading">
          <tr>
            <td colspan="6" class="py-12 text-center">
              <LoadingSpinner />
            </td>
          </tr>
        </tbody>
        <tbody v-else-if="items.length === 0">
          <tr>
            <td colspan="6" class="py-12 text-center text-sm text-gray-400">
              {{ t('admin.dashboard.noDataAvailable') }}
            </td>
          </tr>
        </tbody>
        <tbody v-else class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
          <template v-for="(item, index) in items" :key="item.api_key_id">
            <tr
              class="cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/40"
              :title="t('admin.usage.keyRanking.rowHint')"
              @click="toggle(item.api_key_id)"
            >
              <td class="px-4 py-3 sm:px-6">
                <span class="inline-flex items-center gap-1.5">
                  <svg
                    class="h-3 w-3 shrink-0 text-gray-400 transition-transform"
                    :class="expandedId === item.api_key_id ? 'rotate-90' : ''"
                    fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2.4" aria-hidden="true"
                  ><path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" /></svg>
                  <span
                    v-if="index < 3"
                    class="inline-flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold"
                    :class="RANK_BADGE_CLASSES[index]"
                  >{{ index + 1 }}</span>
                  <span v-else class="inline-block w-6 text-center text-sm tabular-nums text-gray-400">{{ index + 1 }}</span>
                </span>
              </td>
              <td class="max-w-[280px] px-4 py-3">
                <div class="truncate text-sm font-medium text-gray-700 dark:text-gray-200" :title="item.name">
                  {{ item.name || `Key #${item.api_key_id}` }}
                </div>
                <div class="truncate text-xs text-gray-400 dark:text-gray-500" :title="item.email">
                  {{ topModelLabel(item) }}
                </div>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-right text-sm tabular-nums text-gray-500 dark:text-gray-400">{{ item.requests.toLocaleString() }}</td>
              <td class="whitespace-nowrap px-4 py-3 text-right text-sm tabular-nums text-gray-500 dark:text-gray-400">{{ formatCompactNumber(item.total_tokens) }}</td>
              <td class="whitespace-nowrap px-4 py-3 text-right text-sm font-medium tabular-nums text-green-600 dark:text-green-400">${{ formatCostFixed(item.actual_cost, 2) }}</td>
              <td class="whitespace-nowrap px-4 py-3 sm:pr-6">
                <div class="flex items-center justify-end gap-2">
                  <div class="h-1.5 w-16 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                    <div class="h-full rounded-full bg-primary-500" :style="{ width: shareWidth(item) }" />
                  </div>
                  <span class="w-11 text-right text-xs tabular-nums text-gray-500 dark:text-gray-400">{{ sharePct(item) }}%</span>
                </div>
              </td>
            </tr>

            <tr v-if="expandedId === item.api_key_id" class="bg-white dark:bg-dark-900">
              <td colspan="6" class="px-4 pb-4 pt-0 sm:px-6">
                <BreakdownModelDetail
                  :models="item.models || []"
                  :row-cost="item.actual_cost"
                  :overall-cost="resolvedOverallCost"
                  :requests="item.requests"
                  :total-tokens="item.total_tokens"
                />
                <div class="mt-2 flex justify-end">
                  <button
                    type="button"
                    class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                    @click.stop="$emit('select-key', item.api_key_id, item.name)"
                  >
                    {{ t('admin.usage.keyRanking.viewUsage') }} →
                  </button>
                </div>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getAPIKeyBreakdown, type APIKeyBreakdownParams } from '@/api/admin/dashboard'
import { formatCompactNumber, formatCostFixed } from '@/utils/format'
import type { APIKeyBreakdownItem } from '@/types'
import Select from '@/components/common/Select.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import BreakdownModelDetail from './BreakdownModelDetail.vue'

const props = defineProps<{
  startDate: string
  endDate: string
  filters: Record<string, unknown>
  model?: string
  overallCost?: number
}>()

defineEmits<{ (e: 'select-key', apiKeyId: number, name: string): void }>()

const { t } = useI18n()

type SortKey = NonNullable<APIKeyBreakdownParams['sort_by']>
const sortableColumns: { key: SortKey; label: string }[] = [
  { key: 'requests', label: 'admin.usage.keyRanking.columns.requests' },
  { key: 'total_tokens', label: 'admin.usage.keyRanking.columns.tokens' },
  { key: 'actual_cost', label: 'admin.usage.keyRanking.columns.cost' }
]

const limitOptions = [
  { value: 20, label: 'Top 20' },
  { value: 50, label: 'Top 50' },
  { value: 100, label: 'Top 100' },
  { value: 200, label: 'Top 200' }
]

const RANK_BADGE_CLASSES = [
  'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-400',
  'bg-gray-200 text-gray-600 dark:bg-gray-500/20 dark:text-gray-300',
  'bg-orange-100 text-orange-700 dark:bg-orange-500/20 dark:text-orange-400'
]

const items = ref<APIKeyBreakdownItem[]>([])
const loading = ref(false)
const sortBy = ref<SortKey>('actual_cost')
const limit = ref(50)
const expandedId = ref<number | null>(null)
let reqSeq = 0

// 页面没给总额时退化成当前列表合计，保证占比列至少自洽。
const resolvedOverallCost = computed(() => {
  const provided = props.overallCost ?? 0
  if (provided > 0) return provided
  return items.value.reduce((sum, item) => sum + item.actual_cost, 0)
})

const sharePct = (item: APIKeyBreakdownItem): string => {
  const total = resolvedOverallCost.value
  if (total <= 0) return '0.0'
  return ((item.actual_cost / total) * 100).toFixed(1)
}

const shareWidth = (item: APIKeyBreakdownItem): string => {
  const pct = Number(sharePct(item))
  return `${Math.min(Math.max(pct, 1), 100)}%`
}

const topModelLabel = (item: APIKeyBreakdownItem): string => {
  const models = item.models || []
  if (models.length === 0) return item.email || ''
  const top = models[0]
  return `${top.model} · ${t('admin.usage.keyRanking.modelCount', { count: models.length })}`
}

const toggle = (id: number) => {
  expandedId.value = expandedId.value === id ? null : id
}

const setSort = (key: SortKey) => {
  if (sortBy.value === key) return
  sortBy.value = key
  load()
}

const load = async () => {
  const seq = ++reqSeq
  loading.value = true
  try {
    const params: APIKeyBreakdownParams = {
      ...props.filters,
      start_date: props.startDate,
      end_date: props.endDate,
      sort_by: sortBy.value,
      limit: limit.value,
      include_models: true
    }
    if (props.model) params.model = props.model
    const res = await getAPIKeyBreakdown(params)
    if (seq !== reqSeq) return
    items.value = res.api_keys || []
  } catch {
    if (seq !== reqSeq) return
    items.value = []
  } finally {
    if (seq === reqSeq) loading.value = false
  }
}

watch(
  () => [props.startDate, props.endDate, props.model, JSON.stringify(props.filters)],
  () => load(),
  { immediate: true }
)

defineExpose({ reload: load })
</script>

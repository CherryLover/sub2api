<template>
  <div class="space-y-4" data-testid="key-usage-dashboard">
    <UsageDashboardFilters
      :range="range"
      :metric="metric"
      :group-id="groupId"
      :model="model"
      :group-options="groupOptions"
      :model-options="modelOptions"
      :loading="loading"
      @update:range="range = $event"
      @update:metric="metric = $event"
      @update:group="groupId = $event"
      @update:model="model = $event"
      @refresh="loadDashboard"
    />

    <div v-if="!dashboard && loading" class="space-y-4" data-testid="usage-dashboard-skeleton">
      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-6">
        <div v-for="index in 6" :key="index" class="card h-28 animate-pulse bg-gray-100 dark:bg-dark-800" />
      </div>
      <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div class="card h-96 animate-pulse bg-gray-100 dark:bg-dark-800" />
        <div class="card h-96 animate-pulse bg-gray-100 dark:bg-dark-800" />
      </div>
    </div>

    <div
      v-else-if="error && !dashboard"
      role="alert"
      class="flex flex-wrap items-center justify-center gap-2 rounded-lg border border-red-200 bg-red-50 p-5 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300"
      data-testid="usage-dashboard-error"
    >
      <Icon name="exclamationCircle" size="sm" />
      <span>{{ t('keyUsageDashboard.loadFailed') }}</span>
      <button type="button" class="underline" @click="loadDashboard">
        {{ t('keyUsageDashboard.retry') }}
      </button>
    </div>

    <div
      v-else-if="dashboard"
      class="space-y-4 transition-opacity"
      :class="loading ? 'pointer-events-none opacity-60' : ''"
    >
      <div
        v-if="error"
        role="alert"
        class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-2 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200"
      >
        {{ t('keyUsageDashboard.refreshFailed') }}
      </div>

      <UsageStatsCards :data="dashboard" :metric="metric" />
      <UsageAlerts :alerts="dashboard.alerts || []" @select="selectKey" />

      <div class="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <UsageHeatmap
          :keys="dashboard.keys || []"
          :series-labels="dashboard.series_labels || []"
          :range="range"
          :metric="metric"
          :selected-key-id="selectedKeyId"
          @select="selectKey"
        />
        <CurrentKeyPanel
          :api-key-id="selectedKeyId"
          :key-name="selectedKey?.name"
          :group-name="selectedKey?.group_name"
          :params="params"
          :metric="metric"
        />
      </div>

      <TokenUsageTrendChart
        :keys="dashboard.keys || []"
        :series-labels="dashboard.series_labels || []"
        :metric="metric"
      />

      <div class="grid items-start gap-4 xl:grid-cols-2">
        <WeeklyUsageHeatmap :week="dashboard.week || []" :range="range" :metric="metric" />
        <KeyModelHeatmap
          :keys="dashboard.keys || []"
          :models="dashboard.models || { names: [], keys: [] }"
          :metric="metric"
        />
      </div>

      <KeyUsageDetailsTable
        :keys="dashboard.keys || []"
        :metric="metric"
        :selected-key-id="selectedKeyId"
        @select="selectKey"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { userGroupsAPI } from '@/api'
import {
  getKeyUsageDashboard,
  type KeyUsageDashboardMetric,
  type KeyUsageDashboardParams,
  type KeyUsageDashboardRange,
  type KeyUsageDashboardResponse
} from '@/api/keyUsageDashboard'
import Icon from '@/components/icons/Icon.vue'
import UsageDashboardFilters from './UsageDashboardFilters.vue'
import UsageStatsCards from './UsageStatsCards.vue'
import UsageAlerts from './UsageAlerts.vue'
import UsageHeatmap from './UsageHeatmap.vue'
import CurrentKeyPanel from './CurrentKeyPanel.vue'
import TokenUsageTrendChart from './TokenUsageTrendChart.vue'
import WeeklyUsageHeatmap from './WeeklyUsageHeatmap.vue'
import KeyModelHeatmap from './KeyModelHeatmap.vue'
import KeyUsageDetailsTable from './KeyUsageDetailsTable.vue'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  initialKeyId?: number
}>(), {
  initialKeyId: undefined
})

const range = ref<KeyUsageDashboardRange>('7d')
const metric = ref<KeyUsageDashboardMetric>('cost')
const groupId = ref<number | undefined>()
const model = ref<string | undefined>()
const dashboard = ref<KeyUsageDashboardResponse | null>(null)
const selectedKeyId = ref<number | undefined>(props.initialKeyId)
const loading = ref(false)
const error = ref(false)
const groupOptions = ref<Array<{ value: string | number | boolean | null; label: string }>>([])
let controller: AbortController | undefined
let generation = 0

const params = computed<KeyUsageDashboardParams>(() => ({
  range: range.value,
  ...(groupId.value ? { group_id: groupId.value } : {}),
  ...(model.value ? { model: model.value } : {})
}))

const selectedKey = computed(() =>
  dashboard.value?.keys.find((key) => key.api_key_id === selectedKeyId.value)
)

const modelOptions = computed(() => {
  const names = new Set(dashboard.value?.models?.names || [])
  if (model.value) names.add(model.value)
  return [
    { value: '', label: t('keyUsageDashboard.filters.allModels') },
    ...[...names].map((name) => ({ value: name, label: name }))
  ]
})

function isAbortError(value: unknown): boolean {
  if (!value || typeof value !== 'object') return false
  const { name, code } = value as { name?: string; code?: string }
  return name === 'AbortError' || code === 'ERR_CANCELED'
}

async function loadDashboard(): Promise<void> {
  controller?.abort()
  const current = ++generation
  controller = new AbortController()
  loading.value = true
  error.value = false
  try {
    const result = await getKeyUsageDashboard(params.value, { signal: controller.signal })
    if (current !== generation) return
    dashboard.value = result
    const hasSelectedKey = result.keys.some((key) => key.api_key_id === selectedKeyId.value)
    if (!hasSelectedKey) {
      selectedKeyId.value = result.keys[0]?.api_key_id
    }
  } catch (requestError) {
    if (current !== generation || isAbortError(requestError)) return
    error.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}

async function loadGroupOptions(): Promise<void> {
  try {
    const groups = await userGroupsAPI.getAvailable()
    groupOptions.value = [
      { value: '', label: t('keyUsageDashboard.filters.allGroups') },
      ...groups.map((group) => ({ value: group.id, label: group.name }))
    ]
  } catch {
    groupOptions.value = [{ value: '', label: t('keyUsageDashboard.filters.allGroups') }]
  }
}

function selectKey(apiKeyId: number): void {
  selectedKeyId.value = apiKeyId
}

watch(
  [range, groupId, model],
  () => { void loadDashboard() },
  { immediate: true }
)

onMounted(() => {
  void loadGroupOptions()
})

onBeforeUnmount(() => {
  generation++
  controller?.abort()
})
</script>

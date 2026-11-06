<template>
  <section v-if="alerts.length > 0" class="space-y-3" data-testid="usage-alerts">
    <div class="flex items-center gap-2">
      <Icon name="exclamationTriangle" size="sm" class="text-amber-500" />
      <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('keyUsageDashboard.alerts.title') }}
      </h2>
    </div>
    <div class="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
      <button
        v-for="alert in alerts"
        :key="`${alert.type}-${alert.api_key_id}-${alert.count}`"
        type="button"
        class="flex min-w-0 items-start gap-3 rounded-lg border p-3 text-left transition-colors"
        :class="alertClass(alert.type)"
        :data-testid="`usage-alert-${alert.type}`"
        @click="emit('select', alert.api_key_id)"
      >
        <span class="mt-0.5 shrink-0">
          <Icon :name="alertIcon(alert.type)" size="sm" />
        </span>
        <span class="min-w-0">
          <span class="block text-sm font-medium">{{ alertLabel(alert) }}</span>
          <span class="mt-0.5 block truncate text-xs opacity-75">
            {{ alert.api_key_name }}
          </span>
        </span>
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { KeyUsageDashboardAlert } from '@/api/keyUsageDashboard'
import { formatBytes, formatCost, formatPercent } from './utils'

const { t } = useI18n()

const props = defineProps<{
  alerts: KeyUsageDashboardAlert[]
}>()

const emit = defineEmits<{
  select: [apiKeyId: number]
}>()

const alerts = computed(() => props.alerts || [])

function alertClass(type: KeyUsageDashboardAlert['type']): string {
  if (type === 'high_error_rate') {
    return 'border-red-200 bg-red-50 text-red-800 hover:bg-red-100 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200 dark:hover:bg-red-950/50'
  }
  if (type === 'idle') {
    return 'border-gray-200 bg-gray-50 text-gray-700 hover:bg-gray-100 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700'
  }
  return 'border-amber-200 bg-amber-50 text-amber-900 hover:bg-amber-100 dark:border-amber-900/60 dark:bg-amber-950/25 dark:text-amber-200 dark:hover:bg-amber-950/40'
}

function alertIcon(type: KeyUsageDashboardAlert['type']): 'xCircle' | 'document' | 'trendingUp' | 'clock' {
  if (type === 'high_error_rate') return 'xCircle'
  if (type === 'large_request') return 'document'
  if (type === 'usage_spike') return 'trendingUp'
  return 'clock'
}

function alertLabel(alert: KeyUsageDashboardAlert): string {
  if (alert.type === 'high_error_rate') {
    return t('keyUsageDashboard.alerts.highErrorRate', {
      rate: formatPercent(alert.value),
      count: alert.count
    })
  }
  if (alert.type === 'large_request') {
    return t('keyUsageDashboard.alerts.largeRequest', {
      size: formatBytes(alert.value),
      count: alert.count
    })
  }
  if (alert.type === 'usage_spike') {
    return t('keyUsageDashboard.alerts.usageSpike', {
      cost: formatCost(alert.value)
    })
  }
  return t('keyUsageDashboard.alerts.idle', { count: alert.count })
}
</script>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import QuotaNotifyToggle from './QuotaNotifyToggle.vue'
import { useAppStore } from '@/stores/app'
import {
  QUOTA_WEEKLY_DEFAULT_RESET_DAY,
  QUOTA_WEEKLY_DEFAULT_RESET_HOUR,
  defaultQuotaResetTimezone,
} from '@/constants/account'
import type { QuotaThresholdType, QuotaResetMode } from '@/constants/account'

const { t } = useI18n()
const appStore = useAppStore()

const props = defineProps<{
  dim: 'daily' | 'weekly' | 'total'
  label: string
  limit: number | null
  quotaNotifyGlobalEnabled: boolean
  notifyEnabled: boolean | null
  notifyThreshold: number | null
  notifyThresholdType: QuotaThresholdType | null
  // Reset mode (only for daily/weekly, null for total)
  resetMode: QuotaResetMode | null
  resetHour: number | null
  resetDay: number | null  // weekly only
  resetTimezone: string | null
  hintRolling: string
  hintFixed: string
  // Shared options passed from parent
  hourOptions: number[]
  dayOptions: { value: number; key: string }[]
  timezoneOptions?: string[]
}>()

const emit = defineEmits<{
  'update:limit': [value: number | null]
  'update:notifyEnabled': [value: boolean | null]
  'update:notifyThreshold': [value: number | null]
  'update:notifyThresholdType': [value: QuotaThresholdType | null]
  'update:resetMode': [value: QuotaResetMode | null]
  'update:resetHour': [value: number | null]
  'update:resetDay': [value: number | null]
  'update:resetTimezone': [value: string | null]
}>()

const hasResetMode = props.dim !== 'total'

// 固定重置的缺省时区跟服务器项目时区走（public settings 下发），拿不到时退回 UTC
const defaultTimezone = computed(() =>
  defaultQuotaResetTimezone(appStore.cachedPublicSettings?.server_timezone)
)

const limitEnabled = (limit: number | null | undefined) => limit != null && limit > 0

// 周限额默认按自然周（固定 / 周一 / 00:00 / 服务器时区）重置：
// - 还没设周限额、也没选过重置方式时，界面直接预览这套默认（此时不写库，没有副作用）；
// - 已设周限额却没有重置方式的，是迁移 239 之前留下的滚动窗口老账号，如实显示「滚动」；
// - 用户显式选过的方式永远优先。
const effectiveResetMode = computed<QuotaResetMode>(() => {
  if (props.resetMode) return props.resetMode
  if (props.dim === 'weekly' && !limitEnabled(props.limit)) return 'fixed'
  return 'rolling'
})

const applyWeeklyNaturalWeekDefault = () => {
  emit('update:resetMode', 'fixed')
  if (props.resetDay == null) emit('update:resetDay', QUOTA_WEEKLY_DEFAULT_RESET_DAY)
  if (props.resetHour == null) emit('update:resetHour', QUOTA_WEEKLY_DEFAULT_RESET_HOUR)
  if (!props.resetTimezone) emit('update:resetTimezone', defaultTimezone.value)
}

const onLimitInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  const next = Number.isNaN(raw) ? null : raw
  emit('update:limit', next)
  // 周限额从「未启用」变成「已启用」、且从没选过重置方式 → 把预览中的自然周默认真正写进表单
  if (props.dim === 'weekly' && props.resetMode == null && limitEnabled(next) && !limitEnabled(props.limit)) {
    applyWeeklyNaturalWeekDefault()
  }
}

const onModeChange = (e: Event) => {
  const val = (e.target as HTMLSelectElement).value as QuotaResetMode
  emit('update:resetMode', val)
  if (val === 'fixed') {
    if (props.resetHour == null) emit('update:resetHour', 0)
    if (props.dim === 'weekly' && props.resetDay == null) emit('update:resetDay', QUOTA_WEEKLY_DEFAULT_RESET_DAY)
    if (!props.resetTimezone) emit('update:resetTimezone', defaultTimezone.value)
  }
}

function getTimezoneOffsetLabel(tz: string): string {
  try {
    const dtf = new Intl.DateTimeFormat('en-US', { timeZone: tz, timeZoneName: 'shortOffset' })
    const parts = dtf.formatToParts(new Date())
    const tzPart = parts.find(p => p.type === 'timeZoneName')
    return tzPart ? (tzPart.value === 'GMT' ? 'GMT+0' : tzPart.value) : ''
  } catch {
    return ''
  }
}
</script>

<template>
  <div>
    <!-- Title row (only when global notify is enabled) -->
    <div v-if="quotaNotifyGlobalEnabled" class="flex items-center gap-2 mb-1">
      <span class="text-xs font-medium text-gray-700 dark:text-gray-300 flex-1 min-w-0">{{ label }}</span>
      <span v-if="limit && limit > 0" class="text-xs font-medium text-gray-700 dark:text-gray-300 flex-1 min-w-0">{{ t('admin.accounts.quotaNotify.alert') }}</span>
    </div>
    <label v-else class="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1 block">{{ label }}</label>

    <!-- Input row -->
    <div class="flex items-center gap-2">
      <div :class="['relative', quotaNotifyGlobalEnabled ? 'flex-1 min-w-0' : 'flex-1']">
        <span class="absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400 text-sm">$</span>
        <input :value="limit" @input="onLimitInput" type="number" min="0" step="0.01" class="input pl-6 py-1.5 text-sm" :placeholder="t('admin.accounts.quotaLimitPlaceholder')" :data-testid="`quota-${dim}-limit`" />
      </div>
      <QuotaNotifyToggle
        v-if="quotaNotifyGlobalEnabled && limit && limit > 0"
        class="flex-1 min-w-0"
        :enabled="notifyEnabled" :threshold="notifyThreshold" :threshold-type="notifyThresholdType"
        @update:enabled="emit('update:notifyEnabled', $event)" @update:threshold="emit('update:notifyThreshold', $event)" @update:threshold-type="emit('update:notifyThresholdType', $event)"
      />
    </div>

    <!-- Reset mode row (daily/weekly only) -->
    <div v-if="hasResetMode" class="mt-1 flex items-center gap-2 flex-wrap">
      <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetMode') }}</label>
      <select :value="effectiveResetMode" @change="onModeChange" class="input py-1 text-xs w-auto" :data-testid="`quota-${dim}-reset-mode`">
        <option value="rolling">{{ t('admin.accounts.quotaResetModeRolling') }}</option>
        <option value="fixed">{{ t('admin.accounts.quotaResetModeFixed') }}</option>
      </select>
      <template v-if="effectiveResetMode === 'fixed'">
        <!-- Weekly: day of week selector -->
        <template v-if="dim === 'weekly'">
          <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaWeeklyResetDay') }}</label>
          <select :value="resetDay ?? QUOTA_WEEKLY_DEFAULT_RESET_DAY" @change="emit('update:resetDay', Number(($event.target as HTMLSelectElement).value))" class="input py-1 text-xs w-28" :data-testid="`quota-${dim}-reset-day`">
            <option v-for="d in dayOptions" :key="d.value" :value="d.value">{{ t('admin.accounts.dayOfWeek.' + d.key) }}</option>
          </select>
        </template>
        <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetHour') }}</label>
        <select :value="resetHour ?? 0" @change="emit('update:resetHour', Number(($event.target as HTMLSelectElement).value))" class="input py-1 text-xs w-24" :data-testid="`quota-${dim}-reset-hour`">
          <option v-for="h in hourOptions" :key="h" :value="h">{{ String(h).padStart(2, '0') }}:00</option>
        </select>
        <template v-if="timezoneOptions && timezoneOptions.length > 0">
          <select :value="resetTimezone || defaultTimezone" @change="emit('update:resetTimezone', ($event.target as HTMLSelectElement).value)" class="input py-1 text-xs w-auto" :data-testid="`quota-${dim}-reset-timezone`">
            <option v-for="tz in timezoneOptions" :key="tz" :value="tz">{{ tz }} ({{ getTimezoneOffsetLabel(tz) }})</option>
          </select>
        </template>
      </template>
      <span class="text-[11px] text-gray-500 dark:text-gray-400">
        <template v-if="effectiveResetMode === 'fixed'">{{ hintFixed }}</template>
        <template v-else>{{ hintRolling }}</template>
      </span>
    </div>

    <!-- Total dimension hint (no reset mode) -->
    <p v-if="!hasResetMode" class="input-hint mb-0 text-[11px]">{{ hintRolling }}</p>
  </div>
</template>

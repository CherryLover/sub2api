<template>
  <div
    data-test="matrix-scroll"
    class="relative w-full overflow-x-auto overflow-y-auto rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
    :style="{ maxHeight: maxHeight }"
  >
    <table class="w-max border-separate border-spacing-0 text-sm">
      <thead>
        <tr>
          <th
            data-test="account-header"
            scope="col"
            class="sticky left-0 top-0 z-30 w-[176px] min-w-[176px] max-w-[176px] border-b border-r border-gray-200 bg-gray-50 px-3 py-2 text-left align-bottom text-xs font-semibold text-gray-600 dark:border-dark-700 dark:bg-dark-700 dark:text-gray-300 sm:w-[240px] sm:min-w-[240px] sm:max-w-[240px]"
          >
            <span class="block truncate whitespace-nowrap">{{ t('admin.accounts.modelAvailability.matrix.accountColumn') }}</span>
          </th>
          <th
            v-for="column in columns"
            :key="column.model"
            scope="col"
            data-test="model-header"
            :title="columnTitle(column)"
            class="sticky top-0 z-20 w-[150px] min-w-[150px] max-w-[150px] border-b border-gray-200 bg-gray-50 px-3 py-2 text-left align-bottom text-xs font-semibold text-gray-600 dark:border-dark-700 dark:bg-dark-700 dark:text-gray-300"
          >
            <span class="block truncate whitespace-nowrap">{{ columnLabel(column) }}</span>
            <span
              v-if="column.limitedCount > 0"
              class="mt-0.5 block truncate whitespace-nowrap text-[11px] font-normal text-amber-600 dark:text-amber-400"
            >
              {{ t('admin.accounts.modelAvailability.matrix.limitedAccounts', { count: column.limitedCount }) }}
            </span>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="row in rows"
          :key="row.id"
          :data-test="`matrix-row-${row.id}`"
          :class="row.accountBlocked ? 'bg-gray-100/80 dark:bg-dark-700/50' : 'bg-white dark:bg-dark-800'"
        >
          <th
            scope="row"
            data-test="account-cell"
            :class="[
              'sticky left-0 z-10 w-[176px] min-w-[176px] max-w-[176px] border-b border-r border-gray-200 px-3 py-2 text-left font-normal dark:border-dark-700 sm:w-[240px] sm:min-w-[240px] sm:max-w-[240px]',
              row.accountBlocked ? 'bg-gray-100 dark:bg-dark-700' : 'bg-white dark:bg-dark-800'
            ]"
          >
            <div class="flex items-center gap-1.5">
              <span
                v-if="row.accountBlocked"
                data-test="account-blocked-badge"
                class="badge badge-danger shrink-0 whitespace-nowrap text-[10px]"
                :title="accountBlockTitle(row)"
              >
                {{ t('admin.accounts.modelAvailability.matrix.accountBlocked') }}
              </span>
              <span
                data-test="account-name"
                class="block flex-1 truncate whitespace-nowrap font-medium text-gray-900 dark:text-gray-100"
                :title="row.name"
              >
                {{ row.name }}
              </span>
            </div>
            <div class="mt-0.5 flex items-center gap-1.5">
              <span class="shrink-0 whitespace-nowrap text-[11px] text-gray-500 dark:text-gray-400">
                {{ row.platform }}
              </span>
              <span
                v-if="row.accountBlocked"
                data-test="account-blocked-reason"
                class="min-w-0 flex-1 truncate whitespace-nowrap text-[11px] text-red-600 dark:text-red-400"
                :title="accountBlockTitle(row)"
              >
                {{ accountBlockTitle(row) }}
              </span>
              <span
                v-else-if="row.modelsUnavailable"
                data-test="account-models-unavailable"
                class="min-w-0 flex-1 truncate whitespace-nowrap text-[11px] text-gray-400 dark:text-gray-500"
                :title="t('admin.accounts.modelAvailability.matrix.modelListUnavailable')"
              >
                {{ t('admin.accounts.modelAvailability.matrix.modelListUnavailable') }}
              </span>
            </div>
          </th>

          <td
            v-for="cell in row.cells"
            :key="cell.model"
            :data-test="`cell-${row.id}-${cell.model}`"
            :data-state="cell.state"
            :title="cellTitle(cell, row)"
            class="w-[150px] min-w-[150px] max-w-[150px] border-b border-gray-100 px-2 py-2 align-middle dark:border-dark-700/60"
          >
            <div
              v-if="cell.state === 'limited'"
              class="truncate whitespace-nowrap rounded-md bg-amber-50 px-2 py-1 text-[11px] font-medium text-amber-700 ring-1 ring-inset ring-amber-200 dark:bg-amber-500/10 dark:text-amber-300 dark:ring-amber-500/30"
            >
              {{ cellLimitedText(cell) }}
            </div>
            <!--
              整号不可调度时，这个模型「本身没被限流」并不代表能用。
              继续画成绿勾会让最严重的问题被一片绿色淹没，所以整行的勾一律褪色。
            -->
            <div
              v-else-if="cell.state === 'ok'"
              :class="[
                'truncate whitespace-nowrap rounded-md px-2 py-1 text-center text-[11px] font-medium',
                row.accountBlocked
                  ? 'bg-gray-200/60 text-gray-400 dark:bg-dark-600/50 dark:text-gray-500'
                  : 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400'
              ]"
            >
              ✓
            </div>
            <div
              v-else
              class="truncate whitespace-nowrap px-2 py-1 text-center text-[11px] text-gray-300 dark:text-gray-600"
            >
              {{ cell.state === 'unknown' ? '?' : '·' }}
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import { formatDate } from '@/utils/format'

import {
  ACCOUNT_BLOCK_I18N_KEYS,
  MODEL_BLOCK_I18N_KEYS,
  resolveReason,
  type MatrixCell,
  type MatrixColumn,
  type MatrixRow
} from './modelAvailability'

withDefaults(
  defineProps<{
    columns: MatrixColumn[]
    rows: MatrixRow[]
    maxHeight?: string
  }>(),
  { maxHeight: '68vh' }
)

const { t } = useI18n()

/** 解除时间统一按「月-日 时:分」显示，列宽有限，年份没有信息量 */
const shortTime = (value: string | null): string =>
  formatDate(value, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })

const columnLabel = (column: MatrixColumn): string => (column.labelKey ? t(column.labelKey) : column.model)

const columnTitle = (column: MatrixColumn): string =>
  column.labelKey ? `${t(column.labelKey)} (${column.model})` : column.model

const blockSourceText = (source: string): string => {
  const key = ACCOUNT_BLOCK_I18N_KEYS[source] ?? MODEL_BLOCK_I18N_KEYS[source]
  // 后端新增来源时没有文案也要显示原始来源，不能整条吞掉
  return key ? t(key) : source
}

const accountBlockTitle = (row: MatrixRow): string => {
  if (!row.accountBlocked) return ''
  const parts = row.accountBlocks.map((block) => {
    const label = blockSourceText(block.source)
    return block.until ? `${label}（${shortTime(block.until)}）` : label
  })
  return `${t('admin.accounts.modelAvailability.matrix.accountBlockedHint')}: ${parts.join(' / ')}`
}

const reasonText = (reason: string | null): string => {
  const resolved = resolveReason(reason)
  if (resolved.i18nKey) return t(resolved.i18nKey)
  return resolved.raw ?? ''
}

const cellLimitedText = (cell: MatrixCell): string => {
  if (!cell.until) return t('admin.accounts.modelAvailability.matrix.limited')
  return t('admin.accounts.modelAvailability.matrix.limitedUntil', { time: shortTime(cell.until) })
}

const cellTitle = (cell: MatrixCell, row: MatrixRow): string => {
  if (cell.state === 'unsupported') return t('admin.accounts.modelAvailability.matrix.unsupportedHint')
  if (cell.state === 'unknown') return t('admin.accounts.modelAvailability.matrix.unknownHint')
  if (cell.state === 'ok') {
    // 整号停调时别告诉运维「可用」，那是错的
    if (row.accountBlocked) return `${cell.model}\n${accountBlockTitle(row)}`
    return `${cell.model} · ${t('admin.accounts.modelAvailability.matrix.available')}`
  }

  const lines = cell.details.map((detail) => {
    const reason = reasonText(detail.reason)
    const until = detail.until
      ? t('admin.accounts.modelAvailability.matrix.limitedUntil', { time: shortTime(detail.until) })
      : t('admin.accounts.modelAvailability.matrix.limited')
    const source = t('admin.accounts.modelAvailability.matrix.blockSourceLabel', {
      source: blockSourceText(detail.source)
    })
    return reason ? `${until} · ${reason} · ${source}` : `${until} · ${source}`
  })
  return [cell.model, ...lines].join('\n')
}
</script>

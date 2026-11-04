<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { listUpstreamErrorRequests, type OpsUpstreamErrorRequest, type OpsUpstreamErrorRequestParams } from '@/api/admin/ops'
import { formatDateTime } from './ops/utils/opsFormatters'

const { t } = useI18n()
const items = ref<OpsUpstreamErrorRequest[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const loadError = ref(false)
const hours = ref('24')
const status = ref('')
const outcome = ref('')
const keyId = ref('')
const accountId = ref('')
const selected = ref<OpsUpstreamErrorRequest | null>(null)
let controller: AbortController | undefined
let generation = 0
const prefix = 'admin.upstreamErrors.'
const columns = computed(() => [
  { key: 'created_at', label: t(prefix + 'time') },
  { key: 'api_key', label: t(prefix + 'key') },
  { key: 'account', label: t(prefix + 'failedAccount') },
  { key: 'model', label: t('usage.model') },
  { key: 'errors', label: t(prefix + 'events') },
  { key: 'outcome', label: t(prefix + 'outcome') },
  { key: 'final', label: t(prefix + 'finalAccount') },
  { key: 'actions', label: t(prefix + 'details') }
])
const timeOptions = computed(() => [1, 6, 24, 72, 168].map(n => ({ value: String(n), label: t(prefix + 'hours', { n }) })))
const statusOptions = computed(() => [{ value: '', label: '502 / 503' }, { value: '502', label: '502' }, { value: '503', label: '503' }])
const outcomeOptions = computed(() => [
  { value: '', label: t(prefix + 'allOutcomes') },
  { value: 'recovered', label: t(prefix + 'recovered') },
  { value: 'failed', label: t(prefix + 'failed') }
])
const accountLabel = (id: number | null | undefined, name?: string) => id ? `${name || '#'}${name ? ' #' : ''}${id}` : t(prefix + 'notRecorded')
const statusCodes = (row: OpsUpstreamErrorRequest) => [...new Set(row.attempts.map(a => a.upstream_status_code).filter(Boolean))].join(' / ') || '—'
const outcomeClass = (row: OpsUpstreamErrorRequest) => row.outcome === 'recovered'
  ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300'
  : 'bg-rose-100 text-rose-800 dark:bg-rose-900/30 dark:text-rose-300'

async function fetchData() {
  controller?.abort()
  const current = ++generation
  controller = new AbortController()
  loading.value = true
  loadError.value = false
  const end = new Date()
  const params: OpsUpstreamErrorRequestParams = {
    page: page.value, page_size: pageSize.value,
    start_time: new Date(end.getTime() - Number(hours.value) * 3600000).toISOString(),
    end_time: end.toISOString()
  }
  if (status.value) params.status_code = Number(status.value)
  if (outcome.value) params.outcome = outcome.value as 'recovered' | 'failed'
  if (Number(keyId.value) > 0) params.api_key_id = Number(keyId.value)
  if (Number(accountId.value) > 0) params.account_id = Number(accountId.value)
  try {
    const result = await listUpstreamErrorRequests(params, { signal: controller.signal })
    if (current !== generation) return
    items.value = result.items || []
    total.value = result.total
  } catch {
    if (current !== generation) return
    items.value = []
    total.value = 0
    loadError.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}
function search() { page.value = 1; fetchData() }
function changePage(value: number) { page.value = value; fetchData() }
function changePageSize(value: number) { pageSize.value = value; search() }
onMounted(fetchData)
onBeforeUnmount(() => { generation++; controller?.abort() })
</script>

<template>
  <AppLayout>
    <div class="space-y-4">
      <section class="card p-4 sm:p-6">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t(prefix + 'title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t(prefix + 'description') }}</p>
        <form class="mt-5 flex flex-wrap items-end gap-3" @submit.prevent="search">
          <label class="min-w-[140px] flex-1"><span class="input-label">{{ t(prefix + 'range') }}</span><Select v-model="hours" :options="timeOptions" /></label>
          <label class="min-w-[120px] flex-1"><span class="input-label">{{ t(prefix + 'status') }}</span><Select v-model="status" :options="statusOptions" /></label>
          <label class="min-w-[140px] flex-1"><span class="input-label">{{ t(prefix + 'outcome') }}</span><Select v-model="outcome" :options="outcomeOptions" /></label>
          <label class="min-w-[120px] flex-1"><span class="input-label">{{ t(prefix + 'keyId') }}</span><input v-model="keyId" type="number" min="1" step="1" class="input" /></label>
          <label class="min-w-[120px] flex-1"><span class="input-label">{{ t(prefix + 'accountId') }}</span><input v-model="accountId" type="number" min="1" step="1" class="input" /></label>
          <button type="submit" class="btn btn-primary" :disabled="loading">{{ t('common.search') }}</button>
          <button type="button" class="btn btn-secondary" :disabled="loading" @click="fetchData">{{ t('common.refresh') }}</button>
        </form>
        <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ t(prefix + 'retentionHint') }}</p>
      </section>
      <div v-if="loadError" role="alert" class="rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800 dark:border-rose-800 dark:bg-rose-950/30 dark:text-rose-300">
        {{ t(prefix + 'loadFailed') }}
        <button type="button" class="ml-2 underline" @click="fetchData">{{ t('common.refresh') }}</button>
      </div>
      <div class="card overflow-hidden">
        <DataTable :columns="columns" :data="items" :loading="loading" row-key="id">
          <template #cell-created_at="{ row }"><div class="whitespace-nowrap text-sm">{{ formatDateTime(row.created_at) }}</div></template>
          <template #cell-api_key="{ row }"><span class="break-all">{{ accountLabel(row.api_key_id, row.api_key_name) }}</span></template>
          <template #cell-account="{ row }"><span class="break-all">{{ accountLabel(row.account_id, row.account_name) }}</span></template>
          <template #cell-errors="{ row }"><div class="font-mono text-rose-600 dark:text-rose-300">{{ statusCodes(row) }}</div><div class="mt-1 text-xs text-gray-500">{{ t(prefix + 'eventCount', { n: row.attempts.length }) }}</div></template>
          <template #cell-outcome="{ row }"><span class="inline-flex whitespace-nowrap rounded px-2 py-1 text-xs font-medium" :class="outcomeClass(row)">{{ t(prefix + row.outcome) }} · {{ row.client_status_code || '—' }}</span></template>
          <template #cell-final="{ row }"><span class="break-all">{{ row.outcome === 'recovered' && row.final ? accountLabel(row.final.account_id, row.final.account_name) : t(prefix + (row.outcome === 'recovered' ? 'notRecorded' : 'noSuccess')) }}</span></template>
          <template #cell-actions="{ row }"><button type="button" class="btn btn-secondary btn-sm whitespace-nowrap" @click="selected = row">{{ t(prefix + 'details') }}</button></template>
          <template #empty><p class="py-10 text-sm text-gray-500">{{ t(prefix + 'empty') }}</p></template>
        </DataTable>
        <Pagination v-if="total > 0" :page="page" :page-size="pageSize" :total="total" @update:page="changePage" @update:page-size="changePageSize" />
      </div>
    </div>
    <BaseDialog :show="selected !== null" :title="t(prefix + 'details')" width="full" @close="selected = null">
      <div v-if="selected" class="space-y-5">
        <dl class="grid gap-3 break-all text-sm sm:grid-cols-2">
          <div><dt class="text-gray-500">{{ t(prefix + 'requestId') }}</dt><dd class="font-mono">{{ selected.client_request_id || selected.request_id || '—' }}</dd></div>
          <div><dt class="text-gray-500">{{ t(prefix + 'key') }}</dt><dd>{{ accountLabel(selected.api_key_id, selected.api_key_name) }}</dd></div>
          <div><dt class="text-gray-500">{{ t(prefix + 'outcome') }}</dt><dd>{{ t(prefix + selected.outcome) }} · HTTP {{ selected.client_status_code || '—' }}</dd></div>
          <div><dt class="text-gray-500">{{ t(prefix + 'finalAccount') }}</dt><dd>{{ selected.outcome === 'recovered' && selected.final ? accountLabel(selected.final.account_id, selected.final.account_name) : t(prefix + (selected.outcome === 'recovered' ? 'notRecorded' : 'noSuccess')) }}</dd></div>
        </dl>
        <p class="text-xs text-gray-500">{{ t(prefix + 'eventsHint') }}</p>
        <ol class="space-y-3">
          <li v-for="(attempt, index) in selected.attempts" :key="`${selected.id}-${index}`" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
            <div class="flex flex-wrap items-center gap-2 text-sm"><span class="font-mono text-gray-400">#{{ index + 1 }}</span><span class="font-medium">{{ accountLabel(attempt.account_id, attempt.account_name) }}</span><span class="font-mono text-rose-600 dark:text-rose-300">{{ attempt.upstream_status_code || '—' }}</span><span class="text-xs text-gray-500">{{ attempt.kind }} · {{ attempt.at_unix_ms ? formatDateTime(new Date(attempt.at_unix_ms).toISOString()) : t(prefix + 'notRecorded') }}</span></div>
            <p v-if="attempt.upstream_url" class="mt-2 break-all font-mono text-xs text-gray-500">{{ attempt.upstream_url }}</p>
            <p v-if="attempt.upstream_request_id" class="mt-1 break-all font-mono text-xs text-gray-500">{{ t(prefix + 'upstreamRequestId') }}: {{ attempt.upstream_request_id }}</p>
            <p v-if="attempt.stage || attempt.scope || attempt.reason" class="mt-2 break-words font-mono text-xs text-gray-500">{{ [attempt.stage, attempt.scope, attempt.reason].filter(Boolean).join(' · ') }}</p>
            <p v-if="attempt.message" class="mt-3 whitespace-pre-wrap break-words text-sm">{{ attempt.message }}</p>
            <pre v-if="attempt.detail" class="mt-3 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded bg-gray-50 p-3 text-xs dark:bg-dark-900">{{ attempt.detail }}</pre>
            <details v-if="attempt.upstream_response_body" class="mt-3 text-xs"><summary class="cursor-pointer">{{ t(prefix + 'responseBody') }}</summary><pre class="mt-2 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded bg-gray-50 p-3 dark:bg-dark-900">{{ attempt.upstream_response_body }}</pre></details>
          </li>
        </ol>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

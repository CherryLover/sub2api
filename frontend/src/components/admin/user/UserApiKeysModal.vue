<template>
  <BaseDialog :show="show" :title="t('admin.users.userApiKeys')" width="wide" @close="handleClose">
    <div v-if="user" class="space-y-4">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
          <span class="text-lg font-medium text-primary-700 dark:text-primary-300">{{ user.email.charAt(0).toUpperCase() }}</span>
        </div>
        <div><p class="font-medium text-gray-900 dark:text-white">{{ user.email }}</p><p class="text-sm text-gray-500 dark:text-dark-400">{{ user.username }}</p></div>
      </div>
      <div v-if="loading" class="flex justify-center py-8"><svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg></div>
      <div v-else-if="apiKeys.length === 0" class="py-8 text-center"><p class="text-sm text-gray-500">{{ t('admin.users.noApiKeys') }}</p></div>
      <div v-else ref="scrollContainerRef" class="max-h-96 space-y-3 overflow-y-auto" @scroll="closeGroupSelector">
        <div v-for="key in apiKeys" :key="key.id" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
          <div class="flex items-start justify-between">
            <div class="min-w-0 flex-1">
              <div class="mb-1 flex items-center gap-2"><span class="font-medium text-gray-900 dark:text-white">{{ key.name }}</span><span :class="['badge text-xs', statusBadgeClass(key.status)]" :data-test="`key-status-${key.id}`">{{ t('keys.status.' + key.status) }}</span><svg v-if="hasIpRules(key)" class="h-3.5 w-3.5 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"><title>{{ t('admin.apiKeys.ipRestrictionEnabled') }}</title><path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" /></svg></div>
              <p class="truncate font-mono text-sm text-gray-500">{{ key.key.substring(0, 20) }}...{{ key.key.substring(key.key.length - 8) }}</p>
            </div>
          </div>
          <div class="mt-3 flex flex-wrap gap-4 text-xs text-gray-500">
            <div class="flex items-center gap-1">
              <span>{{ t('admin.users.group') }}:</span>
              <button
                :ref="(el) => setGroupButtonRef(key.id, el)"
                @click="openGroupSelector(key)"
                class="-mx-1 -my-0.5 flex cursor-pointer items-center gap-1 rounded-md px-1 py-0.5 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                :disabled="updatingKeyIds.has(key.id)"
              >
                <GroupBadge
                  v-if="key.group_id && key.group"
                  :name="key.group.name"
                  :platform="key.group.platform"
                  :rate-multiplier="key.group.rate_multiplier"
                  :peak-rate-enabled="key.group.peak_rate_enabled"
                  :peak-start="key.group.peak_start"
                  :peak-end="key.group.peak_end"
                  :peak-rate-multiplier="key.group.peak_rate_multiplier"
                />
                <span v-else class="text-gray-400 italic">{{ t('admin.users.none') }}</span>
                <svg v-if="updatingKeyIds.has(key.id)" class="h-3 w-3 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                <svg v-else class="h-3 w-3 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M8.25 15L12 18.75 15.75 15m-7.5-6L12 5.25 15.75 9" /></svg>
              </button>
            </div>
            <div class="flex items-center gap-1"><span>{{ t('admin.users.columns.created') }}: {{ formatDateTime(key.created_at) }}</span></div>
          </div>

          <!-- 操作：启停 / IP 规则 / 删除 -->
          <div class="mt-3 flex flex-wrap items-center gap-2 border-t border-gray-100 pt-3 dark:border-dark-700">
            <button
              type="button"
              :class="[
                'rounded-md px-2.5 py-1 text-xs font-medium transition-colors disabled:opacity-60',
                key.status === 'active'
                  ? 'bg-orange-50 text-orange-700 hover:bg-orange-100 dark:bg-orange-900/20 dark:text-orange-300 dark:hover:bg-orange-900/30'
                  : 'bg-green-50 text-green-700 hover:bg-green-100 dark:bg-green-900/20 dark:text-green-300 dark:hover:bg-green-900/30'
              ]"
              :disabled="updatingKeyIds.has(key.id)"
              :data-test="`key-toggle-${key.id}`"
              @click="toggleKeyStatus(key)"
            >
              {{ key.status === 'active' ? t('admin.apiKeys.disable') : t('admin.apiKeys.enable') }}
            </button>
            <button
              type="button"
              :class="[
                'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                ipEditorKeyId === key.id
                  ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                  : 'bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600'
              ]"
              :data-test="`key-ip-rules-${key.id}`"
              @click="toggleIpEditor(key)"
            >
              {{ t('admin.apiKeys.ipRules') }}
            </button>
            <button
              type="button"
              class="rounded-md bg-red-50 px-2.5 py-1 text-xs font-medium text-red-700 transition-colors hover:bg-red-100 dark:bg-red-900/20 dark:text-red-300 dark:hover:bg-red-900/30"
              :data-test="`key-delete-${key.id}`"
              @click="confirmDelete(key)"
            >
              {{ t('common.delete') }}
            </button>
          </div>

          <!-- IP 白/黑名单编辑（每行一个 IP 或 CIDR，留空表示不限制） -->
          <div v-if="ipEditorKeyId === key.id" class="mt-3 space-y-3" :data-test="`ip-editor-${key.id}`">
            <div>
              <label class="input-label">{{ t('admin.apiKeys.ipWhitelist') }}</label>
              <textarea
                v-model="ipForm.ip_whitelist"
                rows="3"
                class="input font-mono text-sm"
                :placeholder="t('admin.apiKeys.ipWhitelistPlaceholder')"
                data-test="ip-whitelist-input"
              />
              <p class="input-hint">{{ t('admin.apiKeys.ipWhitelistHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.apiKeys.ipBlacklist') }}</label>
              <textarea
                v-model="ipForm.ip_blacklist"
                rows="3"
                class="input font-mono text-sm"
                :placeholder="t('admin.apiKeys.ipBlacklistPlaceholder')"
                data-test="ip-blacklist-input"
              />
              <p class="input-hint">{{ t('admin.apiKeys.ipBlacklistHint') }}</p>
            </div>
            <div class="flex justify-end gap-2">
              <button type="button" class="btn btn-secondary btn-sm" @click="closeIpEditor">
                {{ t('common.cancel') }}
              </button>
              <button
                type="button"
                class="btn btn-primary btn-sm"
                :disabled="updatingKeyIds.has(key.id)"
                data-test="ip-rules-save"
                @click="saveIpRules(key)"
              >
                {{ t('admin.apiKeys.saveIpRules') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </BaseDialog>

  <!-- 删除确认 -->
  <ConfirmDialog
    :show="deletingKey !== null"
    :title="t('admin.apiKeys.deleteKey')"
    :message="t('admin.apiKeys.deleteConfirmMessage', { name: deletingKey?.name ?? '', email: user?.email ?? '' })"
    :confirm-text="t('common.delete')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="handleDelete"
    @cancel="deletingKey = null"
  />

  <!-- Group Selector Dropdown -->
  <Teleport to="body">
    <div
      v-if="groupSelectorKeyId !== null && dropdownPosition"
      ref="dropdownRef"
      class="animate-in fade-in slide-in-from-top-2 fixed z-[100000020] w-64 overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 duration-200 dark:bg-dark-800 dark:ring-white/10"
      :style="{ top: dropdownPosition.top + 'px', left: dropdownPosition.left + 'px' }"
    >
      <div class="max-h-64 overflow-y-auto p-1.5">
        <!-- Unbind option -->
        <button
          @click="changeGroup(selectedKeyForGroup!, null)"
          :class="[
            'flex w-full items-center rounded-lg px-3 py-2 text-sm transition-colors',
            !selectedKeyForGroup?.group_id
              ? 'bg-primary-50 dark:bg-primary-900/20'
              : 'hover:bg-gray-100 dark:hover:bg-dark-700'
          ]"
        >
          <span class="text-gray-500 italic">{{ t('admin.users.none') }}</span>
          <svg
            v-if="!selectedKeyForGroup?.group_id"
            class="ml-auto h-4 w-4 shrink-0 text-primary-600 dark:text-primary-400"
            fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"
          ><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
        </button>
        <!-- Group options -->
        <button
          v-for="group in allGroups"
          :key="group.id"
          @click="changeGroup(selectedKeyForGroup!, group.id)"
          :class="[
            'flex w-full items-center justify-between rounded-lg px-3 py-2 text-sm transition-colors',
            selectedKeyForGroup?.group_id === group.id
              ? 'bg-primary-50 dark:bg-primary-900/20'
              : 'hover:bg-gray-100 dark:hover:bg-dark-700'
          ]"
        >
          <GroupOptionItem
            :name="group.name"
            :platform="group.platform"
            :rate-multiplier="group.rate_multiplier"
            :peak-rate-enabled="group.peak_rate_enabled"
            :peak-start="group.peak_start"
            :peak-end="group.peak_end"
            :peak-rate-multiplier="group.peak_rate_multiplier"
            :description="group.description"
            :selected="selectedKeyForGroup?.group_id === group.id"
          />
        </button>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, type ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import type { AdminUser, AdminGroup, ApiKey } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
const emit = defineEmits(['close'])
const { t } = useI18n()
const appStore = useAppStore()

const apiKeys = ref<ApiKey[]>([])
const allGroups = ref<AdminGroup[]>([])
const loading = ref(false)
const updatingKeyIds = ref(new Set<number>())
const groupSelectorKeyId = ref<number | null>(null)
const dropdownPosition = ref<{ top: number; left: number } | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const scrollContainerRef = ref<HTMLElement | null>(null)
const groupButtonRefs = ref<Map<number, HTMLElement>>(new Map())

const selectedKeyForGroup = computed(() => {
  if (groupSelectorKeyId.value === null) return null
  return apiKeys.value.find((k) => k.id === groupSelectorKeyId.value) || null
})

const setGroupButtonRef = (keyId: number, el: Element | ComponentPublicInstance | null) => {
  if (el instanceof HTMLElement) {
    groupButtonRefs.value.set(keyId, el)
  } else {
    groupButtonRefs.value.delete(keyId)
  }
}

watch(() => props.show, (v) => {
  if (v && props.user) {
    load()
    loadGroups()
  } else {
    closeGroupSelector()
    closeIpEditor()
    deletingKey.value = null
  }
})

const load = async () => {
  if (!props.user) return
  loading.value = true
  groupButtonRefs.value.clear()
  try {
    const res = await adminAPI.users.getUserApiKeys(props.user.id)
    apiKeys.value = res.items || []
  } catch (error) {
    console.error('Failed to load API keys:', error)
  } finally {
    loading.value = false
  }
}

const loadGroups = async () => {
  try {
    const groups = await adminAPI.groups.getAll()
    allGroups.value = groups
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

const DROPDOWN_HEIGHT = 272 // max-h-64 = 16rem = 256px + padding
const DROPDOWN_GAP = 4

const openGroupSelector = (key: ApiKey) => {
  if (groupSelectorKeyId.value === key.id) {
    closeGroupSelector()
  } else {
    const buttonEl = groupButtonRefs.value.get(key.id)
    if (buttonEl) {
      const rect = buttonEl.getBoundingClientRect()
      const spaceBelow = window.innerHeight - rect.bottom
      const openUpward = spaceBelow < DROPDOWN_HEIGHT && rect.top > spaceBelow
      dropdownPosition.value = {
        top: openUpward ? rect.top - DROPDOWN_HEIGHT - DROPDOWN_GAP : rect.bottom + DROPDOWN_GAP,
        left: rect.left
      }
    }
    groupSelectorKeyId.value = key.id
  }
}

const closeGroupSelector = () => {
  groupSelectorKeyId.value = null
  dropdownPosition.value = null
}

const changeGroup = async (key: ApiKey, newGroupId: number | null) => {
  closeGroupSelector()
  if (key.group_id === newGroupId || (!key.group_id && newGroupId === null)) return

  updatingKeyIds.value.add(key.id)
  try {
    const result = await adminAPI.apiKeys.updateApiKeyGroup(key.id, newGroupId)
    // Update local data
    const idx = apiKeys.value.findIndex((k) => k.id === key.id)
    if (idx !== -1) {
      apiKeys.value[idx] = result.api_key
    }
    if (result.auto_granted_group_access && result.granted_group_name) {
      appStore.showSuccess(t('admin.users.groupChangedWithGrant', { group: result.granted_group_name }))
    } else {
      appStore.showSuccess(t('admin.users.groupChangedSuccess'))
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.users.groupChangeFailed'))
  } finally {
    updatingKeyIds.value.delete(key.id)
  }
}

// ---------- 启停 / IP 规则 / 删除 ----------
const statusBadgeClass = (status: ApiKey['status']) => {
  if (status === 'active') return 'badge-success'
  if (status === 'quota_exhausted') return 'badge-warning'
  if (status === 'expired') return 'badge-danger'
  return 'badge-gray'
}

const hasIpRules = (key: ApiKey) =>
  (key.ip_whitelist?.length ?? 0) > 0 || (key.ip_blacklist?.length ?? 0) > 0

const replaceKey = (updated: ApiKey) => {
  const idx = apiKeys.value.findIndex((k) => k.id === updated.id)
  if (idx !== -1) {
    apiKeys.value[idx] = updated
  }
}

const toggleKeyStatus = async (key: ApiKey) => {
  const newStatus = key.status === 'active' ? 'inactive' : 'active'
  updatingKeyIds.value.add(key.id)
  try {
    const result = await adminAPI.apiKeys.update(key.id, { status: newStatus })
    replaceKey(result.api_key)
    appStore.showSuccess(
      newStatus === 'active' ? t('admin.apiKeys.keyEnabled') : t('admin.apiKeys.keyDisabled')
    )
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.apiKeys.failedToUpdate'))
  } finally {
    updatingKeyIds.value.delete(key.id)
  }
}

const ipEditorKeyId = ref<number | null>(null)
const ipForm = ref({ ip_whitelist: '', ip_blacklist: '' })

const toggleIpEditor = (key: ApiKey) => {
  if (ipEditorKeyId.value === key.id) {
    closeIpEditor()
    return
  }
  closeGroupSelector()
  ipForm.value = {
    ip_whitelist: (key.ip_whitelist || []).join('\n'),
    ip_blacklist: (key.ip_blacklist || []).join('\n')
  }
  ipEditorKeyId.value = key.id
}

const closeIpEditor = () => {
  ipEditorKeyId.value = null
}

const parseIPList = (text: string): string[] =>
  text.split('\n').map((ip) => ip.trim()).filter((ip) => ip.length > 0)

const saveIpRules = async (key: ApiKey) => {
  updatingKeyIds.value.add(key.id)
  try {
    const result = await adminAPI.apiKeys.update(key.id, {
      ip_whitelist: parseIPList(ipForm.value.ip_whitelist),
      ip_blacklist: parseIPList(ipForm.value.ip_blacklist)
    })
    replaceKey(result.api_key)
    appStore.showSuccess(t('admin.apiKeys.keyUpdated'))
    closeIpEditor()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.apiKeys.failedToUpdate'))
  } finally {
    updatingKeyIds.value.delete(key.id)
  }
}

const deletingKey = ref<ApiKey | null>(null)

const confirmDelete = (key: ApiKey) => {
  closeGroupSelector()
  deletingKey.value = key
}

const handleDelete = async () => {
  const key = deletingKey.value
  if (!key) return
  try {
    await adminAPI.apiKeys.remove(key.id)
    apiKeys.value = apiKeys.value.filter((k) => k.id !== key.id)
    if (ipEditorKeyId.value === key.id) closeIpEditor()
    appStore.showSuccess(t('admin.apiKeys.keyDeleted'))
    deletingKey.value = null
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.apiKeys.failedToDelete'))
  }
}

const handleKeyDown = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && groupSelectorKeyId.value !== null) {
    event.stopPropagation()
    closeGroupSelector()
  }
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (dropdownRef.value && !dropdownRef.value.contains(target)) {
    // Check if the click is on one of the group trigger buttons
    for (const el of groupButtonRefs.value.values()) {
      if (el.contains(target)) return
    }
    closeGroupSelector()
  }
}

const handleClose = () => {
  closeGroupSelector()
  emit('close')
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleKeyDown, true)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleKeyDown, true)
})
</script>

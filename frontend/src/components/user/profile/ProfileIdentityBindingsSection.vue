<template>
  <div :class="props.embedded ? 'space-y-4' : 'card overflow-hidden'">
    <div
      v-if="!props.embedded"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.authBindings.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.authBindings.description') }}
      </p>
    </div>

    <div :class="props.embedded ? 'space-y-4' : 'divide-y divide-gray-100 dark:divide-dark-700'">
      <div v-if="props.embedded">
        <p class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('profile.authBindings.title') }}
        </p>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('profile.authBindings.description') }}
        </p>
      </div>

      <div :class="rowClass">
        <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div class="flex min-w-0 flex-1 items-start gap-4">
            <div
              class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-primary-100 text-sm font-semibold text-primary-600 dark:bg-primary-900/20 dark:text-primary-300"
            >
              <Icon name="mail" size="sm" class="text-current" />
            </div>

            <div class="min-w-0 flex-1 space-y-3">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="font-medium text-gray-900 dark:text-white">
                  {{ t('profile.authBindings.providers.email') }}
                </h3>
                <span
                  data-testid="profile-binding-email-status"
                  :class="['badge', emailBound ? 'badge-success' : 'badge-gray']"
                >
                  {{
                    emailBound
                      ? t('profile.authBindings.status.bound')
                      : t('profile.authBindings.status.notBound')
                  }}
                </span>
              </div>

              <p
                v-if="displayableEmail"
                class="text-sm text-gray-600 dark:text-gray-300"
              >
                {{ displayableEmail }}
              </p>

              <div
                v-if="hasBindingDetails(emailDetails)"
                class="grid gap-1 text-sm text-gray-500 dark:text-gray-400"
              >
                <p v-if="bindingCountLabel(emailDetails)">
                  {{ bindingCountLabel(emailDetails) }}
                </p>
                <p v-if="bindingNote(emailDetails)">
                  {{ bindingNote(emailDetails) }}
                </p>
              </div>

              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('profile.authBindings.readOnlyHint') }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { User, UserAuthBindingStatus } from '@/types'

const props = withDefaults(
  defineProps<{
    user: User | null
    embedded?: boolean
    compact?: boolean
  }>(),
  {
    embedded: false,
    compact: false,
  }
)

const { t } = useI18n()

const currentUser = computed(() => props.user)
const compact = computed(() => props.compact)
const rowClass = computed(() =>
  props.embedded
    ? compact.value
      ? 'rounded-2xl border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900/40'
      : 'rounded-2xl border border-gray-100 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-900/30'
    : 'px-6 py-5'
)
const emailBound = computed(() => getEmailBindingStatusForUser(currentUser.value))
const emailDetails = computed(() => {
  const binding =
    currentUser.value?.auth_bindings?.email ?? currentUser.value?.identity_bindings?.email
  if (!binding || typeof binding === 'boolean') {
    return null
  }
  return binding
})
const displayableEmail = computed(() => getDisplayableEmail(currentUser.value))

const legacyBindingNoteKeys: Record<string, string> = {
  'Primary account email is managed from the profile form.':
    'profile.authBindings.notes.emailManagedFromProfile',
}

function normalizeBindingStatus(binding: boolean | UserAuthBindingStatus | undefined): boolean | null {
  if (typeof binding === 'boolean') {
    return binding
  }
  if (!binding) {
    return null
  }
  if (typeof binding.bound === 'boolean') {
    return binding.bound
  }
  return Boolean(binding.provider_subject || binding.issuer || binding.provider_key)
}

function getEmailBindingStatusForUser(user: User | null | undefined): boolean {
  if (typeof user?.email_bound === 'boolean') {
    return user.email_bound
  }
  const nested = user?.auth_bindings?.email ?? user?.identity_bindings?.email
  const normalized = normalizeBindingStatus(nested)
  return normalized ?? false
}

function getDisplayableEmail(user: User | null | undefined): string {
  const email = user?.email?.trim() || ''
  if (!email) {
    return ''
  }
  if (email.endsWith('.invalid') && !getEmailBindingStatusForUser(user)) {
    return ''
  }
  return email
}

function bindingCountLabel(details: UserAuthBindingStatus | null): string {
  if (!details || typeof details.bound_count !== 'number' || details.bound_count <= 1) {
    return ''
  }
  return t('profile.authBindings.boundCount', { count: details.bound_count })
}

function bindingNote(details: UserAuthBindingStatus | null): string {
  if (!details) {
    return ''
  }

  const noteKey = details.note_key?.trim() || legacyBindingNoteKeys[details.note?.trim() || ''] || ''
  if (noteKey) {
    const translated = t(noteKey)
    if (translated !== noteKey) {
      return translated
    }
  }

  return details.note?.trim() || ''
}

function hasBindingDetails(details: UserAuthBindingStatus | null): boolean {
  if (!details) {
    return false
  }
  return Boolean(bindingCountLabel(details) || bindingNote(details))
}
</script>

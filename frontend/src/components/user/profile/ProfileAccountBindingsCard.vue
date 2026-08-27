<template>
  <ProfileIdentityBindingsSection :user="user" />
</template>

<script setup lang="ts">
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import type { User } from '@/types'

defineProps<{
  user: User | null
}>()
</script>

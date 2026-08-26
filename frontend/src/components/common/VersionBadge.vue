<template>
  <!--
    内部部署版本不做应用内更新检查/在线升级/回滚，这里只展示当前版本号。
    版本号来自公开设置（siteVersion），无需管理员额外请求。
  -->
  <span v-if="version" class="text-xs text-gray-500 dark:text-dark-400"> v{{ version }} </span>
</template>

<script setup lang="ts">
defineProps<{
  version?: string
}>()
</script>

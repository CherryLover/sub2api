<template>
  <Teleport to="body">
    <Transition name="side-drawer">
      <div
        v-if="show"
        class="fixed inset-0 flex justify-end bg-black/40 backdrop-blur-[1px]"
        :style="{ zIndex }"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
        data-testid="side-drawer-overlay"
        @click.self="handleOverlayClick"
      >
        <div
          ref="panelRef"
          class="side-drawer-panel flex h-full max-w-full flex-col border-l border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-800"
          :style="{ width }"
          data-testid="side-drawer-panel"
          @click.stop
        >
          <!-- Header -->
          <div class="flex flex-shrink-0 items-start justify-between gap-3 border-b border-gray-200 px-4 py-3 dark:border-dark-700 sm:px-5">
            <div class="min-w-0 flex-1">
              <h3 :id="titleId" class="truncate text-base font-semibold text-gray-900 dark:text-white">
                <slot name="title">{{ title }}</slot>
              </h3>
              <div v-if="$slots.subtitle" class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                <slot name="subtitle" />
              </div>
            </div>
            <div class="flex flex-shrink-0 items-center gap-1">
              <slot name="actions" />
              <button
                type="button"
                class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-dark-300"
                :aria-label="t('common.close')"
                data-testid="side-drawer-close"
                @click="emit('close')"
              >
                <Icon name="x" size="md" />
              </button>
            </div>
          </div>

          <!-- Body -->
          <div ref="bodyRef" class="flex-1 overflow-y-auto px-4 py-3 sm:px-5 sm:py-4">
            <slot />
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="flex flex-shrink-0 items-center justify-end gap-3 border-t border-gray-200 px-4 py-3 dark:border-dark-700 sm:px-5">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
/**
 * 通用右侧抽屉：从屏幕右缘滑入的固定面板，用于「不离开当前列表」地查看某一行的明细。
 * - 遮罩点击 / Esc 关闭（均可通过 prop 关掉）
 * - 打开时锁住 body 滚动，复用 BaseDialog 的 body.modal-open 约定
 * - width 直接传 CSS 宽度值（默认 min(640px, 100vw)）
 */
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

let drawerIdCounter = 0
const titleId = `side-drawer-title-${++drawerIdCounter}`

interface Props {
  show: boolean
  title?: string
  width?: string
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  zIndex?: number
}

const props = withDefaults(defineProps<Props>(), {
  title: '',
  width: 'min(640px, 100vw)',
  closeOnEscape: true,
  closeOnClickOutside: true,
  zIndex: 60
})

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
const panelRef = ref<HTMLElement | null>(null)
const bodyRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null

const handleOverlayClick = () => {
  if (props.closeOnClickOutside) emit('close')
}

const handleEscape = (event: KeyboardEvent) => {
  if (props.show && props.closeOnEscape && event.key === 'Escape') {
    emit('close')
  }
}

watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen) {
      previousActiveElement = document.activeElement as HTMLElement | null
      document.body.classList.add('modal-open')
      await nextTick()
      if (bodyRef.value) bodyRef.value.scrollTop = 0
      const firstFocusable = panelRef.value?.querySelector<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      )
      firstFocusable?.focus()
    } else {
      document.body.classList.remove('modal-open')
      if (previousActiveElement && typeof previousActiveElement.focus === 'function') {
        previousActiveElement.focus()
      }
      previousActiveElement = null
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleEscape)
  document.body.classList.remove('modal-open')
})
</script>

<style scoped>
.side-drawer-enter-active,
.side-drawer-leave-active {
  transition: opacity 200ms ease;
}
.side-drawer-enter-active .side-drawer-panel,
.side-drawer-leave-active .side-drawer-panel {
  transition: transform 220ms ease;
}
.side-drawer-enter-from,
.side-drawer-leave-to {
  opacity: 0;
}
.side-drawer-enter-from .side-drawer-panel,
.side-drawer-leave-to .side-drawer-panel {
  transform: translateX(100%);
}
</style>

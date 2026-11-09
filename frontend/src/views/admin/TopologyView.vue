<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TopologyKeyCard from '@/components/admin/topology/TopologyKeyCard.vue'
import TopologyAccountCard from '@/components/admin/topology/TopologyAccountCard.vue'
import TopologyPlatformCard from '@/components/admin/topology/TopologyPlatformCard.vue'
import {
  getTopologySnapshot,
  openTopologyEventStream,
  type RoutingEvent,
  type TopologySnapshot,
  type TopologyStreamHandle,
  type TopologyStreamStatus
} from '@/api/admin/topology'
import {
  accountStatusView,
  curvePath,
  entryYs,
  foldKeys,
  makeCurve,
  pointOnCurve,
  sortAccounts,
  strategyLabel,
  type Curve,
  type Point
} from '@/components/admin/topology/topologyModel'
import {
  activeBlocked,
  activeDots,
  applyRoutingEvent,
  cooldownLeftSeconds,
  createAnimState,
  fxFor,
  gateVisuals,
  hasPendingAnimation,
  pruneAnimState,
  tagText
} from '@/components/admin/topology/topologyAnimation'

const POLL_MS = 5000

const { t } = useI18n()
const tr = (k: string, p?: Record<string, unknown>) => (p ? t(k, p) : t(k))

// ==================== Snapshot ====================

const snapshot = ref<TopologySnapshot | null>(null)
const loading = ref(true)
const loadError = ref(false)
const refreshError = ref(false)
const clock = ref(Date.now())
let pollTimer: ReturnType<typeof setTimeout> | null = null
let snapshotController: AbortController | null = null
let disposed = false

async function loadSnapshot() {
  if (snapshotController) return
  snapshotController = new AbortController()
  try {
    const data = await getTopologySnapshot(snapshotController.signal)
    if (disposed) return
    snapshot.value = data
    clock.value = Date.now()
    loadError.value = false
    refreshError.value = false
  } catch (err) {
    if (disposed || (err as { code?: string })?.code === 'ERR_CANCELED') return
    if (snapshot.value) refreshError.value = true
    else loadError.value = true
  } finally {
    snapshotController = null
    loading.value = false
  }
}

function schedulePoll() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = null
  if (disposed || document.hidden) return
  pollTimer = setTimeout(async () => {
    pollTimer = null
    await loadSnapshot()
    schedulePoll()
  }, POLL_MS)
}

function retryLoad() {
  loading.value = true
  loadError.value = false
  void loadSnapshot().then(schedulePoll)
}

// ==================== Derived data ====================

const showFolded = ref(false)
const folded = computed(() => foldKeys(snapshot.value?.keys ?? []))
const displayedKeys = computed(() =>
  showFolded.value ? [...folded.value.visible, ...folded.value.folded] : folded.value.visible
)
const accounts = computed(() => sortAccounts(snapshot.value?.accounts ?? []))
const strategy = computed(() => strategyLabel(snapshot.value?.groups ?? [], tr))
const totals = computed(() => snapshot.value?.totals)
const isEmpty = computed(
  () => !!snapshot.value && snapshot.value.keys.length === 0 && snapshot.value.accounts.length === 0
)

// ==================== Live events / animation ====================

const anim = shallowRef(createAnimState())
const now = ref(Date.now())
const streamStatus = ref<TopologyStreamStatus>('connecting')
let stream: TopologyStreamHandle | null = null
let rafId: number | null = null
let reducedMotion = false
let motionQuery: MediaQueryList | null = null

function frame() {
  rafId = null
  const ts = Date.now()
  now.value = ts
  anim.value = pruneAnimState(anim.value, ts)
  if (hasPendingAnimation(anim.value, ts)) rafId = requestAnimationFrame(frame)
}

function ensureLoop() {
  if (rafId == null && !disposed) rafId = requestAnimationFrame(frame)
}

function onRoutingEvent(ev: RoutingEvent) {
  const ts = Date.now()
  anim.value = applyRoutingEvent(anim.value, ev, ts, { reducedMotion })
  now.value = ts
  ensureLoop()
}

function openStream() {
  if (stream || disposed) return
  stream = openTopologyEventStream({
    onEvent: onRoutingEvent,
    onStatus: s => {
      streamStatus.value = s
    }
  })
}

function closeStream() {
  stream?.close()
  stream = null
}

const gates = computed(() => gateVisuals(anim.value, now.value))
const blocked = computed(() => activeBlocked(anim.value, now.value))
const keyFx = (id: number) => fxFor(anim.value, 'key', id, now.value)
const accountFx = (id: number) => fxFor(anim.value, 'account', id, now.value)
const cooldownLeft = (id: number) => cooldownLeftSeconds(anim.value, id, now.value)
const exitTag = computed(() => {
  const fx = fxFor(anim.value, 'exit', 0, now.value)
  return fx?.tag ? { text: tagText(fx.tag, tr), bad: fx.kind === 'giveup' } : null
})

// ==================== Layout measurement ====================

interface Rect {
  left: number
  top: number
  right: number
  bottom: number
}

const stage = ref<HTMLElement | null>(null)
const desktop = ref(false)
const geom = ref<{
  width: number
  height: number
  keys: Record<number, Point>
  folded: Point | null
  accounts: Record<number, Point>
  platform: Rect | null
}>({ width: 0, height: 0, keys: {}, folded: null, accounts: {}, platform: null })

let resizeObserver: ResizeObserver | null = null
let measureQueued = false
let desktopQuery: MediaQueryList | null = null

function relRect(el: Element, base: DOMRect): Rect {
  const r = el.getBoundingClientRect()
  return { left: r.left - base.left, top: r.top - base.top, right: r.right - base.left, bottom: r.bottom - base.top }
}

function measure() {
  measureQueued = false
  const root = stage.value
  if (!root) return
  const base = root.getBoundingClientRect()
  const keys: Record<number, Point> = {}
  root.querySelectorAll<HTMLElement>('[data-topo-key]').forEach(el => {
    const r = relRect(el, base)
    keys[Number(el.dataset.topoKey)] = { x: r.right, y: (r.top + r.bottom) / 2 }
  })
  const accountsPts: Record<number, Point> = {}
  root.querySelectorAll<HTMLElement>('[data-topo-account]').forEach(el => {
    const r = relRect(el, base)
    accountsPts[Number(el.dataset.topoAccount)] = { x: r.left, y: (r.top + r.bottom) / 2 }
  })
  const foldedEl = root.querySelector('[data-topo-folded]')
  const fr = foldedEl ? relRect(foldedEl, base) : null
  const platformEl = root.querySelector('[data-topo-platform]')
  geom.value = {
    width: base.width,
    height: base.height,
    keys,
    folded: fr ? { x: fr.right, y: (fr.top + fr.bottom) / 2 } : null,
    accounts: accountsPts,
    platform: platformEl ? relRect(platformEl, base) : null
  }
  observeCards()
}

function queueMeasure() {
  if (measureQueued) return
  measureQueued = true
  requestAnimationFrame(measure)
}

const observed = new Set<Element>()
function observeCards() {
  if (!resizeObserver || !stage.value) return
  const els = stage.value.querySelectorAll('[data-topo-key],[data-topo-account],[data-topo-platform],[data-topo-folded]')
  const live = new Set<Element>(Array.from(els))
  for (const el of observed) {
    if (!live.has(el)) {
      resizeObserver.unobserve(el)
      observed.delete(el)
    }
  }
  for (const el of live) {
    if (!observed.has(el)) {
      resizeObserver.observe(el)
      observed.add(el)
    }
  }
}

watch([displayedKeys, accounts], () => nextTick(queueMeasure))

// ==================== Curves ====================

const exitPoint = computed<Point | null>(() => {
  const p = geom.value.platform
  return p ? { x: p.right, y: (p.top + p.bottom) / 2 } : null
})

const keyCurves = computed(() => {
  const p = geom.value.platform
  const out = new Map<number, Curve>()
  if (!p) return out
  const ids = displayedKeys.value.map(k => k.id).filter(id => geom.value.keys[id])
  const ys = entryYs(ids.length, p.top, p.bottom)
  ids.forEach((id, i) => out.set(id, makeCurve(geom.value.keys[id], { x: p.left, y: ys[i] })))
  return out
})

const foldedCurve = computed<Curve | null>(() => {
  const p = geom.value.platform
  const f = geom.value.folded
  return p && f ? makeCurve(f, { x: p.left, y: (p.top + p.bottom) / 2 }) : null
})

const accountCurves = computed(() => {
  const out = new Map<number, Curve>()
  const exit = exitPoint.value
  if (!exit) return out
  for (const a of accounts.value) {
    const pt = geom.value.accounts[a.id]
    if (pt) out.set(a.id, makeCurve(exit, pt))
  }
  return out
})

const keyLines = computed(() =>
  displayedKeys.value
    .filter(k => keyCurves.value.has(k.id))
    .map(k => {
      const c = keyCurves.value.get(k.id)!
      return { id: k.id, d: curvePath(c), from: c.from, active: k.in_flight > 0 }
    })
)

const accountLines = computed(() =>
  accounts.value
    .filter(a => accountCurves.value.has(a.id))
    .map(a => {
      const c = accountCurves.value.get(a.id)!
      const cooling = cooldownLeftSeconds(anim.value, a.id, now.value) > 0
      const available = accountStatusView(a, clock.value, tr).available
      const cls = cooling ? 'topo-ln-cool' : !available ? 'topo-ln-off' : a.concurrency_current > 0 ? 'topo-ln-on' : 'topo-ln'
      return { id: a.id, d: curvePath(c), cls }
    })
)

const dots = computed(() => {
  if (!desktop.value) return []
  const out: { id: number; x: number; y: number; err: boolean }[] = []
  for (const { dot, pos } of activeDots(anim.value, now.value)) {
    const curve =
      dot.line.kind === 'key'
        ? keyCurves.value.get(dot.line.keyId) ?? foldedCurve.value
        : accountCurves.value.get(dot.line.accountId)
    if (!curve) continue
    const p = pointOnCurve(curve, pos)
    out.push({ id: dot.id, x: p.x, y: p.y, err: dot.tone === 'err' })
  }
  return out
})

// ==================== Key info popover ====================

const tipKeyId = ref<number | null>(null)
function toggleTip(id: number) {
  tipKeyId.value = tipKeyId.value === id ? null : id
}
function onDocClick(e: MouseEvent) {
  if (tipKeyId.value == null) return
  const target = e.target as Element | null
  if (target?.closest('[data-topo-tip],[data-topo-tip-trigger]')) return
  tipKeyId.value = null
}
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') tipKeyId.value = null
}

// ==================== Lifecycle ====================

function onVisibility() {
  if (document.hidden) {
    if (pollTimer) clearTimeout(pollTimer)
    pollTimer = null
    closeStream()
  } else {
    void loadSnapshot().then(schedulePoll)
    openStream()
  }
}

function onDesktopChange() {
  desktop.value = desktopQuery?.matches ?? true
  nextTick(queueMeasure)
}

function onMotionChange() {
  reducedMotion = motionQuery?.matches ?? false
}

onMounted(() => {
  if (typeof window.matchMedia === 'function') {
    desktopQuery = window.matchMedia('(min-width: 1024px)')
    motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
    desktopQuery.addEventListener?.('change', onDesktopChange)
    motionQuery.addEventListener?.('change', onMotionChange)
  }
  desktop.value = desktopQuery?.matches ?? true
  reducedMotion = motionQuery?.matches ?? false
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => queueMeasure())
  }
  document.addEventListener('visibilitychange', onVisibility)
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKeydown)
  window.addEventListener('resize', queueMeasure)
  void loadSnapshot().then(schedulePoll)
  if (!document.hidden) openStream()
})

watch(stage, el => {
  if (el && resizeObserver) {
    resizeObserver.observe(el)
    nextTick(queueMeasure)
  }
})

onBeforeUnmount(() => {
  disposed = true
  if (pollTimer) clearTimeout(pollTimer)
  snapshotController?.abort()
  closeStream()
  if (rafId != null) cancelAnimationFrame(rafId)
  resizeObserver?.disconnect()
  observed.clear()
  desktopQuery?.removeEventListener?.('change', onDesktopChange)
  motionQuery?.removeEventListener?.('change', onMotionChange)
  document.removeEventListener('visibilitychange', onVisibility)
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeydown)
  window.removeEventListener('resize', queueMeasure)
})

const liveClass = computed(() => {
  switch (streamStatus.value) {
    case 'open':
      return { text: 'text-primary-700 dark:text-primary-300', dot: 'bg-primary-500 topo-live-pulse' }
    case 'closed':
      return { text: 'text-gray-500 dark:text-dark-400', dot: 'bg-gray-400' }
    default:
      return { text: 'text-amber-700 dark:text-amber-400', dot: 'bg-amber-400' }
  }
})
</script>

<template>
  <AppLayout>
    <div class="space-y-4" data-testid="topology-view">
      <!-- Header -->
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="text-xl font-bold text-gray-900 dark:text-white">{{ t('admin.topology.title') }}</h2>
          <p class="mt-1 text-[13px] text-gray-500 dark:text-dark-400">{{ t('admin.topology.description') }}</p>
        </div>
        <div class="flex items-center gap-3 text-[13px]">
          <span class="flex items-center gap-1.5 font-semibold" :class="liveClass.text" data-testid="topology-live">
            <span class="h-2 w-2 rounded-full" :class="liveClass.dot" />
            {{ t('admin.topology.live.' + streamStatus) }}
          </span>
          <div class="flex gap-0.5 rounded-[10px] border border-gray-200 bg-white p-[3px] dark:border-dark-600 dark:bg-dark-800" role="group" :aria-label="t('admin.topology.range.label')">
            <button type="button" class="rounded-lg bg-primary-600 px-3 py-1.5 text-xs font-semibold text-white" aria-pressed="true">{{ t('admin.topology.range.now') }}</button>
            <button type="button" class="cursor-not-allowed rounded-lg px-3 py-1.5 text-xs text-gray-400 dark:text-dark-500" disabled :title="t('admin.topology.range.soon')" aria-pressed="false">{{ t('admin.topology.range.m5') }}</button>
            <button type="button" class="cursor-not-allowed rounded-lg px-3 py-1.5 text-xs text-gray-400 dark:text-dark-500" disabled :title="t('admin.topology.range.soon')" aria-pressed="false">{{ t('admin.topology.range.h1') }}</button>
          </div>
        </div>
      </div>

      <!-- Loading / error / empty -->
      <div v-if="loading && !snapshot" class="card flex items-center justify-center gap-3 p-16 text-sm text-gray-500 dark:text-dark-400" data-testid="topology-loading">
        <span class="h-4 w-4 animate-spin rounded-full border-2 border-primary-500 border-t-transparent" />
        {{ t('admin.topology.state.loading') }}
      </div>
      <div v-else-if="loadError && !snapshot" class="card flex flex-col items-center gap-3 p-16 text-sm" data-testid="topology-error">
        <span class="text-red-600 dark:text-red-400">{{ t('admin.topology.state.loadFailed') }}</span>
        <button type="button" class="btn btn-secondary btn-sm" @click="retryLoad">{{ t('admin.topology.state.retry') }}</button>
      </div>
      <div v-else-if="isEmpty" class="card p-16 text-center text-sm text-gray-500 dark:text-dark-400" data-testid="topology-empty">
        {{ t('admin.topology.state.empty') }}
      </div>

      <!-- Stage -->
      <section v-else-if="snapshot && totals" class="card p-4 sm:p-6">
        <div class="flex flex-wrap items-center gap-x-6 gap-y-1 text-[13px] text-gray-500 dark:text-dark-400" data-testid="topology-totals">
          <span>{{ t('admin.topology.totals.inFlight') }} <b class="text-gray-900 dark:text-white">{{ totals.in_flight }}</b></span>
          <span>{{ t('admin.topology.totals.queued') }} <b class="text-gray-900 dark:text-white">{{ totals.queued }}</b></span>
          <span>{{ t('admin.topology.totals.requests5m') }} <b class="text-gray-900 dark:text-white">{{ totals.requests_5m }}</b> {{ t('admin.topology.totals.times') }}</span>
          <span>{{ t('admin.topology.totals.failed') }} <b class="text-gray-900 dark:text-white">{{ totals.failed_5m }}</b></span>
          <span>{{ t('admin.topology.totals.available') }} <b class="text-primary-700 dark:text-primary-300">{{ totals.accounts_available }}</b> / {{ totals.accounts_total }}</span>
          <span v-if="refreshError" class="text-amber-700 dark:text-amber-400" data-testid="topology-refresh-error">{{ t('admin.topology.state.refreshFailed') }}</span>
        </div>

        <div
          ref="stage"
          class="relative mt-5 grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_minmax(0,1.2fr)] lg:gap-x-10 xl:gap-x-20"
        >
          <!-- connection lines (behind the cards) -->
          <svg
            v-if="desktop && geom.width > 0"
            class="pointer-events-none absolute left-0 top-0 z-0 overflow-visible"
            :width="geom.width"
            :height="geom.height"
            :viewBox="`0 0 ${geom.width} ${geom.height}`"
            aria-hidden="true"
            data-testid="topology-lines"
          >
            <path v-for="l in keyLines" :key="'k' + l.id" :d="l.d" :class="l.active ? 'topo-ln-on' : 'topo-ln'" />
            <path v-for="l in accountLines" :key="'a' + l.id" :d="l.d" :class="l.cls" />
            <circle v-for="l in keyLines" :key="'kc' + l.id" :cx="l.from.x" :cy="l.from.y" r="3.5" class="topo-anchor" :class="l.active ? 'topo-anchor-on' : ''" />
            <circle v-if="exitPoint && accountLines.length" :cx="exitPoint.x" :cy="exitPoint.y" r="5" class="topo-exit" />
          </svg>

          <!-- Left: keys -->
          <div class="relative flex min-w-0 flex-col gap-4" :class="tipKeyId != null ? 'z-30' : 'z-10'">
            <div class="text-xs text-gray-400 dark:text-dark-400">{{ t('admin.topology.columns.keys') }}</div>
            <TopologyKeyCard
              v-for="k in displayedKeys"
              :key="k.id"
              :api-key="k"
              :fx="keyFx(k.id)"
              :tip-open="tipKeyId === k.id"
              :class="tipKeyId === k.id ? 'z-30' : ''"
              @toggle-tip="toggleTip"
              @close-tip="tipKeyId = null"
            />
            <button
              v-if="folded.folded.length > 0"
              type="button"
              class="flex h-11 items-center rounded-xl border border-dashed border-gray-200 px-3.5 text-left text-xs text-gray-400 hover:border-gray-300 hover:text-gray-600 dark:border-dark-600 dark:text-dark-400 dark:hover:text-dark-200"
              data-topo-folded
              data-testid="topology-folded"
              @click="showFolded = !showFolded"
            >
              {{ showFolded ? t('admin.topology.key.collapse') : t('admin.topology.key.foldedMore', { n: folded.folded.length }) }}
            </button>
            <div v-if="snapshot.keys.length === 0" class="text-xs text-gray-400">{{ t('admin.topology.key.none') }}</div>
          </div>

          <!-- Middle: platform -->
          <div class="relative z-10 min-w-0 self-start lg:pt-8">
            <TopologyPlatformCard
              :totals="totals"
              :gate-blocks="snapshot.gate_blocks_5m ?? {}"
              :strategy="strategy"
              :gates="gates"
              :blocked="blocked"
            />
            <span
              v-if="desktop && exitTag && exitPoint"
              class="pointer-events-none absolute z-20 whitespace-nowrap rounded-full px-2.5 py-0.5 text-[11px] font-semibold"
              :class="exitTag.bad ? 'bg-red-500 text-white' : 'border border-amber-300 bg-amber-100 text-amber-800 dark:border-amber-700 dark:bg-amber-900/60 dark:text-amber-200'"
              :style="{ right: '-8px', top: `calc(50% + 28px)`, transform: 'translateX(100%)' }"
              data-testid="topology-exit-tag"
            >{{ exitTag.text }}</span>
          </div>

          <!-- Right: accounts -->
          <div class="relative z-10 flex min-w-0 flex-col gap-3">
            <div class="text-xs text-gray-400 dark:text-dark-400">{{ t('admin.topology.columns.accounts') }}</div>
            <TopologyAccountCard
              v-for="a in accounts"
              :key="a.id"
              :account="a"
              :clock="clock"
              :fx="accountFx(a.id)"
              :cooldown-left="cooldownLeft(a.id)"
            />
            <div v-if="accounts.length === 0" class="text-xs text-gray-400">{{ t('admin.topology.account.none') }}</div>
          </div>

          <!-- moving request dots (above the cards) -->
          <svg
            v-if="desktop && geom.width > 0"
            class="pointer-events-none absolute left-0 top-0 z-20 overflow-visible"
            :width="geom.width"
            :height="geom.height"
            aria-hidden="true"
          >
            <circle v-for="d in dots" :key="d.id" :cx="d.x" :cy="d.y" r="5" :class="d.err ? 'topo-dot-err' : 'topo-dot'" />
          </svg>
        </div>

        <!-- Legend -->
        <div class="mt-6 flex flex-wrap items-center gap-x-7 gap-y-2 text-xs text-gray-500 dark:text-dark-400">
          <span class="flex items-center gap-2">
            <svg width="28" height="8" viewBox="0 0 28 8" aria-hidden="true"><path d="M0 4 L28 4" class="topo-ln-on" /></svg>
            {{ t('admin.topology.legend.active') }}
          </span>
          <span class="flex items-center gap-2">
            <svg width="28" height="8" viewBox="0 0 28 8" aria-hidden="true"><path d="M0 4 L28 4" class="topo-ln-cool" /></svg>
            {{ t('admin.topology.legend.cooldown') }}
          </span>
          <span class="flex items-center gap-2">
            <svg width="28" height="8" viewBox="0 0 28 8" aria-hidden="true"><path d="M0 4 L28 4" class="topo-ln-off" style="stroke-width: 2" /></svg>
            {{ t('admin.topology.legend.unavailable') }}
          </span>
          <span class="flex items-center gap-2">
            <span class="h-2.5 w-2.5 rounded-full bg-primary-600 shadow-[0_0_0_3px_#ccfbf1] dark:shadow-[0_0_0_3px_rgba(20,184,166,0.25)]" />
            {{ t('admin.topology.legend.request') }}
          </span>
          <span class="flex items-center gap-2">
            <span class="h-2.5 w-2.5 rounded-full bg-red-500 shadow-[0_0_0_3px_#fee2e2] dark:shadow-[0_0_0_3px_rgba(239,68,68,0.25)]" />
            {{ t('admin.topology.legend.error') }}
          </span>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<style scoped>
.topo-ln {
  fill: none;
  stroke: #e2e8ea;
  stroke-width: 1.5;
}
.topo-ln-on {
  fill: none;
  stroke: #14b8a6;
  stroke-width: 1.75;
  stroke-dasharray: 2 5;
  stroke-linecap: round;
  animation: topo-flow 0.9s linear infinite;
}
.topo-ln-off {
  fill: none;
  stroke: #e5e7eb;
  stroke-width: 1.5;
  stroke-dasharray: 4 5;
  opacity: 0.8;
}
.topo-ln-cool {
  fill: none;
  stroke: #f59e0b;
  stroke-width: 2;
  stroke-dasharray: 2 6;
  stroke-linecap: round;
}
.topo-anchor {
  fill: #ffffff;
  stroke: #d1d5db;
  stroke-width: 1.5;
}
.topo-anchor-on {
  stroke: #14b8a6;
}
.topo-exit {
  fill: #0d9488;
}
.topo-dot {
  fill: #0d9488;
  stroke: #ffffff;
  stroke-width: 3;
  filter: drop-shadow(0 2px 6px rgba(13, 148, 136, 0.45));
}
.topo-dot-err {
  fill: #ef4444;
  stroke: #ffffff;
  stroke-width: 3;
  filter: drop-shadow(0 2px 6px rgba(239, 68, 68, 0.5));
}
.topo-live-pulse {
  box-shadow: 0 0 0 3px #ccfbf1;
}
.dark .topo-ln {
  stroke: #334155;
}
.dark .topo-ln-off {
  stroke: #334155;
  opacity: 0.6;
}
.dark .topo-anchor {
  fill: #1e293b;
  stroke: #475569;
}
.dark .topo-anchor-on {
  stroke: #14b8a6;
}
.dark .topo-dot,
.dark .topo-dot-err {
  stroke: #1e293b;
}
.dark .topo-live-pulse {
  box-shadow: 0 0 0 3px rgba(20, 184, 166, 0.25);
}
@keyframes topo-flow {
  to {
    stroke-dashoffset: -14;
  }
}
@media (prefers-reduced-motion: reduce) {
  .topo-ln-on {
    animation: none;
  }
}
</style>

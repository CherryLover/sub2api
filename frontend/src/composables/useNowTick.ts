/**
 * 共享的「当前时间」心跳
 *
 * 背景：像 429 限流徽标这类 UI 依赖 `now > reset_at` 的比较，而 `Date.now()` /
 * `new Date()` 本身不是响应式依赖 —— computed 只在其它依赖（例如 props 对象引用）
 * 变化时才重算，时间流逝本身不会触发重算。结果就是限流自然到期后徽标仍然挂在
 * 页面上，必须整页刷新才消失（后端不会主动清空 reset_at，列表 ETag 也不会变，
 * 所以自动刷新同样救不了它）。
 *
 * 本 composable 提供一个模块级共享的响应式时间戳：
 * - 全局只有一个 setInterval，无论多少组件订阅（账号列表可能有几百行，
 *   每行一个定时器是不可接受的）；
 * - 最后一个订阅者离开时定时器被清理，不泄漏；
 * - 页面不可见（document.visibilityState === 'hidden'）时暂停，避免标签页在
 *   后台空转唤醒 CPU；重新可见时立即补一次更新，避免回到前台看到过期状态。
 *
 * 用法：
 * ```ts
 * const { now } = useNowTick()
 * const isRateLimited = computed(() => {
 *   const ts = Date.parse(props.account.rate_limit_reset_at ?? '')
 *   return Number.isFinite(ts) && ts > now.value
 * })
 * ```
 * 组件卸载时会自动退订（依赖当前 effect scope）；在组件外调用时请自行调用
 * 返回的 `stop()`。
 */
import { getCurrentScope, onScopeDispose, readonly, ref, type Ref } from 'vue'

/**
 * 心跳间隔（毫秒）。
 *
 * 取值理由：消费方展示的倒计时（`formatCountdown` / `formatCountdownWithSuffix`）
 * 最细只到「分钟」，1 秒心跳带来的 59/60 次重算是纯粹的浪费；但徽标本身希望在
 * reset 时刻「及时」消失，分钟级心跳又太钝。5 秒是折中：最坏情况下徽标晚 5 秒
 * 消失（对运维判断账号是否可用完全无感），而唤醒次数只有 1 秒心跳的 1/12。
 *
 * 注意：需要秒级精度的地方（例如 AccountQuotaInfo 里 "12m 34s" 这种倒计时）
 * 不适合复用这个心跳。
 */
export const NOW_TICK_INTERVAL_MS = 5000

const now = ref(Date.now())
const nowReadonly = readonly(now) as Readonly<Ref<number>>

let subscribers = 0
let timerId: ReturnType<typeof setInterval> | null = null
let visibilityBound = false

const isHidden = (): boolean =>
  typeof document !== 'undefined' && document.visibilityState === 'hidden'

const syncNow = (): void => {
  now.value = Date.now()
}

const startTimer = (): void => {
  if (timerId !== null || isHidden()) return
  timerId = setInterval(syncNow, NOW_TICK_INTERVAL_MS)
}

const stopTimer = (): void => {
  if (timerId === null) return
  clearInterval(timerId)
  timerId = null
}

const handleVisibilityChange = (): void => {
  if (subscribers === 0) return
  if (isHidden()) {
    // 后台标签页：停表，不再空转
    stopTimer()
    return
  }
  // 回到前台：先补一次，再恢复心跳，避免看到隐藏期间冻结的旧状态
  syncNow()
  startTimer()
}

const bindVisibility = (): void => {
  if (visibilityBound || typeof document === 'undefined') return
  document.addEventListener('visibilitychange', handleVisibilityChange)
  visibilityBound = true
}

const unbindVisibility = (): void => {
  if (!visibilityBound || typeof document === 'undefined') return
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  visibilityBound = false
}

const retain = (): void => {
  subscribers += 1
  if (subscribers > 1) return
  // 第一个订阅者：立刻取一次时间（可能距离上次心跳已经很久了），再起表
  syncNow()
  bindVisibility()
  startTimer()
}

const release = (): void => {
  if (subscribers === 0) return
  subscribers -= 1
  if (subscribers > 0) return
  stopTimer()
  unbindVisibility()
}

/**
 * 订阅共享心跳。
 * @returns now 当前时间戳（毫秒，响应式只读）；stop 手动退订（幂等）
 */
export function useNowTick(): { now: Readonly<Ref<number>>; stop: () => void } {
  let stopped = false
  const stop = (): void => {
    if (stopped) return
    stopped = true
    release()
  }

  retain()
  if (getCurrentScope()) {
    onScopeDispose(stop)
  }

  return { now: nowReadonly, stop }
}

/** 仅供测试：观察内部状态（订阅数 / 定时器是否在跑） */
export function _getNowTickState(): { subscribers: number; running: boolean; visibilityBound: boolean } {
  return { subscribers, running: timerId !== null, visibilityBound }
}

/** 仅供测试：强制重置为初始状态 */
export function _resetNowTick(): void {
  subscribers = 0
  stopTimer()
  unbindVisibility()
  now.value = Date.now()
}

/**
 * useNowTick 共享心跳单元测试
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { NOW_TICK_INTERVAL_MS, useNowTick, _getNowTickState, _resetNowTick } from '../useNowTick'

const T0 = Date.UTC(2026, 7, 23, 10, 0, 0)

// jsdom 的 document.visibilityState 是只读 getter，这里替换成可控的
let visibility: DocumentVisibilityState = 'visible'
Object.defineProperty(document, 'visibilityState', {
  configurable: true,
  get: () => visibility
})

function setVisibility(next: DocumentVisibilityState): void {
  visibility = next
  document.dispatchEvent(new Event('visibilitychange'))
}

describe('useNowTick', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(T0)
    visibility = 'visible'
    _resetNowTick()
  })

  afterEach(() => {
    _resetNowTick()
    visibility = 'visible'
    vi.useRealTimers()
  })

  it('心跳到点后 now 自动前进', () => {
    const { now, stop } = useNowTick()

    expect(now.value).toBe(T0)

    vi.advanceTimersByTime(NOW_TICK_INTERVAL_MS)
    expect(now.value).toBe(T0 + NOW_TICK_INTERVAL_MS)

    vi.advanceTimersByTime(NOW_TICK_INTERVAL_MS)
    expect(now.value).toBe(T0 + NOW_TICK_INTERVAL_MS * 2)

    stop()
  })

  it('多个使用者共享同一个定时器和同一个 ref', () => {
    const a = useNowTick()
    const b = useNowTick()
    const c = useNowTick()

    expect(_getNowTickState().subscribers).toBe(3)
    // 只有一个定时器在跑，而不是每个使用者一个
    expect(vi.getTimerCount()).toBe(1)
    expect(a.now).toBe(b.now)
    expect(b.now).toBe(c.now)

    vi.advanceTimersByTime(NOW_TICK_INTERVAL_MS)
    expect(a.now.value).toBe(T0 + NOW_TICK_INTERVAL_MS)
    expect(c.now.value).toBe(T0 + NOW_TICK_INTERVAL_MS)

    a.stop()
    b.stop()
    c.stop()
  })

  it('最后一个使用者离开后定时器被清理，不泄漏', () => {
    const a = useNowTick()
    const b = useNowTick()

    a.stop()
    // 还有人在用 → 定时器继续
    expect(_getNowTickState().running).toBe(true)
    expect(vi.getTimerCount()).toBe(1)

    b.stop()
    expect(_getNowTickState().subscribers).toBe(0)
    expect(_getNowTickState().running).toBe(false)
    expect(_getNowTickState().visibilityBound).toBe(false)
    expect(vi.getTimerCount()).toBe(0)

    // 退订之后时间推进不再产生任何更新
    const frozen = a.now.value
    vi.advanceTimersByTime(NOW_TICK_INTERVAL_MS * 20)
    expect(a.now.value).toBe(frozen)
  })

  it('stop() 幂等，重复调用不会把订阅数扣成负数', () => {
    const a = useNowTick()
    const b = useNowTick()

    a.stop()
    a.stop()
    a.stop()

    expect(_getNowTickState().subscribers).toBe(1)
    expect(_getNowTickState().running).toBe(true)

    b.stop()
    expect(_getNowTickState().subscribers).toBe(0)
  })

  it('组件卸载时自动退订', () => {
    const Consumer = defineComponent({
      setup() {
        const { now } = useNowTick()
        return () => h('div', String(now.value))
      }
    })

    const first = mount(Consumer)
    const second = mount(Consumer)
    expect(_getNowTickState().subscribers).toBe(2)
    expect(vi.getTimerCount()).toBe(1)

    first.unmount()
    expect(_getNowTickState().subscribers).toBe(1)
    expect(_getNowTickState().running).toBe(true)

    second.unmount()
    expect(_getNowTickState().subscribers).toBe(0)
    expect(_getNowTickState().running).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('页面不可见时暂停心跳', () => {
    const { now, stop } = useNowTick()

    setVisibility('hidden')
    expect(_getNowTickState().running).toBe(false)
    expect(vi.getTimerCount()).toBe(0)

    vi.advanceTimersByTime(NOW_TICK_INTERVAL_MS * 10)
    expect(now.value).toBe(T0)

    stop()
  })

  it('重新可见时立即补一次更新并恢复心跳', () => {
    const { now, stop } = useNowTick()

    setVisibility('hidden')
    vi.advanceTimersByTime(NOW_TICK_INTERVAL_MS * 10)
    expect(now.value).toBe(T0)

    // 回到前台：不等下一次心跳，立刻补齐
    setVisibility('visible')
    expect(now.value).toBe(T0 + NOW_TICK_INTERVAL_MS * 10)
    expect(_getNowTickState().running).toBe(true)

    vi.advanceTimersByTime(NOW_TICK_INTERVAL_MS)
    expect(now.value).toBe(T0 + NOW_TICK_INTERVAL_MS * 11)

    stop()
  })

  it('页面处于隐藏状态时订阅不会起表，可见后恢复', () => {
    visibility = 'hidden'

    const { now, stop } = useNowTick()
    expect(_getNowTickState().running).toBe(false)
    expect(vi.getTimerCount()).toBe(0)

    vi.advanceTimersByTime(NOW_TICK_INTERVAL_MS * 3)
    expect(now.value).toBe(T0)

    setVisibility('visible')
    expect(_getNowTickState().running).toBe(true)
    expect(now.value).toBe(T0 + NOW_TICK_INTERVAL_MS * 3)

    stop()
  })
})

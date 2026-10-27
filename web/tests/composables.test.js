import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { defineComponent, ref, nextTick } from 'vue'
import { useDebouncedCallback } from '@/composables/useDebouncedCallback'
import { useDialogModel } from '@/composables/useDialogModel'
import { useResultReport } from '@/composables/useResultReport'

enableAutoUnmount(afterEach)

describe('useDebouncedCallback', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  function mountWith(composable) {
    let result
    const Comp = defineComponent({
      setup() {
        result = composable()
        return () => null
      }
    })
    mount(Comp)
    return result
  }

  it('delay 内多次调用只触发最后一次', () => {
    const fn = vi.fn()
    const wrapped = mountWith(() => useDebouncedCallback(fn, 100))
    wrapped('a')
    wrapped('b')
    wrapped('c')
    expect(fn).not.toHaveBeenCalled()
    vi.advanceTimersByTime(100)
    expect(fn).toHaveBeenCalledTimes(1)
    expect(fn).toHaveBeenCalledWith('c')
  })

  it('cancel 后不再触发', () => {
    const fn = vi.fn()
    const wrapped = mountWith(() => useDebouncedCallback(fn, 100))
    wrapped()
    wrapped.cancel()
    vi.advanceTimersByTime(200)
    expect(fn).not.toHaveBeenCalled()
  })

  it('cancel 后可再次使用', () => {
    const fn = vi.fn()
    const wrapped = mountWith(() => useDebouncedCallback(fn, 100))
    wrapped()
    wrapped.cancel()
    wrapped('again')
    vi.advanceTimersByTime(100)
    expect(fn).toHaveBeenCalledWith('again')
  })

  it('组件卸载自动取消', () => {
    const fn = vi.fn()
    const Comp = defineComponent({
      setup() {
        const wrapped = useDebouncedCallback(fn, 100)
        wrapped()
        return () => null
      }
    })
    const w = mount(Comp)
    w.unmount()
    vi.advanceTimersByTime(200)
    expect(fn).not.toHaveBeenCalled()
  })
})

describe('useDialogModel', () => {
  function mountDialog(onOpen) {
    let visible, emit
    const Comp = defineComponent({
      props: ['modelValue'],
      emits: ['update:modelValue'],
      setup(props, ctx) {
        emit = ctx.emit
        visible = useDialogModel(props, ctx.emit, onOpen)
        return () => null
      }
    })
    const wrapper = mount(Comp, { props: { modelValue: false } })
    return { wrapper, get visible() { return visible }, get emit() { return emit } }
  }

  it('初始值取自 props', async () => {
    const { wrapper, visible } = mountDialog()
    expect(visible.value).toBe(false)
    await wrapper.setProps({ modelValue: true })
    expect(visible.value).toBe(true)
  })

  it('打开时触发 onOpen', async () => {
    const onOpen = vi.fn()
    const { wrapper } = mountDialog(onOpen)
    await wrapper.setProps({ modelValue: true })
    expect(onOpen).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ modelValue: false })
    await wrapper.setProps({ modelValue: true })
    expect(onOpen).toHaveBeenCalledTimes(2)
  })

  it('本地变化 emit 给父级', async () => {
    const { wrapper, visible } = mountDialog()
    visible.value = true
    await nextTick()
    expect(wrapper.emitted('update:modelValue')).toEqual([[true]])
  })
})

describe('useResultReport', () => {
  it('openReport 设置响应式状态并可见', () => {
    const { report, openReport } = useResultReport()
    openReport({ title: 'T', summary: 'S' })
    expect(report.visible).toBe(true)
    expect(report.title).toBe('T')
    expect(report.summary).toBe('S')
    expect(report.confirmText).toBe('知道了') // BASE 默认
  })

  it('defaults 叠加且每次打开重置残留', () => {
    const { report, openReport } = useResultReport({ confirmText: '确定', tone: 'success' })
    openReport({ cancelText: '取消', confirmType: 'danger' })
    expect(report.cancelText).toBe('取消')
    expect(report.tone).toBe('success')
    openReport({ title: '再次' })
    expect(report.cancelText).toBe('') // 上一次的确认模式按钮不残留
    expect(report.title).toBe('再次')
  })

  it('confirm/cancel 关闭弹窗', () => {
    const { report, openReport, confirmReport, cancelReport } = useResultReport()
    openReport({})
    confirmReport()
    expect(report.visible).toBe(false)
    openReport({})
    cancelReport()
    expect(report.visible).toBe(false)
  })

  it('配置 cancelText 时返回 Promise 并正确 resolve', async () => {
    const { openReport, confirmReport, cancelReport } = useResultReport()
    const p1 = openReport({ cancelText: '取消' })
    confirmReport()
    await expect(p1).resolves.toBe(true)

    const p2 = openReport({ cancelText: '取消' })
    cancelReport()
    await expect(p2).resolves.toBe(false)
  })

  it('未配置 cancelText 时 openReport 返回 undefined', () => {
    const { openReport } = useResultReport()
    expect(openReport({ title: 'x' })).toBeUndefined()
  })
})

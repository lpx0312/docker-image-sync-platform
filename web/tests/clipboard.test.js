import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    error: vi.fn()
  }
}))

import { ElMessage } from 'element-plus'
import copyToClipboard from '@/utils/clipboard'

describe('copyToClipboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    delete navigator.clipboard
    // 还原 execCommand 桩
    document.execCommand = undefined
  })

  function stubClipboard(impl = async () => {}) {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn(impl) },
      configurable: true
    })
  }

  it('Clipboard API 成功路径', async () => {
    const writeText = vi.fn(async () => {})
    stubClipboard(async (t) => writeText(t))
    const ok = await copyToClipboard('hello')
    expect(ok).toBe(true)
    expect(writeText).toHaveBeenCalledWith('hello')
    expect(ElMessage.success).toHaveBeenCalledWith('已复制到剪贴板')
  })

  it('Clipboard API 失败降级到 execCommand 成功', async () => {
    stubClipboard(async () => { throw new Error('denied') })
    const exec = vi.fn(() => true)
    document.execCommand = exec
    const ok = await copyToClipboard('fallback')
    expect(ok).toBe(true)
    expect(exec).toHaveBeenCalledWith('copy')
    expect(ElMessage.success).toHaveBeenCalled()
  })

  it('execCommand 返回 false -> 复制失败', async () => {
    stubClipboard(async () => { throw new Error('denied') })
    document.execCommand = vi.fn(() => false)
    const ok = await copyToClipboard('nope')
    expect(ok).toBe(false)
    expect(ElMessage.error).toHaveBeenCalledWith('复制失败')
  })

  it('无 Clipboard API 且 execCommand 抛错 -> 失败', async () => {
    // 不挂 clipboard
    document.execCommand = vi.fn(() => { throw new Error('unsupported') })
    const ok = await copyToClipboard('boom')
    expect(ok).toBe(false)
    expect(ElMessage.error).toHaveBeenCalledWith('复制失败')
  })
})

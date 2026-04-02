/**
 * 剪贴板工具函数
 *
 * 提供复制文本到剪贴板的功能，支持降级处理兼容 HTTP 内网环境
 */
import { ElMessage } from 'element-plus'

/**
 * 复制文本到剪贴板
 *
 * @param {string} text - 要复制的文本
 * @returns {Promise<boolean>} 是否复制成功
 *
 * @example
 * await copyToClipboard('hello world')
 */
export async function copyToClipboard(text) {
  // 优先使用现代 Clipboard API
  if (navigator.clipboard && navigator.clipboard.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      ElMessage.success('已复制到剪贴板')
      return true
    } catch (error) {
      console.warn('navigator.clipboard 调用失败，尝试降级复制方案:', error)
    }
  }

  // 降级方案：使用隐藏的 textarea + document.execCommand('copy')
  try {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.style.position = 'fixed'
    textarea.style.top = '-9999px'
    textarea.style.left = '-9999px'
    textarea.setAttribute('readonly', 'readonly')
    document.body.appendChild(textarea)

    textarea.select()
    textarea.setSelectionRange(0, textarea.value.length)

    const successful = document.execCommand('copy')
    document.body.removeChild(textarea)

    if (successful) {
      ElMessage.success('已复制到剪贴板')
      return true
    } else {
      ElMessage.error('复制失败')
      return false
    }
  } catch (error) {
    console.error('降级复制方案失败:', error)
    ElMessage.error('复制失败')
    return false
  }
}

export default copyToClipboard

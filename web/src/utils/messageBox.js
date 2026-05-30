import { ElMessageBox } from 'element-plus'

const MULTILINE_STYLE = [
  'max-height: 400px',
  'overflow-y: auto',
  'white-space: pre-wrap',
  'word-break: break-all',
  'line-height: 1.6',
  'text-align: left',
  'margin: 0',
].join(';')

/** 将纯文本转为可在 MessageBox 中正确换行展示的 HTML */
export const formatMultilineHtml = (text) => {
  if (!text) return ''
  const escaped = String(text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
  return `<div style="${MULTILINE_STYLE}">${escaped}</div>`
}

/** 多行文本 alert 弹窗 */
export const showMultilineAlert = (message, title, options = {}) => {
  return ElMessageBox.alert(formatMultilineHtml(message), title, {
    dangerouslyUseHTMLString: true,
    customClass: 'multiline-message-box',
    ...options,
  })
}

/** 多行文本 confirm 弹窗 */
export const showMultilineConfirm = (message, title, options = {}) => {
  return ElMessageBox.confirm(formatMultilineHtml(message), title, {
    dangerouslyUseHTMLString: true,
    customClass: 'multiline-message-box',
    ...options,
  })
}

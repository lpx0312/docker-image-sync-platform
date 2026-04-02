/**
 * 格式化工具函数
 *
 * 提供时间、数据等格式化功能
 */
import dayjs from 'dayjs'

/**
 * 格式化时间为标准日期时间格式
 *
 * @param {string|Date} time - 时间值
 * @param {string} format - 格式化模式，默认 'YYYY-MM-DD HH:mm:ss'
 * @returns {string} 格式化后的时间字符串
 *
 * @example
 * formatTime('2024-01-01T12:00:00Z') // '2024-01-01 12:00:00'
 * formatTime(new Date(), 'YYYY/MM/DD') // '2024/01/01'
 */
export function formatTime(time, format = 'YYYY-MM-DD HH:mm:ss') {
  if (!time) return '-'
  return dayjs(time).format(format)
}

/**
 * 格式化持续时间为可读字符串
 *
 * @param {number} seconds - 持续时间（秒）
 * @returns {string} 可读的持续时间字符串
 *
 * @example
 * formatDuration(90) // '1分30秒'
 * formatDuration(3661) // '1小时1分钟'
 */
export function formatDuration(seconds) {
  if (!seconds || seconds <= 0) return '-'

  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = Math.floor(seconds % 60)

  if (hours > 0) {
    return `${hours}小时${minutes > 0 ? minutes + '分钟' : ''}`
  }
  if (minutes > 0) {
    return `${minutes}分${secs > 0 ? secs + '秒' : ''}`
  }
  return `${secs}秒`
}

/**
 * 格式化文件大小
 *
 * @param {number} bytes - 字节数
 * @returns {string} 可读的文件大小字符串
 */
export function formatSize(bytes) {
  if (!bytes || bytes <= 0) return '0 B'

  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let unitIndex = 0
  let size = bytes

  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex++
  }

  return `${size.toFixed(2)} ${units[unitIndex]}`
}

export default {
  formatTime,
  formatDuration,
  formatSize
}

/**
 * 状态映射工具函数
 *
 * 提供镜像同步状态、GitHub Actions 状态的映射和格式化功能
 */

/**
 * 镜像同步状态类型映射
 */
const syncStatusTypeMap = {
  pending: 'info',
  syncing: 'warning',
  success: 'success',
  failed: 'danger'
}

/**
 * 镜像同步状态文本映射
 */
const syncStatusTextMap = {
  pending: '等待中',
  syncing: '同步中',
  success: '成功',
  failed: '失败'
}

/**
 * GitHub Actions 状态类型映射
 */
const githubStatusTypeMap = {
  pending: 'info',
  queued: 'info',
  in_progress: 'warning',
  completed: 'success'
}

/**
 * GitHub Actions 状态文本映射
 */
const githubStatusTextMap = {
  pending: '等待中',
  queued: '排队中',
  in_progress: '执行中',
  completed: '已完成'
}

/**
 * GitHub Actions 结论类型映射
 */
const githubConclusionTypeMap = {
  success: 'success',
  failure: 'danger',
  cancelled: 'warning',
  skipped: 'info',
  timed_out: 'danger'
}

/**
 * GitHub Actions 结论文本映射
 */
const githubConclusionTextMap = {
  success: '成功',
  failure: '失败',
  cancelled: '已取消',
  skipped: '已跳过',
  timed_out: '超时'
}

/**
 * 获取镜像同步状态的 Element Plus Tag 类型
 *
 * @param {string} status - 同步状态
 * @returns {string} Tag 类型
 */
export function getSyncStatusType(status) {
  return syncStatusTypeMap[status] || 'info'
}

/**
 * 获取镜像同步状态的中文文本
 *
 * @param {string} status - 同步状态
 * @returns {string} 状态文本
 */
export function getSyncStatusText(status) {
  return syncStatusTextMap[status] || '未知'
}

/**
 * 获取 GitHub Actions 状态的 Element Plus Tag 类型
 *
 * @param {string} status - GitHub Actions 状态
 * @returns {string} Tag 类型
 */
export function getGitHubStatusType(status) {
  return githubStatusTypeMap[status] || 'info'
}

/**
 * 获取 GitHub Actions 状态的中文文本
 *
 * @param {string} status - GitHub Actions 状态
 * @returns {string} 状态文本
 */
export function getGitHubStatusText(status) {
  return githubStatusTextMap[status] || '未知'
}

/**
 * 获取 GitHub Actions 结论的 Element Plus Tag 类型
 *
 * @param {string} conclusion - GitHub Actions 结论
 * @returns {string} Tag 类型
 */
export function getGitHubConclusionType(conclusion) {
  return githubConclusionTypeMap[conclusion] || 'info'
}

/**
 * 获取 GitHub Actions 结论的中文文本
 *
 * @param {string} conclusion - GitHub Actions 结论
 * @returns {string} 结论文本
 */
export function getGitHubConclusionText(conclusion) {
  return githubConclusionTextMap[conclusion] || '未知'
}

/**
 * 统一的状态类型获取函数（自动判断状态类型）
 *
 * @param {string} status - 状态值
 * @param {string} type - 状态类型 'sync' | 'github-status' | 'github-conclusion'
 * @returns {string} Tag 类型
 */
export function getStatusType(status, type = 'sync') {
  switch (type) {
    case 'sync':
      return getSyncStatusType(status)
    case 'github-status':
      return getGitHubStatusType(status)
    case 'github-conclusion':
      return getGitHubConclusionType(status)
    default:
      return 'info'
  }
}

/**
 * 统一的状态文本获取函数（自动判断状态类型）
 *
 * @param {string} status - 状态值
 * @param {string} type - 状态类型 'sync' | 'github-status' | 'github-conclusion'
 * @returns {string} 状态文本
 */
export function getStatusText(status, type = 'sync') {
  switch (type) {
    case 'sync':
      return getSyncStatusText(status)
    case 'github-status':
      return getGitHubStatusText(status)
    case 'github-conclusion':
      return getGitHubConclusionText(status)
    default:
      return '未知'
  }
}

export default {
  getSyncStatusType,
  getSyncStatusText,
  getGitHubStatusType,
  getGitHubStatusText,
  getGitHubConclusionType,
  getGitHubConclusionText,
  getStatusType,
  getStatusText
}

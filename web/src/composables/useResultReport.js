import { reactive } from 'vue'

/**
 * 统一结果报告弹窗的状态管理。
 *
 * 返回 report（绑定给 ResultReportDialog 的响应式对象）、
 * openReport(opts)（打开弹窗；配置了 cancelText 时返回 Promise<boolean>，
 * 点确认 resolve(true)，点取消/关闭 resolve(false)）、
 * confirmReport / cancelReport（绑定给组件的 confirm/cancel 事件）。
 */
export function useResultReport(defaults = {}) {
  const report = reactive({
    visible: false,
    title: '',
    tone: 'info',
    summary: '',
    sections: [],
    confirmText: '知道了',
    cancelText: '',
    confirmType: 'primary',
    emptyText: '',
    width: '520px',
  })

  let resolver = null

  const openReport = (opts = {}) => {
    Object.assign(report, defaults, opts, { visible: true })
    if (opts.cancelText) {
      return new Promise((resolve) => { resolver = resolve })
    }
  }

  const settle = (result) => {
    report.visible = false
    if (resolver) {
      resolver(result)
      resolver = null
    }
  }

  return {
    report,
    openReport,
    confirmReport: () => settle(true),
    cancelReport: () => settle(false),
  }
}

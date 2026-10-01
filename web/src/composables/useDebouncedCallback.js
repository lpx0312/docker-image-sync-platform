import { onUnmounted } from 'vue'

// 防抖回调：delay 内重复调用只保留最后一次，替代各组件手写的
// 「timer 变量 + clearTimeout + setTimeout」三件套。
// 组件卸载时自动取消未触发的定时器。
export function useDebouncedCallback(fn, delay = 300) {
  let timer = null

  const wrapped = (...args) => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      timer = null
      fn(...args)
    }, delay)
  }

  wrapped.cancel = () => {
    if (timer) clearTimeout(timer)
    timer = null
  }

  onUnmounted(() => wrapped.cancel())

  return wrapped
}

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { syncAPI } from '@/api'

export const useSyncStore = defineStore('sync', () => {
  // 状态
  const syncHistory = ref([])
  const currentTask = ref(null)
  const loading = ref(false)

  // 计算属性
  const hasCurrentTask = computed(() => !!currentTask.value)
  
  const taskStatus = computed(() => {
    if (!currentTask.value) return null
    return currentTask.value.status
  })

  // 动作
  const submitSync = async (imageData) => {
    loading.value = true
    try {
      const response = await syncAPI.submitSync(imageData)
      currentTask.value = response
      return response
    } finally {
      loading.value = false
    }
  }

  const getSyncStatus = async (taskId) => {
    try {
      const response = await syncAPI.getSyncStatus(taskId)
      if (currentTask.value && currentTask.value.task_id === taskId) {
        currentTask.value = { ...currentTask.value, ...response }
      }
      return response
    } catch (error) {
      console.error('获取同步状态失败:', error)
      throw error
    }
  }

  const loadSyncHistory = async (params = {}) => {
    loading.value = true
    try {
      const response = await syncAPI.getSyncHistory(params)
      syncHistory.value = response.data || []
      return response
    } finally {
      loading.value = false
    }
  }

  const clearCurrentTask = () => {
    currentTask.value = null
  }

  const updateTaskStatus = (taskId, status, message = '') => {
    if (currentTask.value && currentTask.value.task_id === taskId) {
      currentTask.value.status = status
      if (message) {
        currentTask.value.message = message
      }
    }
    
    // 更新历史记录中的状态
    const historyItem = syncHistory.value.find(item => item.task_id === taskId)
    if (historyItem) {
      historyItem.status = status
      if (message) {
        historyItem.message = message
      }
    }
  }

  return {
    // 状态
    syncHistory,
    currentTask,
    loading,
    
    // 计算属性
    hasCurrentTask,
    taskStatus,
    
    // 动作
    submitSync,
    getSyncStatus,
    loadSyncHistory,
    clearCurrentTask,
    updateTaskStatus
  }
})
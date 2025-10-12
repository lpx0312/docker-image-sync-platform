/**
 * 同步任务状态管理Store
 * 
 * 功能说明：
 * - 管理镜像同步任务的状态和数据
 * - 提供同步任务的提交、查询、更新功能
 * - 维护同步历史记录和当前任务状态
 * - 统一处理加载状态和错误处理
 * 
 * 状态管理：
 * - syncHistory: 同步历史记录列表
 * - currentTask: 当前正在执行的任务
 * - loading: 全局加载状态
 * 
 * 主要功能：
 * - 单个/批量同步任务提交
 * - 任务状态实时查询和更新
 * - 同步历史记录管理
 * - 任务状态计算和判断
 * 
 * @author Docker Image Sync Platform
 * @version 1.0.0
 */

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { syncAPI } from '@/api'

export const useSyncStore = defineStore('sync', () => {
  /**
   * 响应式状态定义
   */
  
  /** @type {Ref<Array>} 同步历史记录列表 */
  const syncHistory = ref([])
  
  /** @type {Ref<Object|null>} 当前正在执行的同步任务 */
  const currentTask = ref(null)
  
  /** @type {Ref<boolean>} 全局加载状态 */
  const loading = ref(false)

  /**
   * 计算属性定义
   */
  
  /** 
   * 是否有当前正在执行的任务
   * @returns {boolean} 
   */
  const hasCurrentTask = computed(() => !!currentTask.value)
  
  /** 
   * 当前任务的状态
   * @returns {string|null} 任务状态或null
   */
  const taskStatus = computed(() => {
    if (!currentTask.value) return null
    return currentTask.value.status
  })

  /**
   * 动作方法定义
   */
  
  /**
   * 提交同步任务
   * 
   * @param {Object} imageData - 镜像同步数据
   * @param {Array} imageData.images - 镜像列表
   * @param {string} [imageData.architecture] - 目标架构
   * @returns {Promise<Object>} 同步任务响应数据
   * @throws {Error} 网络请求失败或服务器错误
   */
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

  /**
   * 获取单个同步任务状态
   * 
   * @param {string} taskId - 任务ID
   * @returns {Promise<Object>} 任务状态数据
   * @throws {Error} 网络请求失败或任务不存在
   */
  const getSyncStatus = async (taskId) => {
    try {
      const response = await syncAPI.getSyncStatus(taskId)
      // 如果是当前任务，更新本地状态
      if (currentTask.value && currentTask.value.task_id === taskId) {
        currentTask.value = { ...currentTask.value, ...response }
      }
      return response
    } catch (error) {
      console.error('获取同步状态失败:', error)
      throw error
    }
  }

  /**
   * 获取批量同步任务状态
   * 
   * @param {string} taskId - 批量任务ID
   * @returns {Promise<Object>} 批量任务状态和子任务列表
   * @throws {Error} 网络请求失败或任务不存在
   */
  const getBatchSyncStatus = async (taskId) => {
    try {
      const response = await syncAPI.getBatchSyncStatus(taskId)
      // 如果是当前任务，更新本地状态
      if (currentTask.value && currentTask.value.task_id === taskId) {
        currentTask.value = { ...currentTask.value, ...response }
      }
      return response
    } catch (error) {
      console.error('获取批量同步状态失败:', error)
      throw error
    }
  }

  /**
   * 加载同步历史记录
   * 
   * @param {Object} params - 查询参数
   * @param {number} [params.page] - 页码
   * @param {number} [params.page_size] - 每页数量
   * @param {string} [params.status] - 状态筛选
   * @returns {Promise<Object>} 历史记录响应数据
   * @throws {Error} 网络请求失败
   */
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

  /**
   * 清除当前任务状态
   * 
   * 用于任务完成后清理状态，避免界面显示过期信息
   */
  const clearCurrentTask = () => {
    currentTask.value = null
  }

  /**
   * 更新任务状态
   * 
   * 同时更新当前任务和历史记录中的状态信息
   * 
   * @param {string} taskId - 任务ID
   * @param {string} status - 新的任务状态
   * @param {string} [message=''] - 状态消息
   */
  const updateTaskStatus = (taskId, status, message = '') => {
    // 更新当前任务状态
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
    getBatchSyncStatus,
    loadSyncHistory,
    clearCurrentTask,
    updateTaskStatus
  }
})
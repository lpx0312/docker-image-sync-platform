/**
 * 镜像管理状态管理Store
 * 
 * 功能说明：
 * - 管理镜像列表数据和状态
 * - 提供镜像的增删改查功能
 * - 支持分页、筛选、排序功能
 * - 维护镜像统计信息
 * - 处理镜像操作的加载状态
 * 
 * 状态管理：
 * - images: 镜像列表数据
 * - imageStats: 镜像统计信息
 * - pagination: 分页配置
 * - filters: 筛选条件
 * - sorting: 排序配置
 * 
 * 主要功能：
 * - 镜像列表加载和刷新
 * - 镜像详情查询
 * - 镜像删除和重试操作
 * - 镜像存在性检测
 * - 实时统计信息更新
 * 
 * @author Docker Image Sync Platform
 * @version 1.0.0
 */

import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { imageAPI } from '@/api'

export const useImageStore = defineStore('image', () => {
  /**
   * 响应式状态定义
   */
  
  /** @type {Ref<Array>} 镜像列表数据 */
  const images = ref([])
  
  /** 
   * @type {Ref<Object>} 镜像统计信息
   * @property {number} total - 总数量
   * @property {number} pending - 待同步数量
   * @property {number} syncing - 同步中数量
   * @property {number} success - 成功数量
   * @property {number} failed - 失败数量
   */
  const imageStats = ref({
    total: 0,
    pending: 0,
    syncing: 0,
    success: 0,
    failed: 0
  })
  
  /** @type {Ref<boolean>} 全局加载状态 */
  const loading = ref(false)
  
  /** 
   * @type {Ref<Object>} 分页配置
   * @property {number} page - 当前页码
   * @property {number} pageSize - 每页数量
   * @property {number} total - 总记录数
   */
  const pagination = ref({
    page: 1,
    pageSize: 20,
    total: 0
  })
  
  /** 
   * @type {Ref<Object>} 筛选条件配置
   * @property {string} status - 状态筛选
   * @property {string} search - 搜索关键词
   * @property {string} architecture - 架构筛选
   * @property {boolean} deduplicate - 去重开关
   */
  const filters = ref({
    status: '', // 默认不筛选状态
    search: '',
    architecture: '',
    deduplicate: true // 去重开关，默认开启
  })
  
  /** 
   * @type {Ref<Object>} 排序配置
   * @property {string} sortBy - 排序字段
   * @property {string} sortOrder - 排序方向 (asc/desc)
   */
  const sorting = ref({
    sortBy: 'created_at',
    sortOrder: 'desc'
  })

  // 计算属性
  const hasImages = computed(() => images.value.length > 0)

  // 动作
  const loadImages = async (params = {}) => {
    loading.value = true
    try {
      const queryParams = {
        page: pagination.value.page,
        page_size: pagination.value.pageSize,
        sort_by: sorting.value.sortBy,
        sort_order: sorting.value.sortOrder,
        ...filters.value,
        ...params
      }
      
      console.log('loadImages queryParams:', queryParams)
      const response = await imageAPI.getImageList(queryParams)
      images.value = response.data || []
      pagination.value.total = response.total || 0
      
      return response
    } finally {
      loading.value = false
    }
  }

  const loadImageStats = async () => {
    try {
      const response = await imageAPI.getImageStats()
      imageStats.value = response
      return response
    } catch (error) {
      console.error('获取镜像统计失败:', error)
      throw error
    }
  }

  const deleteImage = async (id) => {
    try {
      await imageAPI.deleteImage(id)
      // 从列表中移除
      const index = images.value.findIndex(image => image.id === id)
      if (index > -1) {
        images.value.splice(index, 1)
        pagination.value.total--
      }
      // 重新加载统计
      await loadImageStats()
    } catch (error) {
      console.error('删除镜像失败:', error)
      throw error
    }
  }

  const retrySync = async (id) => {
    try {
      await imageAPI.retrySync(id)
      // 更新镜像状态
      const image = images.value.find(img => img.id === id)
      if (image) {
        image.status = 'pending'
        image.updated_at = new Date().toISOString()
      }
      // 重新加载统计
      await loadImageStats()
    } catch (error) {
      console.error('重试同步失败:', error)
      throw error
    }
  }

  const updatePagination = (page, pageSize) => {
    pagination.value.page = page
    pagination.value.pageSize = pageSize
  }

  const updateFilters = (newFilters) => {
    console.log('updateFilters called with:', newFilters)
    console.log('Current filters before update:', filters.value)
    filters.value = { ...filters.value, ...newFilters }
    console.log('Updated filters:', filters.value)
    pagination.value.page = 1 // 重置到第一页
  }

  const clearFilters = () => {
    filters.value = {
      status: '', // 默认不筛选状态
      search: '',
      architecture: '',
      deduplicate: true // 去重开关，默认开启
    }
    pagination.value.page = 1
  }

  const updateSorting = (sortBy, sortOrder) => {
    sorting.value.sortBy = sortBy
    sorting.value.sortOrder = sortOrder
    pagination.value.page = 1 // 重置到第一页
  }

  const getImageById = async (id) => {
    try {
      const response = await imageAPI.getImageDetail(id)
      return response
    } catch (error) {
      console.error('获取镜像详情失败:', error)
      throw error
    }
  }

  const updateImageStatus = (id, status, message = '') => {
    const image = images.value.find(img => img.id === id)
    if (image) {
      image.status = status
      image.updated_at = new Date().toISOString()
      if (message) {
        image.message = message
      }
    }
  }

  const checkImageExists = async (id) => {
    try {
      const response = await imageAPI.checkImageExists(id)
      // 如果镜像存在，更新本地状态
      if (response.exists) {
        updateImageStatus(id, 'success')
        await loadImageStats()
      }
      return response
    } catch (error) {
      console.error('检测镜像失败:', error)
      throw error
    }
  }

  const batchCheckImages = async (ids) => {
    try {
      const response = await imageAPI.batchCheckImages(ids)
      // 重新加载数据以获取最新状态
      await loadImages()
      await loadImageStats()
      return response
    } catch (error) {
      console.error('批量检测镜像失败:', error)
      throw error
    }
  }

  // 监控filters变化
  watch(filters, (newFilters, oldFilters) => {
    console.log('Filters changed from:', oldFilters, 'to:', newFilters)
    console.trace('Filters change stack trace')
  }, { deep: true })

  return {
    // 状态
    images,
    imageStats,
    loading,
    pagination,
    filters,
    sorting,
    
    // 计算属性
    hasImages,
    
    // 动作
    loadImages,
    loadImageStats,
    deleteImage,
    retrySync,
    updatePagination,
    updateFilters,
    clearFilters,
    updateSorting,
    getImageById,
    updateImageStatus,
    checkImageExists,
    batchCheckImages
  }
})
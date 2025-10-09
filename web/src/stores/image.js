import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { imageAPI } from '@/api'

export const useImageStore = defineStore('image', () => {
  // 状态
  const images = ref([])
  const imageStats = ref({
    total: 0,
    pending: 0,
    syncing: 0,
    success: 0,
    failed: 0
  })
  const loading = ref(false)
  const pagination = ref({
    page: 1,
    pageSize: 20,
    total: 0
  })
  const filters = ref({
    status: 'success', // 默认筛选成功状态
    search: '',
    architecture: ''
  })
  const sorting = ref({
    sortBy: 'updated_at',
    sortOrder: 'desc'
  })

  // 计算属性
  const hasImages = computed(() => images.value.length > 0)
  
  const filteredImages = computed(() => {
    let result = images.value
    
    if (filters.value.status) {
      result = result.filter(image => image.status === filters.value.status)
    }
    
    if (filters.value.search) {
      const search = filters.value.search.toLowerCase()
      result = result.filter(image => 
        image.source_image.toLowerCase().includes(search) ||
        image.target_image.toLowerCase().includes(search)
      )
    }
    
    return result
  })

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
    filters.value = { ...filters.value, ...newFilters }
    pagination.value.page = 1 // 重置到第一页
  }

  const clearFilters = () => {
    filters.value = {
      status: 'success', // 保持默认的成功状态筛选
      search: '',
      architecture: ''
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
    filteredImages,
    
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
    updateImageStatus
  }
})
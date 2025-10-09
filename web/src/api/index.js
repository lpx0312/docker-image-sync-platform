import axios from 'axios'
import { ElMessage } from 'element-plus'

// 创建axios实例
const api = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// 请求拦截器
api.interceptors.request.use(
  (config) => {
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// 响应拦截器
api.interceptors.response.use(
  (response) => {
    return response.data
  },
  (error) => {
    const message = error.response?.data?.error || error.message || '请求失败'
    ElMessage.error(message)
    return Promise.reject(error)
  }
)

// 镜像同步相关API
export const syncAPI = {
  // 提交同步任务
  submitSync: (data) => api.post('/sync/submit', data),
  
  // 获取同步状态
  getSyncStatus: (taskId) => api.get(`/sync/status/${taskId}`),
  
  // 获取同步历史
  getSyncHistory: (params) => api.get('/sync/history', { params })
}

// 镜像管理相关API
export const imageAPI = {
  // 获取镜像列表
  getImageList: (params) => api.get('/images/list', { params }),
  
  // 获取镜像详情
  getImageDetail: (id) => api.get(`/images/${id}`),
  
  // 删除镜像记录
  deleteImage: (id) => api.delete(`/images/${id}`),
  
  // 获取镜像统计
  getImageStats: () => api.get('/images/stats'),
  
  // 重试同步
  retrySync: (id) => api.post(`/images/${id}/retry`)
}

// GitHub Actions相关API
export const githubAPI = {
  // 获取工作流运行列表
  getWorkflowRuns: (params) => api.get('/github/runs', { params }),
  
  // 获取工作流运行详情
  getWorkflowRunDetail: (runId) => api.get(`/github/runs/${runId}`),
  
  // 获取API速率限制
  getRateLimit: () => api.get('/github/rate-limit')
}

// 系统相关API
export const systemAPI = {
  // 健康检查
  healthCheck: () => api.get('/health')
}

export default api
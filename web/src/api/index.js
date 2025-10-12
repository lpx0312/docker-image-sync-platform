/**
 * API 接口模块
 * 
 * 功能说明：
 * - 提供统一的HTTP请求封装
 * - 包含镜像同步、镜像管理、GitHub Actions、系统相关的所有API接口
 * - 统一的错误处理和消息提示
 * - 自动的请求/响应拦截处理
 * 
 * 模块结构：
 * - syncAPI: 镜像同步相关接口
 * - imageAPI: 镜像管理相关接口  
 * - githubAPI: GitHub Actions相关接口
 * - systemAPI: 系统相关接口
 * 
 * @author Docker Image Sync Platform
 * @version 1.0.0
 */

import axios from 'axios'
import { ElMessage } from 'element-plus'

/**
 * 创建axios实例
 * 
 * 配置说明：
 * - baseURL: API基础路径，指向后端服务的v1版本接口
 * - timeout: 请求超时时间30秒，适合处理同步等耗时操作
 * - headers: 默认请求头，设置JSON格式
 */
const api = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

/**
 * 请求拦截器
 * 
 * 功能：
 * - 在请求发送前进行统一处理
 * - 可以添加认证token、请求日志等
 * - 当前为透传模式，保持请求原样
 */
api.interceptors.request.use(
  (config) => {
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

/**
 * 响应拦截器
 * 
 * 功能：
 * - 统一处理响应数据格式
 * - 自动提取响应体中的data字段
 * - 统一错误处理和用户提示
 * - 使用Element Plus的消息组件显示错误
 */
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

/**
 * 镜像同步相关API接口
 * 
 * 提供Docker镜像同步的完整功能，包括：
 * - 单个镜像同步
 * - 批量镜像同步
 * - 模拟同步测试
 * - 同步状态查询
 * - 同步历史记录
 */
export const syncAPI = {
  /**
   * 提交单个镜像同步任务
   * 
   * @param {Object} data - 同步任务数据
   * @param {string} data.original_image - 源镜像地址
   * @param {string} data.acr_image - 目标ACR镜像地址
   * @param {string} data.tag - 镜像标签
   * @param {string} [data.architecture] - 镜像架构 (amd64/arm64)
   * @returns {Promise} 返回任务ID和状态
   */
  submitSync: (data) => api.post('/sync/submit', data),
  
  /**
   * 提交批量镜像同步任务
   * 
   * @param {Object} data - 批量同步数据
   * @param {Array} data.images - 镜像列表
   * @param {boolean} [data.force] - 是否强制同步
   * @returns {Promise} 返回批量任务ID和状态
   */
  submitBatchSync: (data) => api.post('/sync/batch', data),
  
  /**
   * 提交模拟批量同步任务
   * 
   * 用于测试批量同步配置，不执行实际同步操作
   * 
   * @param {Object} data - 模拟同步数据
   * @param {Array} data.images - 镜像列表
   * @returns {Promise} 返回模拟结果
   */
  submitMockBatchSync: (data) => api.post('/sync/batch/mock', data),
  
  /**
   * 获取单个同步任务状态
   * 
   * @param {string} taskId - 任务ID
   * @returns {Promise} 返回任务详细状态信息
   */
  getSyncStatus: (taskId) => api.get(`/sync/status/${taskId}`),
  
  /**
   * 获取批量同步任务状态
   * 
   * @param {string} taskId - 批量任务ID
   * @returns {Promise} 返回批量任务状态和子任务列表
   */
  getBatchSyncStatus: (taskId) => api.get(`/sync/batch/status/${taskId}`),
  
  /**
   * 获取同步历史记录
   * 
   * @param {Object} params - 查询参数
   * @param {number} [params.page] - 页码
   * @param {number} [params.page_size] - 每页数量
   * @param {string} [params.status] - 状态筛选
   * @param {string} [params.image] - 镜像名筛选
   * @returns {Promise} 返回分页的历史记录列表
   */
  getSyncHistory: (params) => api.get('/sync/history', { params })
}

/**
 * 镜像管理相关API接口
 * 
 * 提供镜像记录的完整管理功能，包括：
 * - 镜像列表查询和分页
 * - 镜像详情查看
 * - 镜像记录删除
 * - 镜像统计信息
 * - 同步重试操作
 * - 镜像存在性检测
 */
export const imageAPI = {
  /**
   * 获取镜像列表
   * 
   * @param {Object} params - 查询参数
   * @param {number} [params.page=1] - 页码
   * @param {number} [params.page_size=10] - 每页数量
   * @param {string} [params.status] - 状态筛选 (pending/syncing/success/failed)
   * @param {string} [params.image] - 镜像名筛选
   * @param {string} [params.architecture] - 架构筛选
   * @returns {Promise} 返回分页的镜像列表
   */
  getImageList: (params) => api.get('/images/list', { params }),
  
  /**
   * 获取镜像详情
   * 
   * @param {string|number} id - 镜像记录ID
   * @returns {Promise} 返回镜像的详细信息
   */
  getImageDetail: (id) => api.get(`/images/${id}`),
  
  /**
   * 删除镜像记录
   * 
   * 注意：仅删除数据库记录，不影响实际镜像文件
   * 
   * @param {string|number} id - 镜像记录ID
   * @returns {Promise} 返回删除结果
   */
  deleteImage: (id) => api.delete(`/images/${id}`),
  
  /**
   * 获取镜像统计信息
   * 
   * @returns {Promise} 返回各状态镜像的数量统计
   * @returns {Object} result.total - 总数量
   * @returns {Object} result.pending - 待同步数量
   * @returns {Object} result.syncing - 同步中数量
   * @returns {Object} result.success - 成功数量
   * @returns {Object} result.failed - 失败数量
   */
  getImageStats: () => api.get('/images/stats'),
  
  /**
   * 重试镜像同步
   * 
   * 用于重新执行失败的同步任务
   * 
   * @param {string|number} id - 镜像记录ID
   * @returns {Promise} 返回重试任务的状态
   */
  retrySync: (id) => api.post(`/images/${id}/retry`),
  
  /**
   * 检测单个镜像是否存在
   * 
   * 检查目标镜像仓库中是否已存在该镜像
   * 
   * @param {string|number} id - 镜像记录ID
   * @returns {Promise} 返回镜像存在性检测结果
   */
  checkImageExists: (id) => api.post(`/images/${id}/check`),
  
  /**
   * 批量检测镜像存在性
   * 
   * @param {Array<string|number>} ids - 镜像记录ID列表
   * @returns {Promise} 返回批量检测结果
   */
  batchCheckImages: (ids) => api.post('/images/batch-check', { ids })
}

/**
 * GitHub Actions相关API接口
 * 
 * 提供GitHub Actions工作流的监控和管理功能，包括：
 * - 工作流运行记录查询
 * - 运行详情查看
 * - API速率限制监控
 */
export const githubAPI = {
  /**
   * 获取工作流运行列表
   * 
   * @param {Object} params - 查询参数
   * @param {number} [params.page] - 页码
   * @param {number} [params.page_size] - 每页数量
   * @param {string} [params.status] - 运行状态筛选
   * @param {string} [params.workflow] - 工作流名称筛选
   * @returns {Promise} 返回工作流运行记录列表
   */
  getWorkflowRuns: (params) => api.get('/github/runs', { params }),
  
  /**
   * 获取工作流运行详情
   * 
   * @param {string|number} runId - 工作流运行ID
   * @returns {Promise} 返回运行的详细信息，包括日志和步骤
   */
  getWorkflowRunDetail: (runId) => api.get(`/github/runs/${runId}`),
  
  /**
   * 获取GitHub API速率限制信息
   * 
   * @returns {Promise} 返回当前API调用限制和剩余次数
   * @returns {Object} result.limit - 每小时限制次数
   * @returns {Object} result.remaining - 剩余可用次数
   * @returns {Object} result.reset - 重置时间戳
   */
  getRateLimit: () => api.get('/github/rate-limit')
}

/**
 * 系统相关API接口
 * 
 * 提供系统状态和健康检查功能
 */
export const systemAPI = {
  /**
   * 系统健康检查
   * 
   * 检查后端服务的运行状态和基本信息
   * 
   * @returns {Promise} 返回系统健康状态
   * @returns {Object} result.status - 服务状态 (ok/error)
   * @returns {Object} result.timestamp - 检查时间戳
   * @returns {Object} result.version - 服务版本号
   */
  healthCheck: () => api.get('/health')
}

export default api
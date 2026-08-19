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

api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token') || sessionStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

let isRedirectingToLogin = false

api.interceptors.response.use(
  (response) => {
    return response.data
  },
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
      sessionStorage.removeItem('token')
      sessionStorage.removeItem('user')
      const currentPath = window.location.pathname
      if (currentPath !== '/login') {
        if (!isRedirectingToLogin) {
          isRedirectingToLogin = true
          window.location.href = '/login?redirect=' + encodeURIComponent(currentPath)
        }
      } else {
        isRedirectingToLogin = false
        const isLoginRequest = error.config?.url?.endsWith('/auth/login')
        if (isLoginRequest) {
          const message = error.response?.data?.error || '用户名或密码错误'
          ElMessage.error(message)
        }
      }
      return Promise.reject(error)
    }
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
  getSyncHistory: (params) => api.get('/sync/history', { params }),

  /**
   * 根据源镜像建议目标 ACR
   *
   * @param {string} image - 源镜像地址，如 nginx:1.21
   * @returns {Promise} 返回归属建议与配额摘要
   */
  suggestAcr: (image) => api.get('/sync/suggest-acr', { params: { image } }),

  /**
   * 批量检查镜像与所选 ACR 的归属冲突
   *
   * @param {Object} data
   * @param {string[]} data.images - 镜像列表
   * @param {number} [data.acr_registry_id] - 所选 ACR ID
   * @returns {Promise} 返回冲突检查结果
   */
  checkAcr: (data) => api.post('/sync/check-acr', data),
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
 * 提供系统状态、健康检查和配置管理功能
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
  healthCheck: () => api.get('/health'),

  /**
   * 获取Git仓库配置
   * 
   * 获取当前系统使用的Git仓库类型配置
   * 
   * @returns {Promise} 返回Git仓库配置
   * @returns {Object} result.data.repository_type - 仓库类型 ('gitee' | 'github')
   */
  getGitRepositoryConfig: () => api.get('/config/git-repository'),

  /**
   * 更新Git仓库配置
   * 
   * 更新系统使用的Git仓库类型
   * 
   * @param {string} repositoryType - 仓库类型 ('gitee' | 'github')
   * @returns {Promise} 返回更新结果
   */
  updateGitRepositoryConfig: (repositoryType) => api.put('/config/git-repository', {
    repository_type: repositoryType
  }),

  // ====================================================================
  // Git配置管理接口
  // ====================================================================

  /**
   * 获取Git配置
   * 
   * 获取完整的Git配置信息，包括Gitee和GitHub配置
   * 
   * @returns {Promise} 返回Git配置数据
   */
  getGitConfig: () => api.get('/config/git'),

  /**
   * 更新Gitee配置
   * 
   * @param {Object} config - Gitee配置数据
   * @param {string} config.repo_url - 仓库URL
   * @param {string} config.username - 用户名
   * @param {string} config.password - 密码
   * @param {string} config.email - 邮箱
   * @returns {Promise} 返回更新结果
   */
  updateGiteeConfig: (config) => api.put('/config/git/gitee', config),

  /**
   * 更新GitHub配置
   * 
   * @param {Object} config - GitHub配置数据
   * @param {string} config.repo_url - 仓库URL
   * @param {string} config.username - 用户名
   * @param {string} config.token - 访问令牌
   * @returns {Promise} 返回更新结果
   */
  updateGitHubConfig: (config) => api.put('/config/git/github', config),

  /**
   * 测试Git连接
   *
   * 测试指定Git仓库的连接状态
   *
   * @param {Object} config - 测试配置数据
   * @param {string} config.type - 仓库类型 ('gitee' | 'github')
   * @param {string} config.repo_url - 仓库URL
   * @param {string} config.username - 用户名
   * @param {string} config.password - 密码（Gitee）
   * @param {string} config.token - 访问令牌（GitHub）
   * @param {string} config.email - 邮箱
   * @returns {Promise} 返回连接测试结果
   */
  testGitConnection: (config) => api.post('/config/git/test', config),

  /**
   * 测试Git代码拉取和提交操作
   *
   * 测试GitHub仓库的代码拉取和提交功能
   *
   * @param {Object} config - 测试配置数据
   * @param {string} config.repo_url - 仓库URL
   * @param {string} config.username - 用户名
   * @param {string} config.token - 访问令牌
   * @param {string} config.email - 邮箱
   * @param {string} config.branch - 分支名称
   * @param {string} config.local_path - 本地仓库路径（API模式下不再需要，但保留向后兼容性）
   * @returns {Promise} 返回Git操作测试结果
   */
  testGitOperations: (config) => {
    console.log('API调用 testGitOperations, 配置:', config)
    return api.post('/config/git-test-operations', config).then(response => {
      console.log('API响应 testGitOperations:', response.data)
      return response
    }).catch(error => {
      console.error('API调用 testGitOperations 失败:', error)
      throw error
    })
  },

  // ====================================================================
  // 阿里云配置管理接口
  // ====================================================================

  /**
   * 获取阿里云配置
   * 
   * 获取阿里云容器镜像服务配置
   * 
   * @returns {Promise} 返回阿里云配置数据
   */
  getAliyunConfig: () => api.get('/config/aliyun-db'),

  /**
   * 更新阿里云配置
   * 
   * 更新阿里云镜像仓库的配置信息
   * 
   * @param {Object} config - 阿里云配置数据
   * @param {string} config.registry - 镜像仓库地址
   * @param {string} config.namespace - 命名空间
   * @param {string} config.username - 用户名
   * @param {string} config.password - 密码
   * @returns {Promise} 返回更新结果
   */
  updateAliyunConfig: (config) => api.put('/config/aliyun-db', config),

  /**
   * 测试阿里云连接
   * 
   * 测试阿里云镜像仓库的连接状态
   * 
   * @param {Object} config - 测试配置数据
   * @param {string} config.registry_url - 镜像仓库地址
   * @param {string} config.namespace - 命名空间
   * @param {string} config.username - 用户名
   * @param {string} config.password - 密码
   * @param {string} config.region - 地域
   * @returns {Promise} 返回连接测试结果
   */
  testAliyunConnection: (config) => api.post('/config/aliyun/test', config),

  // ====================================================================
  // 通用配置管理接口
  // ====================================================================

  /**
   * 获取所有配置
   * 
   * 获取系统所有配置项，按组分类
   * 
   * @returns {Promise} 返回所有配置数据
   */
  getAllConfigs: () => api.get('/config/all'),

  /**
   * 获取配置状态
   * 
   * 获取配置的完整性和有效性状态
   * 
   * @returns {Promise} 返回配置状态信息
   */
  getConfigStatus: () => api.get('/config/status')
}

/**
 * 认证相关API接口
 */
export const authAPI = {
  login: (data) => api.post('/auth/login', data),
  getCurrentUser: () => api.get('/auth/me'),
  changePassword: (data) => api.put('/auth/password', data),
  logout: () => api.post('/auth/logout'),
  getLoginLogs: (params) => api.get('/auth/login-logs', { params }),
  listUsers: (params) => api.get('/auth/users', { params }),
  createUser: (data) => api.post('/auth/users', data),
  updateUserStatus: (id, data) => api.put(`/auth/users/${id}/status`, data),
  updateUserRole: (id, data) => api.put(`/auth/users/${id}/role`, data),
  deleteUser: (id) => api.delete(`/auth/users/${id}`),
  resetUserPassword: (id, data) => api.put(`/auth/users/${id}/password`, data),
  getRoleOptions: () => api.get('/auth/roles/options'),
}

export const roleAPI = {
  listPermissions: () => api.get('/auth/permissions'),
  listRoles: () => api.get('/auth/roles'),
  createRole: (data) => api.post('/auth/roles', data),
  getRole: (id) => api.get(`/auth/roles/${id}`),
  updateRole: (id, data) => api.put(`/auth/roles/${id}`, data),
  deleteRole: (id) => api.delete(`/auth/roles/${id}`),
}

/**
 * ACR 配置管理 API
 *
 * 提供阿里云容器镜像服务（ACR）仓库配置的完整管理功能，包括：
 * - 获取所有 ACR 配置
 * - 根据 ID 获取单个配置
 * - 获取默认 ACR 配置
 * - 创建、更新、删除配置
 * - 设置默认 ACR
 */
export const acrRegistryAPI = {
  /**
   * 获取所有 ACR 配置
   *
   * @returns {Promise} 返回 ACR 配置列表
   */
  getAll: () => api.get('/acr-registries'),

  /**
   * 根据 ID 获取 ACR 配置
   *
   * @param {number} id - ACR 配置 ID
   * @returns {Promise} 返回单个 ACR 配置详情
   */
  getById: (id) => api.get(`/acr-registries/${id}`),

  /**
   * 获取默认 ACR 配置
   *
   * @returns {Promise} 返回默认 ACR 配置
   */
  getDefault: () => api.get('/acr-registries/default'),

  /**
   * 创建 ACR 配置
   *
   * @param {Object} data - ACR 配置数据
   * @param {string} data.registry_url - 仓库地址
   * @param {string} data.namespace - 命名空间
   * @param {string} data.username - 用户名
   * @param {string} data.password - 密码
   * @returns {Promise} 返回创建结果
   */
  create: (data) => api.post('/acr-registries', data),

  /**
   * 更新 ACR 配置
   *
   * @param {number} id - ACR 配置 ID
   * @param {Object} data - 需要更新的字段
   * @param {string} [data.registry_url] - 仓库地址
   * @param {string} [data.namespace] - 命名空间
   * @param {string} [data.username] - 用户名
   * @param {string} [data.password] - 密码
   * @returns {Promise} 返回更新结果
   */
  update: (id, data) => api.put(`/acr-registries/${id}`, data),

  /**
   * 删除 ACR 配置
   *
   * @param {number} id - ACR 配置 ID
   * @returns {Promise} 返回删除结果
   */
  delete: (id) => api.delete(`/acr-registries/${id}`),

  /**
   * 设置默认 ACR
   *
   * @param {number} id - ACR 配置 ID
   * @returns {Promise} 返回设置结果
   */
  setDefault: (id) => api.put(`/acr-registries/${id}/default`),

  /**
   * 获取所有 ACR 的仓库配额用量
   *
   * @returns {Promise} 返回各 ACR 的仓库数与剩余配额
   */
  getQuotaSummary: () => api.get('/acr-registries/quota-summary'),

  /**
   * 测试仓库配置连通性（登录凭证 + SWR 管理面 AK/SK）
   *
   * @param {number} id - 镜像仓库配置 ID
   * @returns {Promise} 返回 login_ok / manage_ok 等测试结果
   */
  test: (id) => api.post(`/acr-registries/${id}/test`),
}

// ACR 镜像管理 API
export const acrRepositoryAPI = {
  // 获取镜像列表
  getAll: (acrRegistryId) => {
    return api.get('/acr-repositories', { params: { acr_registry_id: acrRegistryId } })
  },

  // 添加镜像
  create: (data) => {
    return api.post('/acr-repositories', data)
  },

  // 批量添加镜像
  batchCreate: (data) => {
    return api.post('/acr-repositories/batch', data)
  },

  // 批量删除镜像
  batchDelete: (ids) => {
    return api.post('/acr-repositories/batch-delete', { ids })
  },

  // 清理无效镜像（本地存在但 ACR 中不存在）
  cleanInvalid: (acrRegistryId) => {
    return api.post('/acr-repositories/clean-invalid', { acr_registry_id: acrRegistryId })
  },

  // 删除镜像
  delete: (id) => {
    return api.delete(`/acr-repositories/${id}`)
  },

  // 从同步记录导入
  syncFromRecords: (acrRegistryId) => {
    return api.post('/acr-repositories/sync-from-records', { acr_registry_id: acrRegistryId })
  },

  // 从远程仓库导入镜像列表（/v2/_catalog；SWR 不支持）
  importFromRegistry: (acrRegistryId) => {
    return api.post('/acr-repositories/import-from-registry', { acr_registry_id: acrRegistryId })
  },

  // 获取跨仓库重复的仓库名
  getDuplicates: () => api.get('/acr-repositories/duplicates'),
}

// ACR Tag 查询 API
export const acrTagAPI = {
  // 获取 Tag 名称列表（轻量，不含详情）
  getTags: (acrRegistryId, repositoryName) => {
    return api.get('/acr-tags', {
      params: { acr_registry_id: acrRegistryId, repository_name: repositoryName },
      timeout: 30000,
    })
  },

  // 批量获取 Tag 详细信息
  getTagDetailsBatch: (acrRegistryId, repositoryName, tags) => {
    return api.get('/acr-tags/details', {
      params: {
        acr_registry_id: acrRegistryId,
        repository_name: repositoryName,
        tags: tags.join(','),
      },
      timeout: 60000,
    })
  },

  // 获取单个 Tag 详细信息
  getTagDetail: (acrRegistryId, repositoryName, tag) => {
    return api.get('/acr-tags/detail', { params: { acr_registry_id: acrRegistryId, repository_name: repositoryName, tag } })
  },
}

export default api
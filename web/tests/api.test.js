import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn() }
}))

import { ElMessage } from 'element-plus'
import api, {
  syncAPI,
  imageAPI,
  githubAPI,
  systemAPI,
  authAPI,
  roleAPI,
  acrRegistryAPI,
  acrRepositoryAPI,
  acrTagAPI
} from '@/api'

// 记录 adapter 收到的请求，返回可配置响应
let lastRequest = null
let nextResponse = { status: 200, data: {} }

function installAdapter() {
  api.defaults.adapter = async (config) => {
    lastRequest = config
    return {
      data: nextResponse.data,
      status: nextResponse.status,
      statusText: 'OK',
      headers: {},
      config
    }
  }
}

function makeError(status, data = {}, url = '/x') {
  const err = new Error('Request failed with status code ' + status)
  // 拦截器读取 error.config.url 与 error.response.*
  err.config = { url }
  err.response = { status, data, config: { url } }
  return err
}

describe('api 模块（真实 axios 实例 + mock adapter）', () => {
  beforeEach(() => {
    installAdapter()
    localStorage.clear()
    sessionStorage.clear()
    nextResponse = { status: 200, data: {} }
    lastRequest = null
    vi.clearAllMocks()
  })

  describe('请求拦截器：token 注入', () => {
    it('localStorage token 注入 Authorization', async () => {
      localStorage.setItem('token', 'local-tk')
      await systemAPI.healthCheck()
      expect(lastRequest.headers.Authorization).toBe('Bearer local-tk')
    })

    it('sessionStorage token 注入 Authorization', async () => {
      sessionStorage.setItem('token', 'sess-tk')
      await systemAPI.healthCheck()
      expect(lastRequest.headers.Authorization).toBe('Bearer sess-tk')
    })

    it('无 token 不注入', async () => {
      await systemAPI.healthCheck()
      expect(lastRequest.headers.Authorization).toBeUndefined()
    })
  })

  describe('响应拦截器：成功路径', () => {
    it('直接返回 response.data', async () => {
      nextResponse = { status: 200, data: { hello: 'world' } }
      const res = await systemAPI.healthCheck()
      expect(res).toEqual({ hello: 'world' })
    })
  })

  describe('响应拦截器：错误路径', () => {
    afterEach(() => {
      // 还原被测试改写的 location
      Object.defineProperty(window, 'location', {
        value: { href: 'about:blank', pathname: '/', assign: vi.fn(), replace: vi.fn() },
        writable: true
      })
    })

    function stubLocation(pathname) {
      Object.defineProperty(window, 'location', {
        value: { href: 'about:blank', pathname, assign: vi.fn(), replace: vi.fn() },
        writable: true
      })
    }

    it('非 401 错误提示服务端 error 文案', async () => {
      stubLocation('/')
      api.defaults.adapter = async (config) => {
        throw makeError(500, { error: '服务器炸了' })
      }
      await expect(systemAPI.healthCheck()).rejects.toMatchObject({ response: { status: 500 } })
      expect(ElMessage.error).toHaveBeenCalledWith('服务器炸了')
    })

    it('非 401 错误无 data.error 时退回 error.message', async () => {
      stubLocation('/')
      api.defaults.adapter = async () => {
        throw new Error('Network Error')
      }
      await expect(systemAPI.healthCheck()).rejects.toBeInstanceOf(Error)
      expect(ElMessage.error).toHaveBeenCalledWith('Network Error')
    })

    it('401 清空凭据并跳转登录页（带 redirect）', async () => {
      stubLocation('/images')
      localStorage.setItem('token', 'tk')
      localStorage.setItem('user', '{}')
      api.defaults.adapter = async (config) => {
        throw makeError(401, {}, '/auth/me')
      }
      await expect(systemAPI.getConfigStatus()).rejects.toMatchObject({ response: { status: 401 } })
      expect(localStorage.getItem('token')).toBe(null)
      expect(localStorage.getItem('user')).toBe(null)
      expect(window.location.href).toBe('/login?redirect=' + encodeURIComponent('/images'))
    })

    it('401 在登录页且为登录请求时提示用户名或密码错误', async () => {
      stubLocation('/login')
      api.defaults.adapter = async (config) => {
        throw makeError(401, { error: '密码错误' }, '/auth/login')
      }
      await expect(authAPI.login({ username: 'a', password: 'b' })).rejects.toBeTruthy()
      expect(ElMessage.error).toHaveBeenCalledWith('密码错误')
    })

    it('401 在登录页且无 data.error 时使用默认文案', async () => {
      stubLocation('/login')
      api.defaults.adapter = async (config) => {
        throw makeError(401, {}, '/auth/login')
      }
      await expect(authAPI.login({})).rejects.toBeTruthy()
      expect(ElMessage.error).toHaveBeenCalledWith('用户名或密码错误')
    })
  })

  describe('端点定义：方法与路径', () => {
    const cases = [
      ['syncAPI.submitSync', () => syncAPI.submitSync({}), 'post', '/sync/submit'],
      ['syncAPI.submitBatchSync', () => syncAPI.submitBatchSync({}), 'post', '/sync/batch'],
      ['syncAPI.submitMockBatchSync', () => syncAPI.submitMockBatchSync({}), 'post', '/sync/batch/mock'],
      ['syncAPI.getSyncStatus', () => syncAPI.getSyncStatus('t1'), 'get', '/sync/status/t1'],
      ['syncAPI.getBatchSyncStatus', () => syncAPI.getBatchSyncStatus('t1'), 'get', '/sync/batch/status/t1'],
      ['syncAPI.getSyncHistory', () => syncAPI.getSyncHistory({ page: 2 }), 'get', '/sync/history'],
      ['syncAPI.suggestAcr', () => syncAPI.suggestAcr('nginx'), 'get', '/sync/suggest-acr'],
      ['syncAPI.checkAcr', () => syncAPI.checkAcr({}), 'post', '/sync/check-acr'],
      ['imageAPI.getImageList', () => imageAPI.getImageList({ page: 1 }), 'get', '/images/list'],
      ['imageAPI.getImageDetail', () => imageAPI.getImageDetail(3), 'get', '/images/3'],
      ['imageAPI.deleteImage', () => imageAPI.deleteImage(3), 'delete', '/images/3'],
      ['imageAPI.getImageStats', () => imageAPI.getImageStats(), 'get', '/images/stats'],
      ['imageAPI.retrySync', () => imageAPI.retrySync(3), 'post', '/images/3/retry'],
      ['imageAPI.checkImageExists', () => imageAPI.checkImageExists(3), 'post', '/images/3/check'],
      ['imageAPI.batchCheckImages', () => imageAPI.batchCheckImages([1, 2]), 'post', '/images/batch-check'],
      ['githubAPI.getWorkflowRuns', () => githubAPI.getWorkflowRuns({}), 'get', '/github/runs'],
      ['githubAPI.getWorkflowRunDetail', () => githubAPI.getWorkflowRunDetail(9), 'get', '/github/runs/9'],
      ['githubAPI.getRateLimit', () => githubAPI.getRateLimit(), 'get', '/github/rate-limit'],
      ['githubAPI.getActionsUsage', () => githubAPI.getActionsUsage(), 'get', '/github/actions-usage'],
      ['systemAPI.healthCheck', () => systemAPI.healthCheck(), 'get', '/health'],
      ['systemAPI.getGitRepositoryConfig', () => systemAPI.getGitRepositoryConfig(), 'get', '/config/git-repository'],
      ['systemAPI.updateGitRepositoryConfig', () => systemAPI.updateGitRepositoryConfig('github'), 'put', '/config/git-repository'],
      ['systemAPI.getGitConfig', () => systemAPI.getGitConfig(), 'get', '/config/git'],
      ['systemAPI.updateGiteeConfig', () => systemAPI.updateGiteeConfig({}), 'put', '/config/git/gitee'],
      ['systemAPI.updateGitHubConfig', () => systemAPI.updateGitHubConfig({}), 'put', '/config/git/github'],
      ['systemAPI.testGitConnection', () => systemAPI.testGitConnection({}), 'post', '/config/git/test'],
      ['systemAPI.getAliyunConfig', () => systemAPI.getAliyunConfig(), 'get', '/config/aliyun-db'],
      ['systemAPI.updateAliyunConfig', () => systemAPI.updateAliyunConfig({}), 'put', '/config/aliyun-db'],
      ['systemAPI.testAliyunConnection', () => systemAPI.testAliyunConnection({}), 'post', '/config/aliyun/test'],
      ['systemAPI.getAllConfigs', () => systemAPI.getAllConfigs(), 'get', '/config/all'],
      ['systemAPI.getConfigStatus', () => systemAPI.getConfigStatus(), 'get', '/config/status'],
      ['authAPI.login', () => authAPI.login({}), 'post', '/auth/login'],
      ['authAPI.getCurrentUser', () => authAPI.getCurrentUser(), 'get', '/auth/me'],
      ['authAPI.changePassword', () => authAPI.changePassword({}), 'put', '/auth/password'],
      ['authAPI.logout', () => authAPI.logout(), 'post', '/auth/logout'],
      ['authAPI.getLoginLogs', () => authAPI.getLoginLogs({}), 'get', '/auth/login-logs'],
      ['authAPI.listUsers', () => authAPI.listUsers({}), 'get', '/auth/users'],
      ['authAPI.createUser', () => authAPI.createUser({}), 'post', '/auth/users'],
      ['authAPI.updateUserStatus', () => authAPI.updateUserStatus(1, {}), 'put', '/auth/users/1/status'],
      ['authAPI.updateUserRole', () => authAPI.updateUserRole(1, {}), 'put', '/auth/users/1/role'],
      ['authAPI.deleteUser', () => authAPI.deleteUser(1), 'delete', '/auth/users/1'],
      ['authAPI.resetUserPassword', () => authAPI.resetUserPassword(1, {}), 'put', '/auth/users/1/password'],
      ['authAPI.getRoleOptions', () => authAPI.getRoleOptions(), 'get', '/auth/roles/options'],
      ['roleAPI.listPermissions', () => roleAPI.listPermissions(), 'get', '/auth/permissions'],
      ['roleAPI.listRoles', () => roleAPI.listRoles(), 'get', '/auth/roles'],
      ['roleAPI.createRole', () => roleAPI.createRole({}), 'post', '/auth/roles'],
      ['roleAPI.getRole', () => roleAPI.getRole(2), 'get', '/auth/roles/2'],
      ['roleAPI.updateRole', () => roleAPI.updateRole(2, {}), 'put', '/auth/roles/2'],
      ['roleAPI.deleteRole', () => roleAPI.deleteRole(2), 'delete', '/auth/roles/2'],
      ['acrRegistryAPI.getAll', () => acrRegistryAPI.getAll(), 'get', '/acr-registries'],
      ['acrRegistryAPI.getById', () => acrRegistryAPI.getById(1), 'get', '/acr-registries/1'],
      ['acrRegistryAPI.getDefault', () => acrRegistryAPI.getDefault(), 'get', '/acr-registries/default'],
      ['acrRegistryAPI.create', () => acrRegistryAPI.create({}), 'post', '/acr-registries'],
      ['acrRegistryAPI.update', () => acrRegistryAPI.update(1, {}), 'put', '/acr-registries/1'],
      ['acrRegistryAPI.delete', () => acrRegistryAPI.delete(1), 'delete', '/acr-registries/1'],
      ['acrRegistryAPI.setDefault', () => acrRegistryAPI.setDefault(1), 'put', '/acr-registries/1/default'],
      ['acrRegistryAPI.getQuotaSummary', () => acrRegistryAPI.getQuotaSummary(), 'get', '/acr-registries/quota-summary'],
      ['acrRegistryAPI.test', () => acrRegistryAPI.test(1), 'post', '/acr-registries/1/test'],
      ['acrRepositoryAPI.getDuplicates', () => acrRepositoryAPI.getDuplicates(), 'get', '/acr-repositories/duplicates']
    ]

    it.each(cases)('%s -> %s %s', async (name, call, method, path) => {
      await call()
      expect(lastRequest.method).toBe(method)
      expect(lastRequest.url.replace('/api/v1', '')).toBe(path)
    })
  })

  describe('参数化端点', () => {
    it('syncAPI.getSyncHistory 透传查询参数', async () => {
      await syncAPI.getSyncHistory({ status: 'failed', page: 3 })
      expect(lastRequest.params).toMatchObject({ status: 'failed', page: 3 })
    })

    it('imageAPI.getImageList 透传查询参数', async () => {
      await imageAPI.getImageList({ search: 'nginx', page_size: 50 })
      expect(lastRequest.params).toMatchObject({ search: 'nginx', page_size: 50 })
    })

    it('acrRepositoryAPI.getAll 带仓库 ID 查询参数', async () => {
      await acrRepositoryAPI.getAll(7)
      expect(lastRequest.url.replace('/api/v1', '')).toBe('/acr-repositories')
      expect(lastRequest.params).toMatchObject({ acr_registry_id: 7 })
    })

    it('acrTagAPI.getTags 组装查询参数', async () => {
      await acrTagAPI.getTags(1, 'nginx')
      expect(lastRequest.url.replace('/api/v1', '')).toBe('/acr-tags')
      expect(lastRequest.params).toMatchObject({ acr_registry_id: 1, repository_name: 'nginx' })
    })

    it('acrTagAPI.getTagDetail 组装查询参数', async () => {
      await acrTagAPI.getTagDetail(1, 'nginx', '1.25')
      expect(lastRequest.params).toMatchObject({ acr_registry_id: 1, repository_name: 'nginx', tag: '1.25' })
    })

    it('acrTagAPI.getTagDetailsBatch tags 数组转逗号串', async () => {
      await acrTagAPI.getTagDetailsBatch(1, 'nginx', ['1.24', '1.25'])
      expect(lastRequest.params).toMatchObject({ acr_registry_id: 1, repository_name: 'nginx', tags: '1.24,1.25' })
    })

    it('acrRepositoryAPI.batchCreate 透传 body', async () => {
      const body = { acr_registry_id: 1, repository_names: ['a'] }
      await acrRepositoryAPI.batchCreate(body)
      expect(JSON.parse(lastRequest.data)).toEqual(body)
    })

    it('systemAPI.testGitOperations 组装操作开关 body', async () => {
      await systemAPI.testGitOperations({ test_commit: true })
      const body = JSON.parse(lastRequest.data)
      expect(lastRequest.url.replace('/api/v1', '')).toBe('/config/git-test-operations')
      expect(body).toMatchObject({ test_commit: true })
    })
  })
})

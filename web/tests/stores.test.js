import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api', () => ({
  authAPI: {
    login: vi.fn(),
    logout: vi.fn(),
    getCurrentUser: vi.fn(),
    changePassword: vi.fn()
  },
  imageAPI: {
    getImageList: vi.fn(),
    getImageStats: vi.fn(),
    deleteImage: vi.fn(),
    retrySync: vi.fn(),
    getImageDetail: vi.fn(),
    checkImageExists: vi.fn(),
    batchCheckImages: vi.fn()
  },
  syncAPI: {
    submitSync: vi.fn(),
    getSyncStatus: vi.fn(),
    getBatchSyncStatus: vi.fn(),
    getSyncHistory: vi.fn()
  }
}))

import { authAPI, imageAPI, syncAPI } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useImageStore } from '@/stores/image'
import { useSyncStore } from '@/stores/sync'

describe('useAuthStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    sessionStorage.clear()
    vi.clearAllMocks()
  })

  it('初始状态未登录', () => {
    const s = useAuthStore()
    expect(s.isLoggedIn).toBe(false)
    expect(s.isAdmin).toBe(false)
    expect(s.username).toBe('')
    expect(s.hasPermission('sync')).toBe(false)
  })

  it('login（记住我）写入 localStorage 并清 sessionStorage', async () => {
    authAPI.login.mockResolvedValue({
      token: 'tk-1',
      user: { username: 'admin', role_code: 'admin', permissions: ['sync', 'config'] }
    })
    const s = useAuthStore()
    await s.login('admin', 'pw', true)

    expect(s.isLoggedIn).toBe(true)
    expect(s.isAdmin).toBe(true)
    expect(s.username).toBe('admin')
    expect(s.hasPermission('config')).toBe(true)
    expect(localStorage.getItem('token')).toBe('tk-1')
    expect(sessionStorage.getItem('token')).toBe(null)
  })

  it('login（非记住我）写入 sessionStorage 并清 localStorage', async () => {
    localStorage.setItem('token', 'old')
    authAPI.login.mockResolvedValue({ token: 'tk-2', user: { username: 'u' } })
    const s = useAuthStore()
    await s.login('u', 'pw', false)
    expect(sessionStorage.getItem('token')).toBe('tk-2')
    expect(localStorage.getItem('token')).toBe(null)
  })

  it('logout 调用 API 并清空状态；API 失败也照常清理', async () => {
    authAPI.login.mockResolvedValue({ token: 'tk', user: { username: 'a' } })
    const s = useAuthStore()
    await s.login('a', 'p')
    authAPI.logout.mockRejectedValue(new Error('net'))
    await s.logout()
    expect(s.isLoggedIn).toBe(false)
    expect(s.user).toBe(null)
    expect(localStorage.getItem('token')).toBe(null)
    expect(authAPI.logout).toHaveBeenCalled()
  })

  it('未登录时 logout 不调用 API', async () => {
    const s = useAuthStore()
    await s.logout()
    expect(authAPI.logout).not.toHaveBeenCalled()
  })

  it('fetchCurrentUser 失败时清空认证', async () => {
    authAPI.getCurrentUser.mockRejectedValue(new Error('401'))
    sessionStorage.setItem('token', 'tk')
    const s = useAuthStore()
    await s.fetchCurrentUser()
    expect(s.isLoggedIn).toBe(false)
  })

  it('fetchCurrentUser 成功刷新用户与存储', async () => {
    authAPI.getCurrentUser.mockResolvedValue({ username: 'admin', role_code: 'admin' })
    sessionStorage.setItem('token', 'tk')
    const s = useAuthStore()
    await s.fetchCurrentUser()
    expect(s.username).toBe('admin')
    expect(JSON.parse(sessionStorage.getItem('user')).username).toBe('admin')
  })

  it('changePassword 后清空认证', async () => {
    authAPI.login.mockResolvedValue({ token: 'tk', user: { username: 'a' } })
    const s = useAuthStore()
    await s.login('a', 'p')
    await s.changePassword('old', 'New#1234')
    expect(s.isLoggedIn).toBe(false)
  })

  it('updateActivityTime 刷新活跃时间', () => {
    const s = useAuthStore()
    const before = s.lastActivityTime
    s.updateActivityTime()
    expect(s.lastActivityTime).toBeGreaterThanOrEqual(before)
  })

  it('页面刷新后从存储恢复登录态', async () => {
    localStorage.setItem('token', 'tk')
    localStorage.setItem('user', JSON.stringify({ username: 'admin', permissions: ['sync'] }))
    const s = useAuthStore()
    expect(s.isLoggedIn).toBe(true)
    expect(s.username).toBe('admin')
    s.stopAutoLogoutTimer()
  })
})

describe('useImageStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('loadImages 更新列表与分页，参数含过滤与排序', async () => {
    imageAPI.getImageList.mockResolvedValue({ total: 2, data: [{ id: 1 }, { id: 2 }] })
    const s = useImageStore()
    await s.loadImages()
    expect(s.images).toHaveLength(2)
    expect(s.pagination.total).toBe(2)
    expect(s.hasImages).toBe(true)
    const params = imageAPI.getImageList.mock.calls[0][0]
    expect(params.page).toBe(1)
    expect(params.page_size).toBe(20)
    expect(params.deduplicate).toBe(true)
    expect(params.sort_by).toBe('created_at')
  })

  it('loadImages 空响应兜底', async () => {
    imageAPI.getImageList.mockResolvedValue({})
    const s = useImageStore()
    await s.loadImages()
    expect(s.images).toEqual([])
    expect(s.pagination.total).toBe(0)
    expect(s.hasImages).toBe(false)
  })

  it('updateFilters/clearFilters/updatePagination/updateSorting', () => {
    const s = useImageStore()
    s.updatePagination(3, 50)
    expect(s.pagination).toMatchObject({ page: 3, pageSize: 50 })

    s.updateFilters({ status: 'failed', search: 'ng' })
    expect(s.filters.status).toBe('failed')
    expect(s.pagination.page).toBe(1)

    s.updateSorting('sync_status', 'asc')
    expect(s.sorting).toMatchObject({ sortBy: 'sync_status', sortOrder: 'asc' })
    expect(s.pagination.page).toBe(1)

    s.clearFilters()
    expect(s.filters).toMatchObject({ status: '', search: '', architecture: '', deduplicate: true })
  })

  it('deleteImage 本地移除并重载统计', async () => {
    imageAPI.getImageList.mockResolvedValue({ total: 2, data: [{ id: 1 }, { id: 2 }] })
    imageAPI.getImageStats.mockResolvedValue({ total: 1 })
    const s = useImageStore()
    await s.loadImages()
    await s.deleteImage(1)
    expect(s.images.map((i) => i.id)).toEqual([2])
    expect(s.pagination.total).toBe(1)
    expect(imageAPI.getImageStats).toHaveBeenCalled()
  })

  it('deleteImage 列表中不存在时仅重载统计', async () => {
    imageAPI.getImageList.mockResolvedValue({ total: 1, data: [{ id: 9 }] })
    imageAPI.getImageStats.mockResolvedValue({ total: 1 })
    const s = useImageStore()
    await s.loadImages()
    await s.deleteImage(123)
    expect(s.images).toHaveLength(1)
    expect(s.pagination.total).toBe(1)
  })

  it('retrySync 更新本地状态为 pending', async () => {
    imageAPI.getImageList.mockResolvedValue({ total: 1, data: [{ id: 5, sync_status: 'failed' }] })
    imageAPI.getImageStats.mockResolvedValue({})
    imageAPI.retrySync.mockResolvedValue({})
    const s = useImageStore()
    await s.loadImages()
    await s.retrySync(5)
    expect(s.images[0].sync_status).toBe('pending')
    expect(s.images[0].updated_at).toBeTruthy()
  })

  it('checkImageExists 存在时更新成功状态', async () => {
    imageAPI.getImageList.mockResolvedValue({ total: 1, data: [{ id: 5, sync_status: 'failed' }] })
    imageAPI.getImageStats.mockResolvedValue({})
    imageAPI.checkImageExists.mockResolvedValue({ exists: true })
    const s = useImageStore()
    await s.loadImages()
    const resp = await s.checkImageExists(5)
    expect(resp.exists).toBe(true)
    expect(s.images[0].sync_status).toBe('success')
  })

  it('updateImageStatus 带 message 写入错误信息', () => {
    imageAPI.getImageList.mockResolvedValue({ total: 1, data: [{ id: 5 }] })
    const s = useImageStore()
    s.images = [{ id: 5, sync_status: 'syncing' }]
    s.updateImageStatus(5, 'failed', 'boom')
    expect(s.images[0].sync_status).toBe('failed')
    expect(s.images[0].error_message).toBe('boom')
  })

  it('getImageById 透传详情', async () => {
    imageAPI.getImageDetail.mockResolvedValue({ id: 7 })
    const s = useImageStore()
    await expect(s.getImageById(7)).resolves.toEqual({ id: 7 })
  })

  it('batchCheckImages 后重载列表与统计', async () => {
    imageAPI.getImageList.mockResolvedValue({ total: 1, data: [{ id: 5 }] })
    imageAPI.getImageStats.mockResolvedValue({})
    imageAPI.batchCheckImages.mockResolvedValue({ total: 1 })
    const s = useImageStore()
    const resp = await s.batchCheckImages([5])
    expect(resp.total).toBe(1)
    expect(imageAPI.getImageList).toHaveBeenCalled()
  })
})

describe('useSyncStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('初始无当前任务', () => {
    const s = useSyncStore()
    expect(s.hasCurrentTask).toBe(false)
    expect(s.taskStatus).toBe(null)
  })

  it('submitSync 设置当前任务', async () => {
    syncAPI.submitSync.mockResolvedValue({ task_id: 't1', status: 'pending' })
    const s = useSyncStore()
    const resp = await s.submitSync({ images: ['nginx'] })
    expect(resp.task_id).toBe('t1')
    expect(s.hasCurrentTask).toBe(true)
    expect(s.taskStatus).toBe('pending')
    expect(s.loading).toBe(false)
  })

  it('getSyncStatus 命中当前任务时合并状态', async () => {
    syncAPI.submitSync.mockResolvedValue({ task_id: 't1', status: 'running', progress: 0 })
    const s = useSyncStore()
    await s.submitSync({})
    syncAPI.getSyncStatus.mockResolvedValue({ task_id: 't1', status: 'completed', progress: 100 })
    await s.getSyncStatus('t1')
    expect(s.currentTask.status).toBe('completed')
    expect(s.currentTask.progress).toBe(100)
  })

  it('getSyncStatus 非当前任务不合并', async () => {
    syncAPI.submitSync.mockResolvedValue({ task_id: 't1', status: 'running' })
    const s = useSyncStore()
    await s.submitSync({})
    syncAPI.getSyncStatus.mockResolvedValue({ task_id: 'other', status: 'completed' })
    await s.getSyncStatus('other')
    expect(s.currentTask.status).toBe('running')
  })

  it('getBatchSyncStatus 命中当前任务时合并', async () => {
    syncAPI.submitSync.mockResolvedValue({ task_id: 'b1', status: 'running' })
    const s = useSyncStore()
    await s.submitSync({})
    syncAPI.getBatchSyncStatus.mockResolvedValue({ task_id: 'b1', failed_images: 2 })
    await s.getBatchSyncStatus('b1')
    expect(s.currentTask.failed_images).toBe(2)
  })

  it('loadSyncHistory 更新历史并控制 loading', async () => {
    syncAPI.getSyncHistory.mockResolvedValue({ data: [{ task_id: 'h1' }] })
    const s = useSyncStore()
    await s.loadSyncHistory({ page: 1 })
    expect(s.syncHistory).toHaveLength(1)
    expect(s.loading).toBe(false)
  })

  it('clearCurrentTask 与 updateTaskStatus', async () => {
    syncAPI.submitSync.mockResolvedValue({ task_id: 't1', status: 'running' })
    const s = useSyncStore()
    await s.submitSync({})
    s.updateTaskStatus('t1', 'failed', '超时')
    expect(s.currentTask.status).toBe('failed')
    expect(s.currentTask.message).toBe('超时')

    s.syncHistory = [{ task_id: 't1', status: 'running' }]
    s.updateTaskStatus('t1', 'completed')
    expect(s.syncHistory[0].status).toBe('completed')

    s.clearCurrentTask()
    expect(s.hasCurrentTask).toBe(false)
  })
})

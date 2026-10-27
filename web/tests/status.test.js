import { describe, it, expect } from 'vitest'
import {
  getSyncStatusType,
  getSyncStatusText,
  getGitHubStatusType,
  getGitHubStatusText,
  getGitHubConclusionType,
  getGitHubConclusionText,
  getStatusType,
  getStatusText
} from '@/utils/status'

describe('同步状态映射', () => {
  it.each([
    ['pending', 'info'],
    ['syncing', 'warning'],
    ['success', 'success'],
    ['failed', 'danger']
  ])('%s -> 类型 %s', (status, type) => {
    expect(getSyncStatusType(status)).toBe(type)
  })

  it.each([
    ['pending', '等待中'],
    ['syncing', '同步中'],
    ['success', '成功'],
    ['failed', '失败']
  ])('%s -> 文本 %s', (status, text) => {
    expect(getSyncStatusText(status)).toBe(text)
  })

  it('未知状态兜底', () => {
    expect(getSyncStatusType('whatever')).toBe('info')
    expect(getSyncStatusText('whatever')).toBe('未知')
    expect(getSyncStatusType(undefined)).toBe('info')
    expect(getSyncStatusText(null)).toBe('未知')
  })
})

describe('GitHub 状态映射', () => {
  it.each([
    ['pending', 'info'],
    ['queued', 'info'],
    ['in_progress', 'warning'],
    ['completed', 'success']
  ])('%s -> 类型 %s', (status, type) => {
    expect(getGitHubStatusType(status)).toBe(type)
  })

  it.each([
    ['pending', '等待中'],
    ['queued', '排队中'],
    ['in_progress', '执行中'],
    ['completed', '已完成']
  ])('%s -> 文本 %s', (status, text) => {
    expect(getGitHubStatusText(status)).toBe(text)
  })

  it('未知状态兜底', () => {
    expect(getGitHubStatusType('xxx')).toBe('info')
    expect(getGitHubStatusText('xxx')).toBe('未知')
  })
})

describe('GitHub 结论映射', () => {
  it.each([
    ['success', 'success', '成功'],
    ['failure', 'danger', '失败'],
    ['cancelled', 'warning', '已取消'],
    ['skipped', 'info', '已跳过'],
    ['timed_out', 'danger', '超时']
  ])('%s -> %s/%s', (conclusion, type, text) => {
    expect(getGitHubConclusionType(conclusion)).toBe(type)
    expect(getGitHubConclusionText(conclusion)).toBe(text)
  })

  it('未知结论兜底', () => {
    expect(getGitHubConclusionType('zzz')).toBe('info')
    expect(getGitHubConclusionText('zzz')).toBe('未知')
  })
})

describe('统一入口 getStatusType/getStatusText', () => {
  it('默认按 sync 类型分发', () => {
    expect(getStatusType('failed')).toBe('danger')
    expect(getStatusText('failed')).toBe('失败')
  })

  it('github-status 分发', () => {
    expect(getStatusType('in_progress', 'github-status')).toBe('warning')
    expect(getStatusText('in_progress', 'github-status')).toBe('执行中')
  })

  it('github-conclusion 分发', () => {
    expect(getStatusType('timed_out', 'github-conclusion')).toBe('danger')
    expect(getStatusText('timed_out', 'github-conclusion')).toBe('超时')
  })

  it('未知分发类型兜底', () => {
    expect(getStatusType('failed', 'bogus')).toBe('info')
    expect(getStatusText('failed', 'bogus')).toBe('未知')
  })
})

import { describe, it, expect } from 'vitest'
import { formatTime, formatDuration, formatSize } from '@/utils/format'

describe('formatTime', () => {
  it('空值返回占位符', () => {
    expect(formatTime(null)).toBe('-')
    expect(formatTime('')).toBe('-')
    expect(formatTime(0)).toBe('-')
  })

  it('标准格式化', () => {
    expect(formatTime('2024-01-02T03:04:05')).toMatch(/^2024-01-02 03:04:05$/)
  })

  it('自定义格式', () => {
    expect(formatTime('2024-01-02T03:04:05', 'YYYY/MM/DD')).toBe('2024/01/02')
  })

  it('Date 对象输入', () => {
    expect(formatTime(new Date(2024, 0, 2, 3, 4, 5), 'YYYY-MM-DD')).toBe('2024-01-02')
  })
})

describe('formatDuration', () => {
  it('空值与非正数返回占位符', () => {
    expect(formatDuration(0)).toBe('-')
    expect(formatDuration(null)).toBe('-')
    expect(formatDuration(-5)).toBe('-')
  })

  it('秒级', () => {
    expect(formatDuration(45)).toBe('45秒')
  })

  it('分级（含秒）', () => {
    expect(formatDuration(90)).toBe('1分30秒')
  })

  it('整分（无秒）', () => {
    expect(formatDuration(60)).toBe('1分')
  })

  it('小时级（含分）', () => {
    expect(formatDuration(3661)).toBe('1小时1分钟')
  })

  it('整小时', () => {
    expect(formatDuration(7200)).toBe('2小时')
  })
})

describe('formatSize', () => {
  it('空值与非正数', () => {
    expect(formatSize(0)).toBe('0 B')
    expect(formatSize(null)).toBe('0 B')
    expect(formatSize(-1)).toBe('0 B')
  })

  it('B 级', () => {
    expect(formatSize(100)).toBe('100.00 B')
  })

  it('KB/MB/GB 逐级进位', () => {
    expect(formatSize(1024)).toBe('1.00 KB')
    expect(formatSize(1024 * 1024)).toBe('1.00 MB')
    expect(formatSize(1024 * 1024 * 1024)).toBe('1.00 GB')
  })

  it('TB 封顶（不再进位）', () => {
    const pb = 1024 ** 5 // 1PB
    expect(formatSize(pb)).toBe('1024.00 TB')
  })

  it('非整数值保留两位', () => {
    expect(formatSize(1536)).toBe('1.50 KB')
  })
})

import request from './request'
import type { CrawlerSpiderConfig, CrawlerRunLog, CrawlerRunProgress, PageResult } from '@/types'

export interface PlatformInfo {
  code: string
  name: string
  table: string
  lastSyncTime?: string
  sourceCount: number  // 源表（MediaCrawler平台表）记录数
  centralCount: number // 中心表（articles）记录数
}

export interface PlatformSyncProgress {
  platform: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  totalCount: number
  processedCount: number
  newCount: number
  skippedCount: number
  errorCount: number
  startTime: string
  endTime?: string
  duration?: string
  errorMessage?: string
}

export interface PlatformSyncResult {
  platform: string
  totalCount: number
  newCount: number
  skippedCount: number
  errorCount: number
  startTime: string
  endTime: string
  duration: string
  status: string
  errorMessage?: string
}

export const crawlerApi = {
  listSpiders: () => request.get<never, CrawlerSpiderConfig[]>('/crawler/spiders'),

  putSpiders: (spiders: Array<{ spiderKey: string; intervalMinutes: number; enabled: number }>) =>
    request.put<never, CrawlerSpiderConfig[]>('/crawler/spiders', { spiders }),

  runNow: (spiders?: string[]) =>
    request.post<never, { id: number }>('/crawler/run', spiders?.length ? { spiders } : {}),

  runAdvanced: (payload: {
    spiders: string[]
    keywords?: string[]
    topics?: string[]
    startAt?: string
    endAt?: string
  }) => request.post<never, { id: number }>('/crawler/run', payload),

  listRuns: (params?: { page?: number; pageSize?: number }) =>
    request.get<never, PageResult<CrawlerRunLog>>('/crawler/runs', { params }),

  getRunProgress: (id: number) =>
    request.get<never, CrawlerRunProgress>(`/crawler/progress/${id}`),

  getRun: (id: number) => request.get<never, CrawlerRunLog>(`/crawler/runs/${id}`),
}

// 平台数据同步 API（重构版）
export const platformSyncApi = {
  // 获取平台列表
  getPlatformList: () => request.get<never, PlatformInfo[]>('/platform/list'),

  // 获取同步状态
  getStatus: () => request.get<never, PlatformInfo[]>('/platform/sync/status'),

  // 获取同步进度（实时）
  getProgress: (platform?: string) =>
    request.get<never, PlatformSyncProgress | PlatformSyncProgress[]>('/platform/sync/progress', {
      params: platform ? { platform } : undefined,
    }),

  // 手动触发同步（支持多平台）
  syncPlatforms: (platforms: string[]) =>
    request.post<never, { [platform: string]: PlatformSyncResult }>('/platform/sync', {
      platforms,
    }),

  // 同步所有平台
  syncAll: () =>
    request.post<never, { [platform: string]: PlatformSyncResult }>('/platform/sync/all'),
}


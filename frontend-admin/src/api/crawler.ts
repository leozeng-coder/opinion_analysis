import request from './request'
import type { CrawlerSpiderConfig, CrawlerRunLog, CrawlerRunProgress, PageResult } from '@/types'

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

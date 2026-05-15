import request from './request'
import type {
  CrawlerSpiderConfig,
  CrawlerRunLog,
  CrawlerRunRequest,
  CrawlerRunProgress,
  PageData,
} from '@/types'

export const crawlerApi = {
  listSpiders: () => request.get<never, CrawlerSpiderConfig[]>('/crawler/spiders'),

  putSpiders: (spiders: Pick<CrawlerSpiderConfig, 'spiderKey' | 'intervalMinutes' | 'enabled'>[]) =>
    request.put<never, CrawlerSpiderConfig[]>('/crawler/spiders', { spiders }),

  runNow: (spiders?: string[]) =>
    request.post<never, { id: number }>('/crawler/run', spiders?.length ? { spiders } : {}),

  runAdvanced: (payload: CrawlerRunRequest) =>
    request.post<never, { id: number }>('/crawler/run', payload),

  getRun: (id: number) => request.get<never, CrawlerRunLog>(`/crawler/runs/${id}`),

  getRunProgress: (id: number) =>
    request.get<never, CrawlerRunProgress>(`/crawler/progress/${id}`),

  listRuns: (params?: { page?: number; pageSize?: number }) =>
    request.get<never, PageData<CrawlerRunLog>>('/crawler/runs', { params }),
}

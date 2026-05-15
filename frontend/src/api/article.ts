import request from './request'
import type { Article, ArticleStats, PageData } from '@/types'

export interface ArticleQuery {
  page?: number
  pageSize?: number
  platform?: string
  sentiment?: string
  keyword?: string
  startAt?: string
  endAt?: string
}

export const articleApi = {
  list: (params: ArticleQuery) =>
    request.get<never, PageData<Article>>('/articles', { params }),

  detail: (id: number) =>
    request.get<never, Article>(`/articles/${id}`),

  stats: (params?: { startAt?: string; endAt?: string }) =>
    request.get<never, ArticleStats>('/articles/stats', { params }),

  platforms: () =>
    request.get<never, string[]>('/articles/platforms'),
}

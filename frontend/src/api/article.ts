import request from './request'
import type { Article, ArticleStats, TagCount, PageData } from '@/types'

export interface ArticleQuery {
  page?: number
  pageSize?: number
  platform?: string
  sentiment?: string
  keyword?: string
  startAt?: string
  endAt?: string
  tags?: string    // 逗号分隔的 AI 标签，OR 关系
}

export interface TagsQuery {
  startAt?: string
  endAt?: string
  platform?: string
  limit?: number
}

export const articleApi = {
  list: (params: ArticleQuery) =>
    request.get<PageData<Article>>('/articles', { params }),

  detail: (id: number) =>
    request.get<Article>(`/articles/${id}`),

  stats: (params?: { startAt?: string; endAt?: string }) =>
    request.get<ArticleStats>('/articles/stats', { params }),

  platforms: () =>
    request.get<string[]>('/articles/platforms'),

  tags: (params?: TagsQuery) =>
    request.get<TagCount[]>('/articles/tags', { params }),
}

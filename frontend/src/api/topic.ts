import request from './request'
import type { Topic, PageData } from '@/types'

export const topicApi = {
  list: (params?: { page?: number; pageSize?: number }) =>
    request.get<PageData<Topic>>('/topics', { params }),

  detail: (id: number) =>
    request.get<Topic>(`/topics/${id}`),
}

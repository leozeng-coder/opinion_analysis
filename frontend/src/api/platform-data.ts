import request from '@/api/request'

export interface PlatformDataQuery {
  platform?: string
  startDate?: string
  endDate?: string
  page: number
  pageSize: number
}

export interface PlatformDataItem {
  id: number
  platform: string
  title: string
  content: string
  author: string
  avatar?: string
  url: string
  publishTime: string | null
  likeCount?: number
  commentCount?: number
  shareCount?: number
  viewCount?: number
  collectCount?: number
  coverUrl?: string
  ipLocation?: string
}

export interface PlatformDataResponse {
  data: PlatformDataItem[]
  total: number
}

export const platformDataApi = {
  list: (params: PlatformDataQuery): Promise<PlatformDataResponse> =>
    request.get('/platform/data', { params }),
}

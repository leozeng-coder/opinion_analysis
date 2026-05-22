import request from '@/api/request'

export interface PlatformCommentQuery {
  platform: string
  itemId: number
  page: number
  pageSize: number
}

export interface PlatformCommentItem {
  id: number
  commentId: string
  parentCommentId?: string
  content: string
  userId: string
  nickname: string
  avatar?: string
  ipLocation?: string
  createTime: string | null
  likeCount?: number
  subCommentCount?: number
}

export interface PlatformCommentResponse {
  data: PlatformCommentItem[]
  total: number
}

export const platformCommentApi = {
  list: (params: PlatformCommentQuery): Promise<PlatformCommentResponse> =>
    request.get('/platform/comments', { params }),
}

import request from './request'
import type { DataSource } from '@/types'

interface DSReq {
  name: string
  type: string
  url?: string
  config?: string
  status?: number
}

export const adminDataSourceApi = {
  list: () => request.get<never, DataSource[]>('/admin/data-sources'),

  create: (data: DSReq) => request.post<never, DataSource>('/admin/data-sources', data),

  update: (id: number, data: DSReq) =>
    request.put<never, DataSource>(`/admin/data-sources/${id}`, data),

  delete: (id: number) => request.delete(`/admin/data-sources/${id}`),
}

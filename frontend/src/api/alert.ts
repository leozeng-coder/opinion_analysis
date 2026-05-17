import request from './request'
import type { AlertRule, AlertRecord, PageData } from '@/types'

export const alertApi = {
  listRules: () => request.get<never, AlertRule[]>('/alerts/rules'),

  createRule: (data: Omit<AlertRule, 'id' | 'createdBy' | 'createdAt'>) =>
    request.post<never, AlertRule>('/alerts/rules', data),

  deleteRule: (id: number) => request.delete(`/alerts/rules/${id}`),

  listRecords: (params?: { page?: number; pageSize?: number; startAt?: string }) =>
    request.get<never, PageData<AlertRecord>>('/alerts/records', { params }),
}

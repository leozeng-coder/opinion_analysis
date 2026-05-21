import request from './request'
import type { AlertRule, AlertRecord, PageData, AlertRulePayload } from '@/types'

export const alertApi = {
  listRules: () => request.get<never, AlertRule[]>('/alerts/rules'),

  createRule: (data: AlertRulePayload) =>
    request.post<never, AlertRule>('/alerts/rules', data),

  updateRule: (id: number, data: AlertRulePayload) =>
    request.put<never, AlertRule>(`/alerts/rules/${id}`, data),

  deleteRule: (id: number) => request.delete(`/alerts/rules/${id}`),

  listRecords: (params?: { page?: number; pageSize?: number; startAt?: string }) =>
    request.get<never, PageData<AlertRecord>>('/alerts/records', { params }),

  evaluate: (sync = true) =>
    request.post<never, { evaluated: number; triggered: number; skipped: number; errors?: string[]; source: string } | { message: string }>(
      '/alerts/evaluate',
      {},
      { params: sync ? { sync: '1' } : undefined },
    ),
}

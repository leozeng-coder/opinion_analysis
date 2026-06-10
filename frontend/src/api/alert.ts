import request from './request'
import type { AlertRule, AlertRecord, PageData, AlertRulePayload } from '@/types'

export const alertApi = {
  listRules: () => request.get<AlertRule[]>('/alerts/rules'),

  createRule: (data: AlertRulePayload) =>
    request.post<AlertRule>('/alerts/rules', data),

  updateRule: (id: number, data: AlertRulePayload) =>
    request.put<AlertRule>(`/alerts/rules/${id}`, data),

  deleteRule: (id: number) => request.delete(`/alerts/rules/${id}`),

  listRecords: (params?: { page?: number; pageSize?: number; startAt?: string }) =>
    request.get<PageData<AlertRecord>>('/alerts/records', { params }),

  getRecordDetail: (id: number) =>
    request.get<AlertRecord>(`/alerts/records/${id}`),

  markAsRead: (id: number) =>
    request.patch(`/alerts/records/${id}/read`),

  evaluate: (sync = true) =>
    request.post<AlertEvaluateResult | { message: string }>(
      '/alerts/evaluate',
      {},
      { params: sync ? { sync: '1' } : undefined },
    ),
}

export interface AlertRuleResult {
  ruleId: number
  ruleName: string
  triggered: boolean
  skipReason?: string
  matchCount?: number
  threshold?: number
  windowStart?: string
}

export interface AlertEvaluateResult {
  evaluated: number
  triggered: number
  skipped: number
  errors?: string[]
  source: string
  details?: AlertRuleResult[]
}

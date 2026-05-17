import request from './request'
import type { AuditLog, PageResult } from '@/types'

export const adminAuditApi = {
  list: (params?: {
    page?: number
    pageSize?: number
    actorName?: string
    action?: string
    resource?: string
    startAt?: string
    endAt?: string
  }) => request.get<never, PageResult<AuditLog>>('/admin/audit-logs', { params }),
}

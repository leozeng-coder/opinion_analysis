import request from './request'
import type { PageResult, RagStatus, RagSyncLog } from '@/types'

export const adminRagApi = {
  status: () => request.get<never, RagStatus>('/admin/rag/status'),
  runs: (params: { page?: number; pageSize?: number }) =>
    request.get<never, PageResult<RagSyncLog>>('/admin/rag/runs', { params }),
  triggerSync: () =>
    request.post<never, { syncLogId: number; message: string; raw?: unknown }>(
      '/admin/rag/sync',
      {},
    ),
}

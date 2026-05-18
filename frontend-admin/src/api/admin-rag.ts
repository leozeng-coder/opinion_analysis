import request from './request'
import type { PageResult, RagKBArticle, RagStatus, RagSyncLog } from '@/types'

export const adminRagApi = {
  status: () => request.get<never, RagStatus>('/admin/rag/status'),
  runs: (params: { page?: number; pageSize?: number }) =>
    request.get<never, PageResult<RagSyncLog>>('/admin/rag/runs', { params }),
  triggerSync: () =>
    request.post<never, { syncLogId: number; message: string; raw?: unknown }>(
      '/admin/rag/sync',
      {},
    ),
  getConfig: () =>
    request.get<never, { sync_enabled: boolean }>('/admin/rag/config'),
  updateConfig: (payload: { sync_enabled: boolean }) =>
    request.put<never, { ok: boolean; sync_enabled: boolean }>('/admin/rag/config', payload),
  listArticles: (params: {
    page?: number
    page_size?: number
    keyword?: string
    platform?: string
    synced?: 'yes' | 'no' | ''
  }) => request.get<never, PageResult<RagKBArticle>>('/admin/rag/articles', { params }),
  deleteEmbedding: (id: number) =>
    request.delete<never, { ok: boolean }>(`/admin/rag/articles/${id}/embedding`),
}

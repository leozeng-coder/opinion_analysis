import request from './request'
import type {
  PageResult,
  RagConfig,
  RagKBArticle,
  RagMilvusRebuildResult,
  RagRestartResult,
  RagStatus,
  RagSyncLog,
  UpdateRagConfigPayload,
} from '@/types'

export const adminRagApi = {
  status: () => request.get<never, RagStatus>('/admin/rag/status'),
  runs: (params: { page?: number; pageSize?: number }) =>
    request.get<never, PageResult<RagSyncLog>>('/admin/rag/runs', { params }),
  triggerSync: () =>
    request.post<never, { syncLogId: number; message: string; raw?: unknown }>(
      '/admin/rag/sync',
      {},
    ),
  getConfig: () => request.get<never, RagConfig>('/admin/rag/config'),
  updateConfig: (payload: UpdateRagConfigPayload) =>
    request.put<never, RagConfig & { ok?: boolean }>('/admin/rag/config', payload),
  rebuildMilvus: () =>
    request.post<never, RagMilvusRebuildResult>('/admin/rag/milvus/rebuild', {}),
  restartService: () =>
    request.post<never, RagRestartResult>('/admin/rag/restart', {}, { timeout: 45000 }),
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

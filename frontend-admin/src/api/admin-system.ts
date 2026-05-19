import request from './request'
import type { SystemConfigResponse, SystemHealth, ConfigSnapshot, UpdateTaggerPayload } from '@/types'

export const adminSystemApi = {
  config: () => request.get<never, SystemConfigResponse>('/admin/system/config'),
  health: () => request.get<never, SystemHealth>('/admin/system/health'),
  updateTagger: (payload: UpdateTaggerPayload) =>
    request.put<never, SystemConfigResponse>('/admin/system/tagger', payload),
  settingHistory: (params: { domain: 'rag' | 'tagger'; page?: number; pageSize?: number }) =>
    request.get<never, { list: ConfigSnapshot[]; total: number; page: number }>(
      '/admin/system/settings/history',
      { params },
    ),
  deleteSettingHistory: (id: number) =>
    request.delete<never, { ok: boolean; message: string }>(`/admin/system/settings/history/${id}`),
  reapplySettingHistory: (id: number) =>
    request.post<never, { ok: boolean; message: string; domain: string; warning?: string }>(
      `/admin/system/settings/history/${id}/reapply`,
      {},
    ),
}

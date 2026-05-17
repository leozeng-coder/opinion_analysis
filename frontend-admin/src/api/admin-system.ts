import request from './request'
import type { SystemConfigResponse, SystemHealth, UpdateTaggerPayload } from '@/types'

export const adminSystemApi = {
  config: () => request.get<never, SystemConfigResponse>('/admin/system/config'),
  health: () => request.get<never, SystemHealth>('/admin/system/health'),
  updateTagger: (payload: UpdateTaggerPayload) =>
    request.put<never, SystemConfigResponse>('/admin/system/tagger', payload),
}

import request from './request'
import type { SystemSetting } from '@/types'

export const adminSettingApi = {
  list: () => request.get<never, SystemSetting[]>('/admin/settings'),

  update: (key: string, value: string) =>
    request.put<never, SystemSetting>(`/admin/settings/${key}`, { value }),
}

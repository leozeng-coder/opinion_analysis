import request from './request'
import type { User, PageResult } from '@/types'

export const adminUserApi = {
  list: (params?: { page?: number; pageSize?: number; keyword?: string; role?: string }) =>
    request.get<never, PageResult<User>>('/admin/users', { params }),

  update: (id: number, data: { role?: string; status?: number; nickname?: string; email?: string }) =>
    request.put<never, User>(`/admin/users/${id}`, data),

  resetPassword: (id: number) =>
    request.post<never, { password: string }>(`/admin/users/${id}/reset-password`),

  delete: (id: number) => request.delete(`/admin/users/${id}`),
}

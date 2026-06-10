import request from './request'
import type { User } from '@/types'

export const authApi = {
  login: (data: { username: string; password: string }) =>
    request.post<{ token: string; user: User }>('/auth/login', data),

  register: (data: { username: string; password: string; email: string; nickname?: string }) =>
    request.post<{ id: number }>('/auth/register', data),

  profile: () => request.get<User>('/auth/profile'),
}

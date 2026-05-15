import request from './request'
import type { User } from '@/types'

export const authApi = {
  login: (data: { username: string; password: string }) =>
    request.post<never, { token: string; user: User }>('/auth/login', data),

  register: (data: { username: string; password: string; email: string; nickname?: string }) =>
    request.post<never, { id: number }>('/auth/register', data),

  profile: () => request.get<never, User>('/auth/profile'),
}

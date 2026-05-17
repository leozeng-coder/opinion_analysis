import request from './request'
import type { User } from '@/types'

export const authApi = {
  login: (username: string, password: string) =>
    request.post<never, { token: string; user: User }>('/auth/login', { username, password }),

  profile: () => request.get<never, User>('/auth/profile'),
}

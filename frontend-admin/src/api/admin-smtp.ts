import request from './request'

export interface SmtpConfig {
  host: string
  port: number
  username: string
  from: string
  useTls: boolean
  passwordSet: boolean
  onCrawl: boolean
}

export interface UpdateSmtpPayload {
  host?: string
  port?: number
  username?: string
  password?: string
  from?: string
  useTls?: boolean
  onCrawl?: boolean
}

export const adminSmtpApi = {
  getConfig: () => request.get<never, SmtpConfig>('/admin/system/smtp'),
  updateConfig: (payload: UpdateSmtpPayload) =>
    request.put<never, { message: string }>('/admin/system/smtp', payload),
  test: (to: string) =>
    request.post<never, { message: string }>('/admin/system/smtp/test', { to }),
}

export const alertApi = {
  evaluate: (sync = true) =>
    request.post<never, { evaluated: number; triggered: number; skipped: number } | { message: string }>(
      '/alerts/evaluate',
      {},
      { params: sync ? { sync: '1' } : undefined },
    ),
}

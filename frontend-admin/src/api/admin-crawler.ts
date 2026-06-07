import request from './request'

export interface CrawlerCookieInfo {
  set: boolean
  masked: string
}

export interface CrawlerConfigResponse {
  maxNotesCount: number
  maxConcurrency: number
  sleepSecMin: number
  sleepSecMax: number
  enableComments: boolean
  enableSubComments: boolean
  enableIPProxy: boolean
  ipProxyPoolCount: number
  ipProxyProvider: string
  proxyKdlSecretId: string
  proxyKdlSignature: string
  proxyKdlUsername: string
  proxyKdlPassword: string
  proxyWandouAppKey: string
  xhsSortType: string
  weiboSearchType: string
  dySortType: number
  zhihuSort: string
  zhihuSearchTime: string
  cookies: Record<string, CrawlerCookieInfo>
}

export interface UpdateCrawlerPayload {
  maxNotesCount?: number
  maxConcurrency?: number
  sleepSecMin?: number
  sleepSecMax?: number
  enableComments?: boolean
  enableSubComments?: boolean
  enableIPProxy?: boolean
  ipProxyPoolCount?: number
  ipProxyProvider?: string
  proxyKdlSecretId?: string
  proxyKdlSignature?: string
  proxyKdlUsername?: string
  proxyKdlPassword?: string
  proxyWandouAppKey?: string
  xhsSortType?: string
  weiboSearchType?: string
  dySortType?: number
  zhihuSort?: string
  zhihuSearchTime?: string
  cookies?: Record<string, string>
}

export const adminCrawlerApi = {
  getConfig: () =>
    request.get<never, CrawlerConfigResponse>('/admin/system/crawler'),
  updateConfig: (payload: UpdateCrawlerPayload) =>
    request.put<never, CrawlerConfigResponse>('/admin/system/crawler', payload),
}

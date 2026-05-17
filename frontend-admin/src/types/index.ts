export interface User {
  id: number
  username: string
  email: string
  nickname: string
  role: 'admin' | 'analyst' | 'viewer'
  status: 0 | 1
  createdAt: string
  updatedAt: string
}

export interface SystemSetting {
  key: string
  value: string
  desc: string
  updatedAt: string
  updatedBy: number
}

export interface AuditLog {
  id: number
  actorId: number
  actorName: string
  action: string
  resource: string
  resourceId: string
  method: string
  path: string
  status: number
  payload: string
  ip: string
  userAgent: string
  createdAt: string
}

export interface DataSource {
  id: number
  name: string
  type: string
  url: string
  config: string
  status: 0 | 1
  createdAt: string
  updatedAt: string
}

export interface CrawlerSpiderConfig {
  id: number
  spiderKey: string
  displayName: string
  intervalMinutes: number
  enabled: 0 | 1
  createdAt: string
  updatedAt: string
}

export interface CrawlerRunLog {
  id: number
  spiders: string
  mode: 'basic' | 'advanced'
  params: string
  status: 'running' | 'success' | 'failed'
  message: string
  progress: number
  progressDetail: string
  triggeredBy: number
  startedAt: string
  finishedAt?: string
}

export interface CrawlerRunProgress {
  id: number
  status: 'running' | 'success' | 'failed'
  progress: number
  progressDetail?: string
  detail?: {
    phase?: string
    totalSpiders?: number
    currentSpider?: string
    completedSpiders?: string[]
    itemsInSpider?: number
  }
}

export interface CrawlerRunFilter {
  keywords?: string[]
  topics?: string[]
  startAt?: string
  endAt?: string
}

export interface PageResult<T> {
  total: number
  list: T[]
}

export interface HealthProbe {
  ok: boolean
  message?: string
  latencyMs: number
}

export interface SystemHealth {
  database: HealthProbe
  llm: HealthProbe
  pendingTagging: number
  lastCrawlerRun?: {
    id: number
    spiders: string
    status: string
    startedAt: string
    finishedAt?: string
  }
  timestamp: string
}

export interface TaggerConfig {
  enabled: boolean
  llmModel: string
  llmBaseUrl: string
  llmApiKey: string
  apiKeySet: boolean
  intervalSeconds: number
  batchSize: number
  maxPerTick: number
}

export interface SystemConfigResponse {
  tagger: TaggerConfig
  note?: string
}

export interface UpdateTaggerPayload {
  enabled?: boolean
  llmModel?: string
  llmBaseUrl?: string
  llmApiKey?: string
  intervalSeconds?: number
  batchSize?: number
  maxPerTick?: number
}

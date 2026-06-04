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

export interface RagConfig {
  sync_enabled: boolean
  embed_provider: 'local' | 'api' | string
  embed_model: string
  embed_api_base?: string
  embed_api_key?: string
  api_key_set?: boolean
  chunk_max_chars: number
  chunk_overlap: number
  sync_interval_sec: number
  sync_batch: number
  env_overrides?: string[]
  note?: string
  warnings?: string[]
  warning?: string
  service_applied?: boolean
}

export interface UpdateRagConfigPayload {
  sync_enabled?: boolean
  embed_provider?: 'local' | 'api' | string
  embed_model?: string
  embed_api_base?: string
  embed_api_key?: string
  chunk_max_chars?: number
  chunk_overlap?: number
  sync_interval_sec?: number
  sync_batch?: number
}

export interface RagSnapshotConfig {
  sync_enabled: boolean
  embed_provider: string
  embed_model: string
  embed_api_base: string
  embed_api_key: string
  chunk_max_chars: number
  chunk_overlap: number
  sync_interval_sec: number
  sync_batch: number
}

export interface TaggerSnapshotConfig {
  enabled: boolean
  llm_model: string
  llm_base_url: string
  llm_api_key: string
  interval_seconds: number
  batch_size: number
  max_per_tick: number
}

export interface ConfigSnapshot {
  id: number
  domain: 'rag' | 'tagger'
  config: RagSnapshotConfig | TaggerSnapshotConfig
  updatedBy: number
  updatedByName: string
  createdAt: string
}

/** 管理端：RAG / 句向量服务状态（与对话 LLM 区分） */
export interface RagStatus {
  ragEnabled: boolean
  embeddingServiceUrl: string
  serviceReachable: boolean
  embedModel: string
  embedDim: number
  collectionDim?: number
  dimensionMismatch?: boolean
  embedderReady?: boolean
  embedderError?: string
  processManaged?: boolean
  processRunning?: boolean
  processPid?: number
  milvusUri: string
  collection: string
  note: string
  syncIntervalSecondsHint: number
  syncEnabled?: boolean
  serviceError?: string
  embedProvider?: string
}

export interface RagRestartResult {
  ok: boolean
  pid?: number
  healthReady: boolean
  starting?: boolean
  elapsedMs: number
  message: string
}

export interface RagMilvusRebuildResult {
  ok: boolean
  collection: string
  dropped_previous: boolean
  embed_dimension: number
  collection_dimension: number
  articles_reset_for_resync: number
}

export interface RagSyncLog {
  id: number
  status: 'running' | 'success' | 'failed'
  progress: number
  progressDetail: string
  message: string
  articlesProcessed: number
  chunksUpserted: number
  chunksDeleted: number
  mode: 'scheduled' | 'manual'
  startedAt: string
  finishedAt?: string
}

export interface RagKBArticle {
  id: number
  title: string
  platform: string
  topic: string
  publishedAt?: string
  embeddingHash?: string
  embeddingSyncedAt?: string
  synced: boolean
}

export interface RagKBChunk {
  chunkPk: string
  chunkIdx: number
  snippet: string
  chunkType: 'content' | 'comment' | string
}

export interface RagKBArticleDetail {
  article: {
    id: number
    title: string
    platform: string
    author: string
    originUrl: string
    sentiment: string
    sentScore: number
    aiTags?: string | null
    publishedAt?: string
    embeddingSyncedAt?: string
    synced: boolean
  }
  chunks: RagKBChunk[]
}

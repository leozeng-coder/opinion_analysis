// 通用分页响应
export interface PageData<T> {
  total: number
  list: T[]
}

export interface ApiResponse<T = null> {
  code: number
  message: string
  data: T
}

// 用户
export interface User {
  id: number
  username: string
  email: string
  nickname: string
  role: 'admin' | 'analyst' | 'viewer'
  status: number
  createdAt: string
}

// 数据来源
export interface DataSource {
  id: number
  name: string
  type: string
  url: string
  status: number
}

// 舆情文章
export interface Article {
  id: number
  sourceId: number
  source?: DataSource
  title: string
  content: string
  author: string
  originUrl: string
  platform: string
  sentiment: 'positive' | 'neutral' | 'negative'
  sentScore: number
  keywords: string[]
  publishedAt: string
  createdAt: string
}

// 热点话题
export interface Topic {
  id: number
  name: string
  keywords: string[]
  heatScore: number
  articleCount: number
  trend: 'rising' | 'stable' | 'falling'
  startAt: string
}

// 预警规则
export interface AlertRule {
  id: number
  name: string
  keywords: string
  sentiment: string
  threshold: number
  interval: number
  notifyType: string
  notifyConf: string
  status: number
  createdBy: number
  createdAt: string
}

// 预警记录
export interface AlertRecord {
  id: number
  ruleId: number
  rule?: AlertRule
  title: string
  content: string
  status: 'pending' | 'read'
  createdAt: string
}

// 爬虫调度（与后端 /api/crawler 对齐）
export interface CrawlerSpiderConfig {
  id: number
  spiderKey: string
  displayName: string
  intervalMinutes: number
  enabled: number
  createdAt: string
  updatedAt: string
}

export interface CrawlerRunFilter {
  keywords?: string[]
  topics?: string[]
  startAt?: string
  endAt?: string
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

/** GET /crawler/progress/:id 返回的 data */
export interface CrawlerRunProgress {
  id: number
  status: 'running' | 'success' | 'failed'
  progress: number
  detail: {
    phase?: string
    currentSpider?: string | null
    completedSpiders?: string[]
    totalSpiders?: number
    itemsInSpider?: number
  } | null
  /** 原始 JSON 字符串，detail 解析失败时用于前端兜底 */
  progressDetail?: string
}

export interface CrawlerRunRequest extends CrawlerRunFilter {
  spiders?: string[]
}

// 统计数据
export interface ArticleStats {
  sentiment: { sentiment: string; count: number }[]
  platform: { platform: string; count: number }[]
  trend: { date: string; count: number }[]
}

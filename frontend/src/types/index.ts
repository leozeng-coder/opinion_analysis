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
  aiTags?: string | null   // 后端返回的 JSON 字符串，例如 '["科技创新","人工智能"]'；NULL=未打标
  publishedAt: string
  createdAt: string
}

// AI 标签词频
export interface TagCount {
  tag: string
  count: number
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
  remark?: string
  keywordsAnd?: string
  keywordsOr?: string
  sentiment: string
  threshold: number
  interval: number
  timeRangeDays?: number
  notifyType: string
  notifyConf: string
  status: number
  createdBy: number
  createdAt: string
}

export interface AlertRulePayload {
  name: string
  remark?: string
  keywordListAnd?: string[]
  keywordListOr?: string[]
  sentiment?: string
  threshold?: number
  interval?: number
  timeRangeDays?: number
  notifyType: string
  notifyEmail?: string
  notifyWebhook?: string
  notifyPhone?: string
  status?: number
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

// MediaCrawler 爬虫管理（新版）
export interface MediaCrawlerStartRequest {
  platform: 'xhs' | 'dy' | 'ks' | 'bili' | 'wb' | 'tieba' | 'zhihu'
  login_type: 'qrcode' | 'cookie'
  crawler_type: 'search' | 'detail' | 'creator'
  keywords?: string
  specified_ids?: string
  creator_ids?: string
  save_option: 'db' | 'json' | 'jsonl' | 'csv' | 'sqlite' | 'mongodb' | 'excel'
  enable_comments: boolean
  enable_sub_comments: boolean
  headless: boolean
  start_page: number
  cookies?: string
}

export interface CrawlerStatus {
  status: 'idle' | 'running' | 'stopping' | 'error'
  platform?: string
  crawler_type?: string
  started_at?: string
  error_message?: string
}

export interface CrawlerLog {
  id: number
  timestamp: string
  level: 'info' | 'warning' | 'error' | 'success' | 'debug'
  message: string
}

export interface Platform {
  value: string
  label: string
  icon: string
}

export interface ConfigOption {
  value: string
  label: string
}

export interface ConfigOptions {
  login_types: ConfigOption[]
  crawler_types: ConfigOption[]
  save_options: ConfigOption[]
}

// 统计数据
export interface ArticleStats {
  sentiment: { sentiment: string; count: number }[]
  platform: { platform: string; count: number }[]
  trend: { date: string; count: number }[]
  hotTopicCount?: number
}

// 仪表盘
export interface DashboardKPIMetric {
  count: number
  changePercent?: number
}

export interface DashboardNegativeRatio {
  percent: number
  changePoints?: number
}

export interface DashboardSummary {
  date: string
  text: string
  keywords: string[]
}

export interface SentimentTrendPoint {
  date: string
  positive: number
  neutral: number
  negative: number
  total: number
}

export interface DashboardCrawlerRun {
  id: number
  spiders: string
  status: string
  startedAt: string
  finishedAt?: string
}

export interface DashboardStatus {
  lastCrawlerRun?: DashboardCrawlerRun
  pendingTagging: number
  latestArticleAt?: string
}

export interface DashboardOverview {
  summary?: DashboardSummary
  kpi: {
    todayNew: DashboardKPIMetric
    hotTopics: DashboardKPIMetric
    todayAlerts: DashboardKPIMetric
    negativeRatio: DashboardNegativeRatio
  }
  sentimentTrend: SentimentTrendPoint[]
  hotTags: TagCount[]
  recentAlerts: AlertRecord[]
  recentNegative: Article[]
  platform: { platform: string; count: number }[]
  status: DashboardStatus
}

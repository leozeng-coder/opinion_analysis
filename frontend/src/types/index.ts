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

// 工作流相关类型
export interface WorkflowNode {
  id: string
  type: string
  subType?: string
  label: string
  position: { x: number; y: number }
  config: Record<string, any>
}

export interface WorkflowEdge {
  id: string
  source: string
  target: string
  label?: string
}

export interface Workflow {
  id?: number
  name: string
  description?: string
  status: number
  triggerType: string
  triggerConfig: Record<string, any>
  nodes: WorkflowNode[]
  edges: WorkflowEdge[]
  createdAt?: string
  updatedAt?: string
  createdBy?: number
}

export interface WorkflowExecution {
  id: number
  workflowId: number
  status: 'running' | 'success' | 'failed' | 'partial_success' | 'cancelled'
  startedAt: string
  finishedAt?: string
  errorMsg?: string
}

export interface WorkflowNodeExecution {
  id: number
  executionId: number
  nodeId: string
  status: 'running' | 'success' | 'failed' | 'partial_success' | 'cancelled' | 'inherited'
  startedAt: string
  finishedAt?: string
  input?: Record<string, any>
  output?: Record<string, any>
  errorMsg?: string
}

export interface NodeTypeDefinition {
  type: 'trigger' | 'action' | 'condition'
  subType: string
  label: string
  icon: string
  description: string
  configSchema: {
    name: string
    label: string
    type: 'text' | 'number' | 'select' | 'textarea' | 'cron'
    required?: boolean
    options?: { label: string; value: string }[]
    placeholder?: string
  }[]
}

// 节点类型注册表
export const NODE_TYPES: NodeTypeDefinition[] = [
  // Trigger 节点
  {
    type: 'trigger',
    subType: 'schedule',
    label: '定时触发',
    icon: 'ClockCircleOutlined',
    description: '按照 Cron 表达式定时执行',
    configSchema: [
      { name: 'cron', label: 'Cron 表达式', type: 'text', required: true, placeholder: '0 2 * * *' },
    ],
  },
  {
    type: 'trigger',
    subType: 'manual',
    label: '手动触发',
    icon: 'PlayCircleOutlined',
    description: '手动点击执行',
    configSchema: [],
  },
  {
    type: 'trigger',
    subType: 'webhook',
    label: 'Webhook',
    icon: 'ApiOutlined',
    description: '通过 HTTP 请求触发',
    configSchema: [
      { name: 'path', label: 'Webhook 路径', type: 'text', required: true, placeholder: '/webhook/my-workflow' },
    ],
  },

  // Action 节点
  {
    type: 'action',
    subType: 'ai_tagger',
    label: 'AI 打标',
    icon: 'TagsOutlined',
    description: '对未打标的文章进行 AI 标签生成',
    configSchema: [
      { name: 'batchSize', label: '批次大小', type: 'number', required: true, placeholder: '50' },
    ],
  },
  {
    type: 'action',
    subType: 'rag_vectorize',
    label: 'RAG 向量化',
    icon: 'DatabaseOutlined',
    description: '将文章向量化并存入 Milvus',
    configSchema: [],
  },
  {
    type: 'action',
    subType: 'alert_evaluate',
    label: '告警评估',
    icon: 'BellOutlined',
    description: '评估所有告警规则',
    configSchema: [],
  },
  {
    type: 'action',
    subType: 'digest_generate',
    label: '生成摘要',
    icon: 'FileTextOutlined',
    description: '生成每日舆情摘要',
    configSchema: [
      { name: 'days', label: '天数', type: 'number', required: true, placeholder: '1' },
    ],
  },
  {
    type: 'action',
    subType: 'http_request',
    label: 'HTTP 请求',
    icon: 'ApiOutlined',
    description: '发送 HTTP 请求',
    configSchema: [
      { name: 'url', label: 'URL', type: 'text', required: true, placeholder: 'https://api.example.com' },
      { name: 'method', label: '请求方法', type: 'select', required: true, options: [
        { label: 'GET', value: 'GET' },
        { label: 'POST', value: 'POST' },
        { label: 'PUT', value: 'PUT' },
        { label: 'DELETE', value: 'DELETE' },
      ]},
      { name: 'body', label: '请求体', type: 'textarea', placeholder: 'JSON 格式' },
    ],
  },

  // Condition 节点
  {
    type: 'condition',
    subType: 'condition_if',
    label: '条件判断',
    icon: 'BranchesOutlined',
    description: '根据条件分支执行',
    configSchema: [
      { name: 'condition', label: '条件表达式', type: 'textarea', required: true, placeholder: 'taggedCount > 10' },
    ],
  },
]

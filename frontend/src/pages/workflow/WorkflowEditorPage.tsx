import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useLocation } from 'react-router-dom'
import ReactFlow, {
  Node,
  Controls,
  Background,
  useNodesState,
  useEdgesState,
  addEdge,
  Connection,
  MarkerType,
  Handle,
  Position,
  useStore,
  useReactFlow,
  useNodesInitialized,
  NodeChange,
  NodePositionChange,
} from 'reactflow'
import 'reactflow/dist/style.css'
import { Form, Input, Button, Card, Space, message, Switch, InputNumber, Select, Drawer, Alert, Tag, Table, Modal, Timeline, Spin, Empty, Tooltip, Popconfirm } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  SaveOutlined,
  ArrowLeftOutlined,
  PlusOutlined,
  MinusCircleOutlined,
  PlayCircleOutlined,
  StopOutlined,
  CloseOutlined,
  HistoryOutlined,
  ReloadOutlined,
  SyncOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  EyeOutlined,
  CodeOutlined,
  DownloadOutlined,
  FileTextOutlined,
  UnlockOutlined,
} from '@ant-design/icons'
import PageHeader from '@/components/common/PageHeader'
import { useAuthStore } from '@/store/auth'
import { workflowApi, reportApi } from '@/api/workflow'
import { crawlerApi } from '@/api/crawler'
import { alertApi } from '@/api/alert'
import { topicApi } from '@/api/topic'
import { Workflow, WorkflowExecution, WorkflowNodeExecution, CrawlerLog, Topic } from '@/types'

const { TextArea } = Input

// Inject shimmer animation CSS once at module load
if (typeof document !== 'undefined' && !document.getElementById('wf-node-anim')) {
  const s = document.createElement('style')
  s.id = 'wf-node-anim'
  s.textContent = `
    @keyframes wfShimmer {
      0%   { transform: translateX(-120%) skewX(-12deg); }
      100% { transform: translateX(320%)  skewX(-12deg); }
    }
  `
  document.head.appendChild(s)
}

// Context: maps nodeId → execution status ('running'|'success'|'failed'|'skipped'|'cancelled')
const ExecutionStatusContext = React.createContext<Record<string, string>>({})

// 节点类型注册表
export const NODE_REGISTRY = {
  ai_tagger: {
    label: 'AI 打标',
    description: '自动为文章添加 AI 标签',
    color: '#1890ff',
    icon: '🤖',
    configSchema: [
      { name: 'batchSize', label: '批次大小', type: 'number', required: true, default: 20, min: 1, max: 100 },
      { name: 'onlyProvidedIds', label: '仅处理上游 articleIds', type: 'boolean', required: false, default: true },
    ],
  },
  alert_evaluate: {
    label: '告警评估',
    description: '评估告警规则（可指定规则与时间范围）',
    color: '#ff4d4f',
    icon: '⚠️',
    configSchema: [
      { name: 'ruleIds', label: '评估规则', type: 'alert-rules-select', required: false },
      { name: 'timeRangeDays', label: '查询时间范围(天)', type: 'number', required: false, min: 1, max: 365, placeholder: '留空则用各规则自己配置的时间范围' },
    ],
  },
  rag_vectorize: {
    label: 'RAG 向量化',
    description: '触发 RAG 服务对新文章进行向量化',
    color: '#52c41a',
    icon: '📊',
    configSchema: [
      { name: 'onlyProvidedIds', label: '仅处理上游 articleIds', type: 'boolean', required: false, default: true },
      { name: 'waitForCompletion', label: '等待向量化完成', type: 'boolean', required: false, default: true },
      { name: 'timeoutMinutes', label: '超时时间(分钟)', type: 'number', required: false, default: 5, min: 1, max: 60 },
    ],
  },
  condition: {
    label: '条件判断',
    description: '条件不成立时，下游节点将被跳过',
    color: '#faad14',
    icon: '🔀',
    configSchema: [
      {
        name: 'logic',
        label: '组合方式',
        type: 'select',
        required: false,
        default: 'and',
        options: [
          { label: '全部满足 (AND)', value: 'and' },
          { label: '任一满足 (OR)', value: 'or' },
        ],
      },
      { name: 'conditions', label: '条件规则', type: 'condition-rules', required: false },
    ],
  },
  delay: {
    label: '延迟',
    description: '延迟指定秒数',
    color: '#722ed1',
    icon: '⏱️',
    configSchema: [
      { name: 'seconds', label: '延迟秒数', type: 'number', required: true, default: 60, min: 1, max: 3600 },
    ],
  },
  platform_sync: {
    label: '平台数据同步',
    description: '将 MediaCrawler 平台表同步到 articles 中心表',
    color: '#597ef7',
    icon: '🔄',
    configSchema: [
      {
        name: 'platforms',
        label: '平台',
        type: 'select-multiple',
        required: false,
        options: [
          { label: '小红书', value: 'xiaohongshu' },
          { label: '微博', value: 'weibo' },
          { label: '抖音', value: 'douyin' },
          { label: '快手', value: 'kuaishou' },
          { label: 'B站', value: 'bilibili' },
          { label: '百度贴吧', value: 'tieba' },
          { label: '知乎', value: 'zhihu' },
        ],
        placeholder: '留空则继承上游爬虫节点的平台'
      },
      {
        name: 'syncMode',
        label: '同步模式',
        type: 'select',
        required: false,
        default: 'incremental',
        options: [
          { label: '增量同步', value: 'incremental' },
          { label: '全量同步', value: 'full' },
        ],
      },
      { name: 'syncSinceMinutes', label: '回溯分钟数', type: 'number', required: false, default: 0, min: 0, max: 10080, placeholder: '无上游爬虫时按最近 N 分钟同步，0 表示用系统记录' },
      { name: 'enableSentiment', label: '同步时情感分析', type: 'boolean', required: false, default: false },
    ],
  },
  crawler_run: {
    label: '执行爬虫',
    description: '触发爬虫任务抓取数据（整合全部爬虫调度参数）',
    color: '#13c2c2',
    icon: '🕷️',
    configSchema: [
      {
        name: 'platform',
        label: '平台',
        type: 'select',
        required: false,
        options: [
          { label: '小红书', value: 'xhs' },
          { label: '微博', value: 'wb' },
          { label: '抖音', value: 'dy' },
          { label: '快手', value: 'ks' },
          { label: 'B站', value: 'bili' },
          { label: '百度贴吧', value: 'tieba' },
          { label: '知乎', value: 'zhihu' },
        ],
        placeholder: '留空默认知乎',
      },
      {
        name: 'crawlerType',
        label: '爬取类型',
        type: 'select',
        required: false,
        options: [
          { label: '关键词搜索', value: 'search' },
          { label: '指定内容ID', value: 'detail' },
          { label: '创作者主页', value: 'creator' },
        ],
        placeholder: '留空默认 search',
      },
      { name: 'topics', label: '话题', type: 'tags', required: true, placeholder: '输入话题后按回车添加' },
      { name: 'keywords', label: '关键词', type: 'tags', required: false, placeholder: '按回车添加，多个关键词逐条添加', showIf: { field: 'crawlerType', values: ['search', '', undefined] } },
      { name: 'specifiedIds', label: '指定内容 ID', type: 'text', required: false, placeholder: '多个 ID 用逗号分隔', showIf: { field: 'crawlerType', value: 'detail' } },
      { name: 'creatorIds', label: '创作者 ID', type: 'text', required: false, placeholder: '多个 ID 用逗号分隔', showIf: { field: 'crawlerType', value: 'creator' } },
      {
        name: 'loginType',
        label: '登录方式',
        type: 'select',
        required: false,
        options: [
          { label: 'Cookie 登录', value: 'cookie' },
          { label: '扫码登录', value: 'qrcode' },
        ],
        placeholder: '留空默认 cookie',
      },
      {
        name: 'saveOption',
        label: '存储方式',
        type: 'select',
        required: false,
        options: [
          { label: '数据库', value: 'db' },
          { label: 'JSON', value: 'json' },
          { label: 'JSONL', value: 'jsonl' },
          { label: 'CSV', value: 'csv' },
          { label: 'Excel', value: 'excel' },
        ],
        placeholder: '留空默认 db',
      },
      { name: 'startPage', label: '起始页', type: 'number', required: false, default: 1, min: 1, max: 100 },
      { name: 'maxNotesCount', label: '爬取数量', type: 'number', required: false, min: 1, placeholder: '留空则使用后台管理的默认值' },
      { name: 'maxCommentsCount', label: '一级评论数量', type: 'number', required: false, min: 1, placeholder: '留空则使用后台管理的默认值' },
      { name: 'maxSubCommentsCount', label: '二级评论数量', type: 'number', required: false, min: 1, placeholder: '留空则使用后台管理的默认值' },
      {
        type: 'boolean-pair',
        name: '_bp_crawl1',
        items: [
          { name: 'enableComments', label: '爬取评论', default: true },
          { name: 'enableSubComments', label: '爬取二级评论', default: false },
        ],
      },
      {
        type: 'boolean-pair',
        name: '_bp_crawl2',
        items: [
          { name: 'headless', label: '无头模式', default: true },
          { name: 'waitForCompletion', label: '等待爬取完成', default: true },
        ],
      },
      { name: 'timeoutMinutes', label: '超时时间(分钟)', type: 'number', required: false, default: 60, min: 1, max: 480 },
    ],
  },
  crawler_schedule: {
    label: '配置爬虫调度',
    description: '更新爬虫定时任务配置',
    color: '#2f54eb',
    icon: '⏰',
    configSchema: [
      { name: 'spiderKey', label: '爬虫标识', type: 'text', required: true, placeholder: 'broad-topic 或 deep-sentiment' },
      { name: 'intervalMinutes', label: '执行间隔(分钟)', type: 'number', required: true, default: 60, min: 1, max: 10080 },
      { name: 'enabled', label: '启用', type: 'boolean', required: true },
    ],
  },
  data_patch: {
    label: '补数',
    description: '计算平台源表与中心表的差集，将未同步的数据 ID 传给下游平台同步节点补录',
    color: '#4096ff',
    icon: '🔧',
    configSchema: [
      {
        name: 'platforms',
        label: '平台',
        type: 'select-multiple',
        required: true,
        options: [
          { label: '小红书', value: 'xhs' },
          { label: '微博', value: 'wb' },
          { label: '抖音', value: 'dy' },
          { label: '快手', value: 'ks' },
          { label: 'B站', value: 'bili' },
          { label: '百度贴吧', value: 'tieba' },
          { label: '知乎', value: 'zhihu' },
        ],
        placeholder: '选择需要补数的平台',
      },
      { name: 'topics', label: '话题', type: 'tags', required: false, placeholder: '指定同步话题（可选）' },
    ],
  },
  data_filter: {
    label: '数据过滤',
    description: '对上游文章做正则/AI 过滤（先正则后 AI），仅传递保留项',
    color: '#fa8c16',
    icon: '🧹',
    configSchema: [
      { name: 'enableRegex', label: '启用正则过滤', type: 'boolean', required: false, default: false },
      { name: 'regexKeywords', label: '关键词（正则/包含）', type: 'tags', required: false, placeholder: '回车添加，支持正则；任一命中即匹配' },
      {
        name: 'regexKeywordMode',
        label: '关键词命中处理',
        type: 'select',
        required: false,
        default: 'keep',
        options: [
          { label: '保留命中项', value: 'keep' },
          { label: '剔除命中项', value: 'exclude' },
        ],
      },
      { name: 'minLength', label: '最小字数', type: 'number', required: false, min: 0, max: 100000, placeholder: '留空 = 不限' },
      { name: 'maxLength', label: '最大字数', type: 'number', required: false, min: 0, max: 100000, placeholder: '留空 = 不限' },
      { name: 'enableAI', label: '启用 AI 过滤', type: 'boolean', required: false, default: false },
      { name: 'aiRequirement', label: 'AI 过滤需求', type: 'textarea', required: false, placeholder: '用自然语言描述保留条件，例如：只保留与「新能源汽车」行业舆情相关、且为负面情绪的内容' },
      { name: 'deleteFiltered', label: '从数据库移除被过滤的文章', type: 'boolean', required: false, default: true },
    ],
  },
  crawler_status: {
    label: '检查爬虫状态',
    description: '查询爬虫运行状态',
    color: '#eb2f96',
    icon: '📈',
    configSchema: [
      { name: 'runID', label: '运行ID', type: 'number', required: false, placeholder: '留空则检查最近运行' },
      { name: 'checkRecent', label: '检查最近运行', type: 'boolean', required: false },
    ],
  },
  digest_generate: {
    label: '生成摘要',
    description: '触发每日舆情 AI 分析摘要生成',
    color: '#9254de',
    icon: '📝',
    configSchema: [
      { name: 'days', label: '统计天数', type: 'number', required: false, default: 1, min: 1, max: 30 },
    ],
  },
  analysis_report: {
    label: 'AI 分析报告',
    description: '基于本次爬取的文章生成 AI 分析报告（Markdown 或 HTML），存入 Redis 7天，可下载',
    color: '#eb2f96',
    icon: '📊',
    configSchema: [
      {
        name: 'format',
        label: '报告格式',
        type: 'select',
        required: false,
        default: 'markdown',
        options: [
          { label: 'Markdown（分析叙述为主）', value: 'markdown' },
          { label: 'HTML（可视化图表为主）', value: 'html' },
        ],
      },
      {
        name: 'htmlTheme',
        label: 'HTML 风格',
        type: 'select',
        required: false,
        default: 'random',
        showIf: { field: 'format', value: 'html' },
        options: [
          { label: '🎲 随机（每次不同）', value: 'random' },
          { label: '🌊 深海蓝', value: '深海蓝' },
          { label: '🔥 暮光橙', value: '暮光橙' },
          { label: '🌿 翡翠绿', value: '翡翠绿' },
          { label: '🔮 紫夜', value: '紫夜' },
          { label: '🖋️ 青墨', value: '青墨' },
          { label: '🌸 玫瑰金', value: '玫瑰金' },
          { label: '🌌 星辰灰', value: '星辰灰' },
          { label: '✨ 琥珀金', value: '琥珀金' },
        ],
      },
      { type: 'group-label', label: '文章分析', name: '_grp_article' },
      {
        type: 'number-pair', name: '_pair_article',
        items: [
          { name: 'maxGroups', label: '话题组数', default: 5, min: 1, max: 10, placeholder: '按标签频次取前N组' },
          { name: 'sampleSize', label: '每组样本数', default: 8, min: 3, max: 20, placeholder: '每话题组送入LLM的代表文章数' },
        ],
      },
      { type: 'group-label', label: '评论分析', name: '_grp_comment' },
      {
        type: 'number-pair', name: '_pair_comment',
        items: [
          { name: 'maxTopicCards', label: '话题卡片数', default: 8, min: 1, max: 20, placeholder: '最多展示几个话题卡片' },
          { name: 'commentSampleSize', label: '每题分析条数', default: 18, min: 5, max: 50, placeholder: '每话题送入LLM的评论数' },
        ],
      },
    ],
  },
  http_request: {
    label: 'HTTP 请求',
    description: '向外部地址发送任意 HTTP 请求（webhook/回调）',
    color: '#08979c',
    icon: '🌐',
    configSchema: [
      { name: 'url', label: '请求地址', type: 'text', required: true, placeholder: 'https://example.com/webhook' },
      {
        name: 'method',
        label: '请求方法',
        type: 'select',
        required: true,
        default: 'POST',
        options: [
          { label: 'GET', value: 'GET' },
          { label: 'POST', value: 'POST' },
          { label: 'PUT', value: 'PUT' },
          { label: 'DELETE', value: 'DELETE' },
          { label: 'PATCH', value: 'PATCH' },
        ],
      },
      { name: 'body', label: '请求体', type: 'text', required: false, placeholder: 'JSON 字符串，可留空' },
      { name: 'timeoutSeconds', label: '超时时间(秒)', type: 'number', required: false, default: 30, min: 1, max: 300 },
    ],
  },
}

// 工作流图结构校验，返回警告信息列表（不拦截保存）
function validateWorkflowGraph(
  nodes: Node[],
  edges: { source: string; target: string }[]
): string[] {
  const warnings: string[] = []
  if (nodes.length === 0) return warnings

  const inDegree: Record<string, number> = {}
  const outDegree: Record<string, number> = {}
  nodes.forEach((n) => { inDegree[n.id] = 0; outDegree[n.id] = 0 })
  edges.forEach((e) => { outDegree[e.source] = (outDegree[e.source] || 0) + 1; inDegree[e.target] = (inDegree[e.target] || 0) + 1 })

  const sourceNodes = nodes.filter((n) => NODE_CATEGORY[n.data?.type] === 'source')
  const syncNodes   = nodes.filter((n) => NODE_CATEGORY[n.data?.type] === 'sync')

  // 1. 没有起始节点（crawler_run）
  if (sourceNodes.length === 0) {
    warnings.push('工作流没有起始节点（执行爬虫），数据来源不明确')
  }

  // 2. 起始节点有入边（不应该有上游）
  sourceNodes.forEach((n) => {
    if (inDegree[n.id] > 0) {
      warnings.push(`起始节点「${NODE_REGISTRY[n.data.type as keyof typeof NODE_REGISTRY]?.label || n.data.type}」有入边，起始节点通常不应有上游`)
    }
  })

  // 3. 数据同步节点没有上游爬虫节点
  syncNodes.forEach((n) => {
    const hasSourceUpstream = edges.some((e) => {
      if (e.target !== n.id) return false
      const upNode = nodes.find((nd) => nd.id === e.source)
      return upNode && NODE_CATEGORY[upNode.data?.type] === 'source'
    })
    if (!hasSourceUpstream && inDegree[n.id] === 0) {
      warnings.push(`数据同步节点「${NODE_REGISTRY[n.data.type as keyof typeof NODE_REGISTRY]?.label || n.data.type}」没有连接上游节点，可能无法获取数据`)
    }
  })

  // 4. 孤立节点（无入边且无出边，且不是起始节点）
  nodes.forEach((n) => {
    if (inDegree[n.id] === 0 && outDegree[n.id] === 0 && NODE_CATEGORY[n.data?.type] !== 'source') {
      warnings.push(`节点「${NODE_REGISTRY[n.data.type as keyof typeof NODE_REGISTRY]?.label || n.data.type}」未连接任何节点`)
    }
  })

  // 5. condition 节点没有出边
  nodes.filter((n) => n.data?.type === 'condition').forEach((n) => {
    if (outDegree[n.id] === 0) {
      warnings.push('条件判断节点没有下游节点，判断结果无法生效')
    }
  })

  // 6. 多个起始节点
  if (sourceNodes.length > 1) {
    warnings.push(`存在 ${sourceNodes.length} 个起始节点，工作流将依次执行所有爬虫，请确认是否符合预期`)
  }

  return warnings
}


function parseCronToFields(cron: string): Record<string, any> | null {
  const p = cron.trim().split(/\s+/)
  if (p.length !== 5) return null
  const [min, hour, dom, , dow] = p

  if (min.startsWith('*/') && hour === '*' && dom === '*')
    return { tc_scheduleMode: 'interval', tc_intervalUnit: 'minutes', tc_intervalValue: +min.slice(2) }
  if (min === '0' && hour.startsWith('*/') && dom === '*')
    return { tc_scheduleMode: 'interval', tc_intervalUnit: 'hours', tc_intervalValue: +hour.slice(2) }
  if (min === '0' && hour === '0' && dom.startsWith('*/'))
    return { tc_scheduleMode: 'interval', tc_intervalUnit: 'days', tc_intervalValue: +dom.slice(2) }

  const m = parseInt(min, 10), h = parseInt(hour, 10)
  if (isNaN(m) || isNaN(h)) return null
  if (dom === '*' && dow !== '*' && !dow.startsWith('*/'))
    return { tc_scheduleMode: 'fixed', tc_fixedFreq: 'weekly',   tc_fixedMinute: m, tc_fixedHour: h, tc_fixedWeekday:  parseInt(dow, 10) }
  if (dom !== '*' && !dom.startsWith('*/') && dow === '*')
    return { tc_scheduleMode: 'fixed', tc_fixedFreq: 'monthly',  tc_fixedMinute: m, tc_fixedHour: h, tc_fixedMonthDay: parseInt(dom, 10) }
  if (dom === '*' && dow === '*')
    return { tc_scheduleMode: 'fixed', tc_fixedFreq: 'daily',    tc_fixedMinute: m, tc_fixedHour: h }
  return null
}

// 从表单字段构造 cron 字符串
function buildCronFromValues(v: Record<string, any>): string | null {
  const m = v.tc_fixedMinute ?? 0, h = v.tc_fixedHour ?? 0
  if (v.tc_scheduleMode === 'fixed') {
    if (v.tc_fixedFreq === 'daily')   return `${m} ${h} * * *`
    if (v.tc_fixedFreq === 'weekly')  return `${m} ${h} * * ${v.tc_fixedWeekday ?? 1}`
    if (v.tc_fixedFreq === 'monthly') return `${m} ${h} ${v.tc_fixedMonthDay ?? 1} * *`
  }
  if (v.tc_scheduleMode === 'interval') {
    const n = v.tc_intervalValue
    if (!n || n <= 0) return null
    if (v.tc_intervalUnit === 'minutes') return `*/${n} * * * *`
    if (v.tc_intervalUnit === 'hours')   return `0 */${n} * * *`
    if (v.tc_intervalUnit === 'days')    return `0 0 */${n} * *`
  }
  return null
}

// 条件节点：比较运算符选项
const CONDITION_OPERATORS = [
  { label: '>', value: '>' },
  { label: '≥', value: '>=' },
  { label: '<', value: '<' },
  { label: '≤', value: '<=' },
  { label: '=', value: '==' },
  { label: '≠', value: '!=' },
]

// 条件节点：可选上游字段（仅展示中文，value 为后端实际字段名）
// 均为数值类计数字段，因此比较运算符 > >= < <= == != 都适用。
const CONDITION_FIELD_SUGGESTIONS = [
  { value: 'articlesCount', label: '同步文章数' },
  { value: 'syncNewCount', label: '新增文章数' },
  { value: 'taggedCount', label: '打标文章数' },
  { value: 'alertCount', label: '触发告警数' },
  { value: 'ragArticlesDone', label: '向量化文章数' },
]

// 节点分类
const NODE_CATEGORY: Record<string, 'source' | 'sync' | 'process' | 'ops'> = {
  crawler_run:      'source',
  data_patch:       'source',
  platform_sync:    'sync',
  data_filter:      'process',
  ai_tagger:        'process',
  rag_vectorize:    'process',
  alert_evaluate:   'process',
  digest_generate:  'process',
  condition:        'process',
  delay:            'process',
  crawler_schedule: 'ops',
  crawler_status:   'ops',
  http_request:     'ops',
  analysis_report:  'process',
}

// 节点形状渲染：source=圆形，sync=菱形，process=圆角矩形，ops=六边形
const NodeShape = ({ category, color, size, icon, hovered, hasTarget, hasSource }: {
  category: 'source' | 'sync' | 'process' | 'ops'
  color: string
  size: number
  icon: string
  hovered: boolean
  hasTarget: boolean
  hasSource: boolean
}) => {
  const handleTarget = (
    <Handle
      type="target"
      position={Position.Left}
      style={{
        width: 5, height: 18, borderRadius: 2,
        background: '#9e9e9e', border: 'none',
        opacity: hasTarget || hovered ? 1 : 0,
        transition: 'opacity .15s',
      }}
    />
  )
  const handleSource = (
    <Handle
      type="source"
      position={Position.Right}
      style={{
        width: 12, height: 12,
        background: color, border: '2px solid #fff',
        boxShadow: `0 0 0 1px ${color}`,
        opacity: hasSource || hovered ? 1 : 0,
        transition: 'opacity .15s',
      }}
    />
  )

  const commonInner = (extraStyle: React.CSSProperties) => (
    <div style={{
      width: size, height: size,
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      fontSize: 28, position: 'relative',
      ...extraStyle,
    }}>
      {handleTarget}
      <span>{icon}</span>
      {handleSource}
    </div>
  )

  if (category === 'source') {
    // 左侧大圆角、右侧直角，参考 n8n trigger 节点风格
    return (
      <div style={{
        width: size, height: size, position: 'relative',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        borderRadius: '22px 4px 4px 22px',
        background: '#1f2937',
        boxShadow: '0 4px 12px rgba(0,0,0,0.25)',
        fontSize: 30,
      }}>
        {handleTarget}
        <span style={{ filter: `drop-shadow(0 0 4px ${color})`, color }}>{icon}</span>
        {handleSource}
      </div>
    )
  }

  if (category === 'sync') {
    // 菱形：用旋转的正方形实现，外层 div 撑开空间，内层旋转
    const d = Math.round(size * 0.72)
    return (
      <div style={{ width: size, height: size, position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        {handleTarget}
        <div style={{
          width: d, height: d,
          transform: 'rotate(45deg)',
          background: '#fff',
          border: `2px solid ${color}`,
          boxShadow: '0 2px 8px rgba(0,0,0,0.08)',
        }} />
        <span style={{ position: 'absolute', fontSize: 24, pointerEvents: 'none' }}>{icon}</span>
        {handleSource}
      </div>
    )
  }

  if (category === 'ops') {
    // 圆形：彩色描边 + 白色背景
    return commonInner({
      borderRadius: '50%',
      background: '#fff',
      border: `2.5px solid ${color}`,
      boxShadow: '0 2px 8px rgba(0,0,0,0.08)',
    })
  }

  // process：圆角矩形（原有样式）
  return commonInner({
    borderRadius: 16,
    background: '#fff',
    border: `1.5px solid ${color}`,
    boxShadow: '0 2px 8px rgba(0,0,0,0.06)',
  })
}

// 自定义节点组件
const CustomNode = ({ id, data }: any) => {
  const nodeType = NODE_REGISTRY[data.type as keyof typeof NODE_REGISTRY]
  const color = nodeType?.color || '#8c8c8c'
  const category = NODE_CATEGORY[data.type] || 'process'
  const subtitle = data.label && !String(data.label).startsWith('node_') ? data.label : ''

  const edges = useStore((s) => s.edges)
  const [hovered, setHovered] = useState(false)
  const hasTarget = edges.some((e) => e.target === id)
  const hasSource = edges.some((e) => e.source === id)

  const statusMap = React.useContext(ExecutionStatusContext)
  const execStatus = statusMap[id]
  const isRunning  = execStatus === 'running'
  const isSuccess  = execStatus === 'success' || execStatus === 'partial_success'
  const isFailed   = execStatus === 'failed'
  const isDimmed   = execStatus === 'skipped' || execStatus === 'cancelled' || execStatus === 'inherited'

  // Clip shape for shimmer overlay matches each category's visual boundary
  const shimmerClip: React.CSSProperties =
    category === 'source' ? { borderRadius: '22px 4px 4px 22px' } :
    category === 'ops'    ? { borderRadius: '50%' } :
    category === 'sync'   ? { clipPath: 'polygon(50% 0%, 100% 50%, 50% 100%, 0% 50%)' } :
                            { borderRadius: 16 }

  return (
    <div
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{ position: 'relative', cursor: 'pointer', opacity: isDimmed ? 0.4 : 1, transition: 'opacity .3s' }}
    >
      <NodeShape
        category={category}
        color={color}
        size={72}
        icon={nodeType?.icon || ''}
        hovered={hovered}
        hasTarget={hasTarget}
        hasSource={hasSource}
      />

      {/* 流光效果：执行中节点，节点主色光带从左向右扫过（数据流方向） */}
      {isRunning && (
        <div style={{
          position: 'absolute', top: 0, left: 0, width: 72, height: 72,
          overflow: 'hidden', pointerEvents: 'none', zIndex: 5,
          ...shimmerClip,
        }}>
          <div style={{
            position: 'absolute', top: 0, left: 0,
            width: '50%', height: '100%',
            background: `linear-gradient(90deg, transparent 0%, ${color}66 50%, transparent 100%)`,
            animation: 'wfShimmer 1.4s ease-in-out infinite',
          }} />
        </div>
      )}

      {/* 状态徽章：成功 / 失败 */}
      {(isSuccess || isFailed) && (
        <div style={{
          position: 'absolute', top: -5, right: -5,
          width: 18, height: 18, borderRadius: '50%',
          background: isSuccess ? '#52c41a' : '#ff4d4f',
          border: '2px solid #fff',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: 10, color: '#fff', fontWeight: 700,
          zIndex: 10, boxShadow: '0 1px 4px rgba(0,0,0,0.2)',
          pointerEvents: 'none',
        }}>
          {isSuccess ? '✓' : '✗'}
        </div>
      )}

      <div
        style={{
          position: 'absolute',
          top: '100%',
          left: '50%',
          transform: 'translateX(-50%)',
          marginTop: 6,
          width: 120,
          textAlign: 'center',
          pointerEvents: 'none',
        }}
      >
        <div style={{ fontSize: 12, fontWeight: 600, color: '#262626', lineHeight: 1.3 }}>
          {nodeType?.label || data.type}
        </div>
        {subtitle && (
          <div style={{ fontSize: 11, color: '#8c8c8c', lineHeight: 1.3, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
            {subtitle}
          </div>
        )}
      </div>
    </div>
  )
}

const nodeTypes = {
  custom: CustomNode,
}

// 连线样式：更明显的深色加粗连线 + 更大更黑的箭头
const EDGE_MARKER = { type: MarkerType.ArrowClosed, width: 18, height: 18, color: '#333333' }
const EDGE_STYLE = { stroke: '#595959', strokeWidth: 2 }

// 对齐吸附：计算被拖拽节点与其它节点的对齐线，并给出吸附后的坐标。
// 比较项：左/右/水平中心（竖直对齐线）、上/下/垂直中心（水平对齐线）。
type HelperLinesResult = {
  horizontal?: number
  vertical?: number
  snapX?: number
  snapY?: number
}

const SNAP_DISTANCE = 6 // 进入该像素阈值内即吸附

function getHelperLines(change: NodePositionChange, nodes: Node[]): HelperLinesResult {
  const result: HelperLinesResult = {}
  const nodeA = nodes.find((n) => n.id === change.id)
  if (!nodeA || !change.position) return result

  const aw = nodeA.width ?? 0
  const ah = nodeA.height ?? 0
  const a = {
    left: change.position.x,
    right: change.position.x + aw,
    top: change.position.y,
    bottom: change.position.y + ah,
    cx: change.position.x + aw / 2,
    cy: change.position.y + ah / 2,
  }

  let vDist = SNAP_DISTANCE
  let hDist = SNAP_DISTANCE

  for (const nodeB of nodes) {
    if (nodeB.id === nodeA.id) continue
    const bw = nodeB.width ?? 0
    const bh = nodeB.height ?? 0
    const b = {
      left: nodeB.position.x,
      right: nodeB.position.x + bw,
      top: nodeB.position.y,
      bottom: nodeB.position.y + bh,
      cx: nodeB.position.x + bw / 2,
      cy: nodeB.position.y + bh / 2,
    }

    // 竖直对齐线（对齐 x）
    const vChecks: { d: number; snap: number; line: number }[] = [
      { d: Math.abs(a.left - b.left), snap: b.left, line: b.left },
      { d: Math.abs(a.right - b.right), snap: b.right - aw, line: b.right },
      { d: Math.abs(a.left - b.right), snap: b.right, line: b.right },
      { d: Math.abs(a.right - b.left), snap: b.left - aw, line: b.left },
      { d: Math.abs(a.cx - b.cx), snap: b.cx - aw / 2, line: b.cx },
    ]
    for (const c of vChecks) {
      if (c.d < vDist) {
        vDist = c.d
        result.snapX = c.snap
        result.vertical = c.line
      }
    }

    // 水平对齐线（对齐 y）
    const hChecks: { d: number; snap: number; line: number }[] = [
      { d: Math.abs(a.top - b.top), snap: b.top, line: b.top },
      { d: Math.abs(a.bottom - b.bottom), snap: b.bottom - ah, line: b.bottom },
      { d: Math.abs(a.top - b.bottom), snap: b.bottom, line: b.bottom },
      { d: Math.abs(a.bottom - b.top), snap: b.top - ah, line: b.top },
      { d: Math.abs(a.cy - b.cy), snap: b.cy - ah / 2, line: b.cy },
    ]
    for (const c of hChecks) {
      if (c.d < hDist) {
        hDist = c.d
        result.snapY = c.snap
        result.horizontal = c.line
      }
    }
  }

  return result
}

// 自动适配视口：在节点尺寸测量完成（useNodesInitialized）后立即 fitView，
// 保证一进入就把所有节点完整展示（含缩小过长流程），且不会出现"先错位再跳动"。
// trigger 变化时（重新加载工作流）允许再适配一次。
const FitOnLoad: React.FC<{ trigger: number }> = ({ trigger }) => {
  const initialized = useNodesInitialized()
  const { fitView } = useReactFlow()
  const fittedFor = useRef<number>(-1)

  useEffect(() => {
    if (initialized && fittedFor.current !== trigger) {
      fitView({ padding: 0.15 })
      fittedFor.current = trigger
    }
  }, [initialized, trigger, fitView])

  return null
}

// 对齐辅助线渲染：在画布上叠加一层 canvas 画出吸附参考线（不拦截鼠标）。
const HelperLines: React.FC<{ horizontal?: number; vertical?: number }> = ({ horizontal, vertical }) => {
  const width = useStore((s) => s.width)
  const height = useStore((s) => s.height)
  const tx = useStore((s) => s.transform[0])
  const ty = useStore((s) => s.transform[1])
  const zoom = useStore((s) => s.transform[2])
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return

    const dpi = window.devicePixelRatio || 1
    canvas.width = width * dpi
    canvas.height = height * dpi
    ctx.scale(dpi, dpi)
    ctx.clearRect(0, 0, width, height)
    ctx.strokeStyle = '#1677ff'
    ctx.lineWidth = 1
    ctx.setLineDash([4, 3])

    if (typeof vertical === 'number') {
      const x = vertical * zoom + tx
      ctx.beginPath()
      ctx.moveTo(x, 0)
      ctx.lineTo(x, height)
      ctx.stroke()
    }
    if (typeof horizontal === 'number') {
      const y = horizontal * zoom + ty
      ctx.beginPath()
      ctx.moveTo(0, y)
      ctx.lineTo(width, y)
      ctx.stroke()
    }
  }, [width, height, tx, ty, zoom, horizontal, vertical])

  return (
    <canvas
      ref={canvasRef}
      style={{ position: 'absolute', top: 0, left: 0, width, height, pointerEvents: 'none', zIndex: 10 }}
    />
  )
}

// ============ 执行 / 控制台相关辅助 ============

type ConsoleLevel = 'info' | 'success' | 'warning' | 'error' | 'debug'

interface ConsoleLine {
  key: string
  time?: string
  level: ConsoleLevel
  tag?: string // 工作流节点标签；爬虫实时日志不带 tag，保持与爬虫调度页一致
  message: string
}

const consoleLevelColor = (level: string): string => {
  switch (level) {
    case 'error':
      return 'red'
    case 'warning':
      return 'orange'
    case 'success':
      return 'green'
    case 'debug':
      return 'default'
    default:
      return 'blue'
  }
}

const EXEC_STATUS_CONFIG: Record<string, { color: string; icon: React.ReactNode; label: string }> = {
  running: { color: 'processing', icon: <SyncOutlined spin />, label: '运行中' },
  success: { color: 'success', icon: <CheckCircleOutlined />, label: '成功' },
  partial_success: { color: 'warning', icon: <ExclamationCircleOutlined />, label: '部分成功' },
  failed: { color: 'error', icon: <CloseCircleOutlined />, label: '失败' },
  cancelled: { color: 'default', icon: <StopOutlined />, label: '已取消' },
  inherited: { color: 'default', icon: <HistoryOutlined />, label: '继承' },
}

const getExecStatusConfig = (status: string) =>
  EXEC_STATUS_CONFIG[status] || { color: 'default', icon: null, label: status }

const fmtTime = (t?: string) => (t ? t.replace('T', ' ').slice(11, 19) : '')

// 基于 nodes / edges 计算线性执行计划（Kahn 拓扑排序，适配链式工作流）
const computePlan = (nodes: Node[], edges: { source: string; target: string }[]): Node[] => {
  if (nodes.length === 0) return []
  const indegree = new Map<string, number>()
  const adj = new Map<string, string[]>()
  nodes.forEach((n) => indegree.set(n.id, 0))
  edges.forEach((e) => {
    if (!indegree.has(e.source) || !indegree.has(e.target)) return
    adj.set(e.source, [...(adj.get(e.source) || []), e.target])
    indegree.set(e.target, (indegree.get(e.target) || 0) + 1)
  })
  const queue = nodes.filter((n) => (indegree.get(n.id) || 0) === 0).map((n) => n.id)
  const ordered: string[] = []
  const seen = new Set<string>()
  while (queue.length) {
    const cur = queue.shift()!
    if (seen.has(cur)) continue
    seen.add(cur)
    ordered.push(cur)
    for (const next of adj.get(cur) || []) {
      indegree.set(next, (indegree.get(next) || 0) - 1)
      if ((indegree.get(next) || 0) <= 0) queue.push(next)
    }
  }
  // 未被遍历到的节点（孤立 / 成环）兜底追加
  nodes.forEach((n) => {
    if (!seen.has(n.id)) ordered.push(n.id)
  })
  const map = new Map(nodes.map((n) => [n.id, n]))
  return ordered.map((nid) => map.get(nid)!).filter(Boolean)
}

const nodeDisplayLabel = (data: any): string => {
  if (data?.label && !String(data.label).startsWith('node_')) return data.label
  return NODE_REGISTRY[data?.type as keyof typeof NODE_REGISTRY]?.label || data?.type || '节点'
}

// 根据节点类型与输出，生成「处理了多少、怎么处理」的中文结果摘要
const summarizeNodeOutput = (type: string | undefined, output?: Record<string, any>): string => {
  if (!output) return ''
  const num = (v: any) => (typeof v === 'number' ? v : Number(v) || 0)
  const arr = (v: any) => (Array.isArray(v) ? v : [])

  switch (type) {
    case 'crawler_run': {
      const parts: string[] = []
      if (output.platform) parts.push(`平台：${output.platform}`)
      const kw = arr(output.keywords)
      if (kw.length) parts.push(`关键词：${kw.join('、')}`)
      if (output.waitedCompletion === false) parts.push('已触发(未等待)')
      return parts.join('，')
    }
    case 'platform_sync':
      return `同步新增 ${num(output.syncNewCount)} 条，本次文章 ${num(output.articlesCount)} 篇`
    case 'data_filter': {
      const parts = [`处理数据 ${num(output.filterInputCount)} 条`, `保留 ${num(output.filterOutputCount)} 条`]
      if (num(output.regexRemovedCount) > 0) parts.push(`正则过滤 ${num(output.regexRemovedCount)} 条`)
      if (num(output.aiRemovedCount) > 0) parts.push(`AI 过滤 ${num(output.aiRemovedCount)} 条`)
      if (num(output.deletedCount) > 0) parts.push(`已移除 ${num(output.deletedCount)} 条`)
      return parts.join('，')
    }
    case 'ai_tagger':
      return `打标文章 ${num(output.taggedCount)} 篇`
    case 'rag_vectorize':
      if (output.ragStatus === 'skipped') return `已跳过（${output.ragMessage || '无需向量化'}）`
      return `向量化文章 ${num(output.ragArticlesDone)} 篇，写入块 ${num(output.ragChunksUpserted)}，删除块 ${num(output.ragChunksDeleted)}`
    case 'alert_evaluate':
      return `评估规则 ${num(output.evaluated)} 条，触发告警 ${num(output.alertCount)} 条`
    case 'digest_generate':
      if (output.digestGenerated === false) return `未生成（${output.digestMessage || ''}）`
      return `生成摘要 ${output.digestStartDate || ''}${output.digestEndDate ? ' ~ ' + output.digestEndDate : ''}`
    case 'condition':
      return `条件${output.conditionResult ? '成立，继续向下' : '不成立，下游跳过'}`
    case 'crawler_schedule':
      return `爬虫 ${output.spiderKey || ''}，间隔 ${num(output.intervalMinutes)} 分钟，${output.enabled ? '已启用' : '已停用'}`
    case 'crawler_status':
      return output.crawlerStatus ? `爬虫状态：${output.crawlerStatus}` : ''
    case 'http_request':
      return `HTTP ${num(output.httpStatusCode)}${output.httpSuccess ? ' 成功' : ' 失败'}`
    case 'analysis_report':
      if (output.reportId) return `报告已生成（${output.reportFormat || 'markdown'}），ID: ${String(output.reportId).slice(0, 8)}...`
      return '报告生成中'
    case 'data_patch':
      return `发现缺失数据 ${(output.missingCount as number) || 0} 条，待补录`
    default:
      return ''
  }
}

// 把工作流节点执行记录 + 爬虫实时日志合并成控制台行
const buildConsoleLines = (params: {
  execution: WorkflowExecution | null
  nodeLogs: WorkflowNodeExecution[]
  nodes: Node[]
  crawlerLogs: CrawlerLog[]
}): ConsoleLine[] => {
  const { execution, nodeLogs, nodes, crawlerLogs } = params
  const lines: ConsoleLine[] = []
  const nodeMap = new Map(nodes.map((n) => [n.id, n.data]))

  if (execution) {
    lines.push({
      key: 'exec-start',
      time: fmtTime(execution.startedAt),
      level: 'info',
      tag: '工作流',
      message: `开始执行工作流 (执行 #${execution.id})`,
    })
  }

  for (const log of nodeLogs) {
    const data = nodeMap.get(log.nodeId)
    const label = data ? nodeDisplayLabel(data) : log.nodeId
    const type = data?.type

    // 继承的前序节点只显示一行简要信息
    if (log.status === 'inherited') {
      lines.push({
        key: `node-${log.id}-inherited`,
        time: fmtTime(log.startedAt),
        level: 'info',
        tag: label,
        message: '(继承上次执行结果)',
      })
      continue
    }

    lines.push({
      key: `node-${log.id}-start`,
      time: fmtTime(log.startedAt),
      level: 'info',
      tag: label,
      message: '开始执行',
    })

    // 爬虫节点：注入与「爬虫调度」页一模一样的实时日志（不带 tag，仅供当前运行展示，不入库）
    if (type === 'crawler_run' && crawlerLogs.length > 0) {
      for (const cl of crawlerLogs) {
        lines.push({
          key: `crawler-${cl.id}`,
          time: cl.timestamp,
          level: cl.level,
          message: cl.message,
        })
      }
    }

    // 节点内部进度日志（由节点在执行过程中写入 output.progress）
    const progressLines: string[] = Array.isArray(log.output?.progress) ? log.output.progress : []
    for (let pi = 0; pi < progressLines.length; pi++) {
      lines.push({
        key: `node-${log.id}-progress-${pi}`,
        time: fmtTime(log.startedAt),
        level: 'info',
        tag: label,
        message: progressLines[pi],
      })
    }

    if (log.status !== 'running') {
      const summary = summarizeNodeOutput(type, log.output)
      const withSummary = (base: string) => (summary ? `${base} — ${summary}` : base)
      const endMap: Record<string, { level: ConsoleLevel; text: string }> = {
        success: { level: 'success', text: withSummary('执行完成') },
        failed: { level: 'error', text: `执行失败${log.errorMsg ? '：' + log.errorMsg : ''}` },
        cancelled: { level: 'warning', text: '已取消' },
        partial_success: { level: 'warning', text: withSummary('部分成功') },
      }
      const end = endMap[log.status]
      if (end) {
        lines.push({
          key: `node-${log.id}-end`,
          time: fmtTime(log.finishedAt),
          level: end.level,
          tag: label,
          message: end.text,
        })
      }
    }
  }

  if (execution && execution.status !== 'running') {
    const finishMap: Record<string, { level: ConsoleLevel; text: string }> = {
      success: { level: 'success', text: '工作流执行成功' },
      failed: { level: 'error', text: '工作流执行失败' },
      cancelled: { level: 'warning', text: '工作流已取消' },
      partial_success: { level: 'warning', text: '工作流部分成功' },
    }
    const f = finishMap[execution.status]
    if (f) {
      lines.push({
        key: 'exec-end',
        time: fmtTime(execution.finishedAt),
        level: f.level,
        tag: '工作流',
        message: f.text + (execution.errorMsg ? '：' + execution.errorMsg : ''),
      })
    }
  }

  return lines
}

// 控制台「最新一次执行」内容的浏览器缓存（按工作流 id），切换页面后仍可查看
const CONSOLE_CACHE_PREFIX = 'wf_console_cache_'

interface ConsoleCache {
  execution: WorkflowExecution | null
  nodeLogs: WorkflowNodeExecution[]
  crawlerLogs: CrawlerLog[]
}

const loadConsoleCache = (wfId: string): ConsoleCache | null => {
  try {
    const raw = localStorage.getItem(CONSOLE_CACHE_PREFIX + wfId)
    if (!raw) return null
    return JSON.parse(raw) as ConsoleCache
  } catch {
    return null
  }
}

const saveConsoleCache = (wfId: string, data: ConsoleCache) => {
  try {
    // 爬虫实时日志只缓存最近 200 条，避免超出 localStorage 容量
    const trimmed: ConsoleCache = {
      execution: data.execution,
      nodeLogs: data.nodeLogs,
      crawlerLogs: data.crawlerLogs.slice(-200),
    }
    localStorage.setItem(CONSOLE_CACHE_PREFIX + wfId, JSON.stringify(trimmed))
  } catch {
    // 容量超限等异常忽略
  }
}

const clearConsoleCache = (wfId: string) => {
  try {
    localStorage.removeItem(CONSOLE_CACHE_PREFIX + wfId)
    localStorage.setItem(CONSOLE_CACHE_PREFIX + wfId + '_cleared', '1')
  } catch {
    // 忽略
  }
}

const isConsoleCacheCleared = (wfId: string): boolean => {
  try {
    return localStorage.getItem(CONSOLE_CACHE_PREFIX + wfId + '_cleared') === '1'
  } catch {
    return false
  }
}

const resetConsoleCacheCleared = (wfId: string) => {
  try {
    localStorage.removeItem(CONSOLE_CACHE_PREFIX + wfId + '_cleared')
  } catch {
    // 忽略
  }
}

const WorkflowEditorPage: React.FC = () => {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const isEdit = !!id
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  // React Flow 状态
  const [nodes, setNodes, onNodesChange] = useNodesState([])
  const [edges, setEdges, onEdgesChange] = useEdgesState([])

  // 节点配置抽屉
  const [drawerVisible, setDrawerVisible] = useState(false)
  const [selectedNode, setSelectedNode] = useState<Node | null>(null)
  const [nodeConfigForm] = Form.useForm()

  // 画布拖拽：ReactFlow 实例与画布容器引用（用于把屏幕坐标换算成画布坐标）
  const reactFlowWrapper = useRef<HTMLDivElement>(null)
  const [rfInstance, setRfInstance] = useState<any>(null)

  // 对齐吸附辅助线位置
  const [helperLineH, setHelperLineH] = useState<number | undefined>(undefined)
  const [helperLineV, setHelperLineV] = useState<number | undefined>(undefined)

  // 自增触发器：每次加载工作流后 +1，让画布在节点测量完成后自动适配一次
  const [fitTrigger, setFitTrigger] = useState(0)

  // 告警规则（供「告警评估」节点选择）
  const [alertRules, setAlertRules] = useState<{ id: number; name: string }[]>([])

  // 话题列表（供工作流表单选择）
  const [topicOptions, setTopicOptions] = useState<{ label: string; value: string }[]>([])

  // 爬取数量上限（从后台管理配置读取）
  const [crawlerMaxLimit, setCrawlerMaxLimit] = useState<number>(500)
  const [crawlerMaxCommentsLimit, setCrawlerMaxCommentsLimit] = useState<number>(1000)
  const [crawlerMaxSubCommentsLimit, setCrawlerMaxSubCommentsLimit] = useState<number>(500)

  // 右键菜单
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; node: Node } | null>(null)

  // 全局点击关闭右键菜单
  useEffect(() => {
    if (!contextMenu) return
    const handleGlobalClick = () => setContextMenu(null)
    document.addEventListener('click', handleGlobalClick)
    return () => document.removeEventListener('click', handleGlobalClick)
  }, [contextMenu])

  // ============ 执行 / 控制台状态 ============
  const location = useLocation()
  // 控制台面板模式：console=实时日志，history=历史记录，hidden=收起
  const [consoleMode, setConsoleMode] = useState<'console' | 'history' | 'hidden'>('console')
  const [isExecuting, setIsExecuting] = useState(false)
  const [executing, setExecuting] = useState(false) // 触发执行请求中（防抖按钮）
  const [currentExecution, setCurrentExecution] = useState<WorkflowExecution | null>(null)
  const [nodeLogs, setNodeLogs] = useState<WorkflowNodeExecution[]>([])
  const [crawlerLogs, setCrawlerLogs] = useState<CrawlerLog[]>([])

  // 历史记录
  const [history, setHistory] = useState<WorkflowExecution[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)
  const [detailVisible, setDetailVisible] = useState(false)
  const [detailExecution, setDetailExecution] = useState<WorkflowExecution | null>(null)
  const [detailLogs, setDetailLogs] = useState<WorkflowNodeExecution[]>([])
  const [detailLoading, setDetailLoading] = useState(false)
  const [regenerating, setRegenerating] = useState(false)

  // 当前控制台显示的是哪次执行的日志（用于右键重跑的参考）
  // null 表示实时执行中的 currentExecution，非 null 表示用户查看了某次历史执行
  const [replayExecId, setReplayExecId] = useState<number | null>(null)

  // 画布锁定：执行中或查看历史时，禁止编辑
  const isLocked = isExecuting || replayExecId !== null

  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const consoleEndRef = useRef<HTMLDivElement>(null)
  const panelContentRef = useRef<HTMLDivElement>(null)
  const currentExecIdRef = useRef<number | null>(null)
  const autoRunHandledRef = useRef(false)
  const cacheRestoredRef = useRef(false)

  // 当前工作流是否包含爬虫节点（决定是否拉取爬虫实时日志）
  const hasCrawlerNode = nodes.some((n) => n.data?.type === 'crawler_run')

  useEffect(() => {
    if (isEdit) {
      loadWorkflow()
    }
  }, [id])

  // 挂载时从浏览器缓存恢复「最新一次执行」的控制台内容
  useEffect(() => {
    if (!isEdit || !id || cacheRestoredRef.current) return
    cacheRestoredRef.current = true
    const cached = loadConsoleCache(id)
    if (cached) {
      setCurrentExecution(cached.execution)
      setNodeLogs(cached.nodeLogs || [])
      setCrawlerLogs(cached.crawlerLogs || [])
      if (cached.execution) currentExecIdRef.current = cached.execution.id
    }
  }, [isEdit, id])

  useEffect(() => {
    alertApi
      .listRules()
      .then((rules) => setAlertRules(rules || []))
      .catch(() => setAlertRules([]))
  }, [])

  useEffect(() => {
    topicApi
      .list({ pageSize: 200 })
      .then((res) => setTopicOptions((res.list || []).map((t: Topic) => ({ label: t.name, value: t.name }))))
      .catch(() => setTopicOptions([]))
  }, [])

  useEffect(() => {
    workflowApi
      .getCrawlerLimits()
      .then((res) => {
        setCrawlerMaxLimit(res.maxNotesCount || 500)
        setCrawlerMaxCommentsLimit(res.maxCommentsCount || 1000)
        setCrawlerMaxSubCommentsLimit(res.maxSubCommentsCount || 500)
      })
      .catch(() => {
        setCrawlerMaxLimit(500)
        setCrawlerMaxCommentsLimit(1000)
      })
  }, [])


  const loadWorkflow = async () => {
    setLoading(true)
    try {
      const workflow = await workflowApi.detail(Number(id))
      // 解析 triggerConfig 结构化字段
      const tc = (workflow.triggerConfig || {}) as any
      const savedCron: string = tc.cron || ''
      const cronFields = savedCron ? (parseCronToFields(savedCron) ?? {}) : {}
      form.setFieldsValue({
        name: workflow.name,
        description: workflow.description,
        status: workflow.status === 1,
        triggerType: workflow.triggerType || 'manual',
        tc_webhookSecret: tc.secret || undefined,
        // schedule 默认 fixed-daily 兜底
        tc_scheduleMode: cronFields.tc_scheduleMode ?? 'fixed',
        tc_fixedFreq:     cronFields.tc_fixedFreq     ?? 'daily',
        tc_fixedHour:     cronFields.tc_fixedHour     ?? 8,
        tc_fixedMinute:   cronFields.tc_fixedMinute   ?? 0,
        tc_fixedWeekday:  cronFields.tc_fixedWeekday  ?? 1,
        tc_fixedMonthDay: cronFields.tc_fixedMonthDay ?? 1,
        tc_intervalValue: cronFields.tc_intervalValue ?? 1,
        tc_intervalUnit:  cronFields.tc_intervalUnit  ?? 'hours',
      })

      // 转换为 React Flow 格式（优先使用已保存的位置，避免重新编辑时布局错乱）
      const flowNodes = (workflow.nodes || []).map((node: any, index: number) => ({
        id: node.id,
        type: 'custom',
        position:
          node.position && typeof node.position.x === 'number' && typeof node.position.y === 'number'
            ? node.position
            : { x: 100 + index * 250, y: 100 },
        data: {
          label: node.label || node.id,
          type: node.type,
          config: node.config,
        },
      }))

      const flowEdges = (workflow.edges || []).map((edge: any, index: number) => ({
        id: `edge-${index}`,
        source: edge.source,
        target: edge.target,
        type: 'smoothstep',
        animated: true,
        style: EDGE_STYLE,
        markerEnd: EDGE_MARKER,
      }))

      setNodes(flowNodes)
      setEdges(flowEdges)
      setFitTrigger((t) => t + 1) // 节点测量完成后自动适配视口，完整展示所有节点
    } catch (error) {
      message.error('加载工作流失败')
    } finally {
      setLoading(false)
    }
  }

  // ============ 执行监控 ============

  const stopPolling = useCallback(() => {
    if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current)
      pollTimerRef.current = null
    }
  }, [])

  // 拉取一次执行状态 + 节点日志 + 爬虫实时日志
  const pollOnce = useCallback(async () => {
    const execId = currentExecIdRef.current
    if (!execId || !id) return
    try {
      const tasks: [
        Promise<{ list?: WorkflowExecution[] }>,
        Promise<WorkflowNodeExecution[]>,
        Promise<{ logs: CrawlerLog[] }> | null,
      ] = [
        workflowApi.executions(Number(id), { page: 1, pageSize: 10 }),
        workflowApi.executionLogs(execId),
        hasCrawlerNode ? crawlerApi.getLogs(200) : null,
      ]
      const [execRes, logs, crawlerRes] = await Promise.all([tasks[0], tasks[1], tasks[2]])

      const list = execRes.list || []
      setHistory(list)
      const exec = list.find((e) => e.id === execId) || null
      if (exec) setCurrentExecution(exec)
      setNodeLogs(logs || [])
      if (crawlerRes) setCrawlerLogs(crawlerRes.logs || [])

      // 终态：停止轮询并解除编辑锁定
      if (exec && exec.status !== 'running') {
        stopPolling()
        setIsExecuting(false)
      }
    } catch (error) {
      // 轮询失败静默处理，下一轮重试
    }
  }, [id, hasCrawlerNode, stopPolling])

  const startMonitor = useCallback(
    (execId: number) => {
      currentExecIdRef.current = execId
      stopPolling()
      pollOnce()
      pollTimerRef.current = setInterval(pollOnce, 2000)
    },
    [pollOnce, stopPolling]
  )

  // 切换到历史模式时滚动到顶部
  useEffect(() => {
    if (consoleMode === 'history' && panelContentRef.current) {
      panelContentRef.current.scrollTop = 0
    }
  }, [consoleMode])

  // 进入终态后自动滚动到底部（仅控制台模式）
  useEffect(() => {
    if (consoleMode !== 'console') return
    consoleEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [nodeLogs, crawlerLogs, currentExecution, consoleMode])

  // 控制台内容变化时写入浏览器缓存（仅在有执行记录时；清空由按钮显式处理）
  useEffect(() => {
    if (!isEdit || !id) return
    if (currentExecution) {
      saveConsoleCache(id, { execution: currentExecution, nodeLogs, crawlerLogs })
    }
  }, [isEdit, id, currentExecution, nodeLogs, crawlerLogs])

  // 卸载时清理定时器
  useEffect(() => stopPolling, [stopPolling])

  // 加载完成后：
  // 1) 若有正在运行的执行（可能是后台/定时触发），自动接管监控并展示完整日志；
  // 2) 否则若缓存里的执行在离开期间已结束，刷新为最终状态 + 完整节点日志。
  useEffect(() => {
    if (!isEdit || loading) return
    let cancelled = false
    workflowApi
      .executions(Number(id), { page: 1, pageSize: 10 })
      .then(async (res) => {
        if (cancelled) return
        const list = res.list || []
        setHistory(list)
        const running = list.find((e) => e.status === 'running')
        if (running && !pollTimerRef.current) {
          // 后台正在执行：接管监控，拉取完整节点日志 + 爬虫实时日志
          if (id) resetConsoleCacheCleared(id)
          setIsExecuting(true)
          setCurrentExecution(running)
          setConsoleMode('console')
          startMonitor(running.id)
          return
        }
        if (!running) {
          // 用户手动清空过控制台 → 不自动恢复最新执行
          if (id && isConsoleCacheCleared(id)) return
          // 没有运行中的执行：展示最新一次执行，刷新为服务端最终状态 + 完整节点日志
          const latest = list[0]
          if (latest) {
            const sameAsCache = currentExecIdRef.current === latest.id
            currentExecIdRef.current = latest.id
            setCurrentExecution(latest)
            try {
              const logs = await workflowApi.executionLogs(latest.id)
              if (!cancelled) setNodeLogs(logs || [])
            } catch {
              // 拉取失败则保留缓存日志
            }
            // 爬虫实时日志来自内存缓冲，无法为非缓存的历史执行恢复
            if (!sameAsCache) setCrawlerLogs([])
          }
        }
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [isEdit, id, loading])

  const downloadReport = async (reportId: string, reportFormat?: string) => {
    try {
      const token = useAuthStore.getState().token
      const resp = await fetch(`/api/reports/${reportId}/download`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      })
      if (!resp.ok) throw new Error('报告不存在或已过期')
      const blob = await resp.blob()
      const ext = reportFormat === 'html' ? 'html' : 'md'
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `report-${String(reportId).slice(0, 8)}.${ext}`
      a.click()
      URL.revokeObjectURL(url)
    } catch (e: any) {
      message.error(e?.message || '下载失败')
    }
  }

  const handleRegenerateReport = async () => {
    if (!detailExecution?.id) return
    setRegenerating(true)
    try {
      const res = await reportApi.regenerate({ executionId: detailExecution.id })
      message.success(`报告重新生成成功（${res.articleCount} 篇文章）`)
      downloadReport(res.reportId, res.format)
    } catch (e: any) {
      message.error(e?.response?.data?.message || e?.message || '重新生成失败')
    } finally {
      setRegenerating(false)
    }
  }

  const handleNodeContextMenu = useCallback((event: React.MouseEvent, node: Node) => {
    event.preventDefault()
    setContextMenu({ x: event.clientX, y: event.clientY, node })
  }, [])

  // 查看某次历史执行：把那次执行的日志加载到控制台，节点画布显示对应状态
  const handleReplayExecution = useCallback(async (execution: WorkflowExecution) => {
    setDetailVisible(false)
    setCurrentExecution(execution)
    setReplayExecId(execution.id)
    setNodeLogs([])
    setCrawlerLogs([])
    setConsoleMode('console')
    currentExecIdRef.current = execution.id
    try {
      const logs = await workflowApi.executionLogs(execution.id)
      setNodeLogs(logs || [])
    } catch {
      message.error('加载执行日志失败')
    }
  }, [])

  // 从指定节点重跑，以 execId 作为前序状态参考
  const handleExecuteFromNode = useCallback(async (nodeId: string, execId: number) => {
    setContextMenu(null)
    if (!isEdit || !id) {
      message.warning('请先保存工作流后再执行')
      return
    }
    setExecuting(true)
    try {
      const exec = await workflowApi.executeFromNode(Number(id), nodeId, execId)
      setDetailVisible(false)
      message.success(`已从节点重跑（执行 #${exec.id}）`)
      resetConsoleCacheCleared(id)
      setIsExecuting(true)
      setReplayExecId(null)
      setCurrentExecution({ id: exec.id, workflowId: Number(id), status: 'running', startedAt: new Date().toISOString() })
      setNodeLogs([])
      setCrawlerLogs([])
      setConsoleMode('console')
      startMonitor(exec.id)
    } catch (error: any) {
      const msg = error?.response?.data?.message || error?.message || '重跑失败'
      message.error(msg)
    } finally {
      setExecuting(false)
    }
  }, [isEdit, id, startMonitor])

  // 右键菜单触发：优先使用当前查看/实时执行的 execId
  const handleContextMenuRerun = useCallback((node: Node) => {
    const execId = replayExecId ?? (currentExecution?.status !== 'running' ? currentExecution?.id : null)
      ?? history.find((e) => e.status !== 'running')?.id
    if (!execId) {
      message.warning('没有可参考的历史执行记录，请先执行一次工作流，或在历史记录中双击进入')
      setContextMenu(null)
      return
    }
    handleExecuteFromNode(node.id, execId)
  }, [replayExecId, currentExecution, history, handleExecuteFromNode])

  const handleExecute = useCallback(async () => {
    if (!isEdit || !id) {
      message.warning('请先保存工作流后再执行')
      return
    }
    setExecuting(true)
    try {
      const exec = await workflowApi.execute(Number(id))
      message.success(`已开始执行（执行 #${exec.id}）`)
      resetConsoleCacheCleared(id)
      setIsExecuting(true)
      setCurrentExecution(exec)
      setNodeLogs([])
      setCrawlerLogs([])
      setConsoleMode('console')
      startMonitor(exec.id)
    } catch (error: any) {
      message.error(error?.message || '执行失败')
    } finally {
      setExecuting(false)
    }
  }, [isEdit, id, startMonitor])

  const handleCancelExecution = useCallback(async () => {
    const execId = currentExecIdRef.current
    if (!execId) return
    try {
      await workflowApi.cancelExecution(execId)
      message.success('取消信号已发送')
      pollOnce()
    } catch (error) {
      // 错误提示由拦截器处理
    }
  }, [pollOnce])

  // ============ 控制台面板交互 ============

  // 关闭控制台（×）：实时日志态 → 历史态；历史态 → 收起
  const handleCloseConsole = () => {
    if (consoleMode === 'console') {
      refreshHistory()
      setConsoleMode('history')
    } else {
      setConsoleMode('hidden')
    }
  }

  const handleOpenHistory = () => {
    refreshHistory()
    setConsoleMode('history')
  }

  // 顶部「控制台」按钮：收起 ⇄ 展开（展开默认进入实时控制台）
  const toggleConsole = () => {
    setConsoleMode((m) => (m === 'hidden' ? 'console' : 'hidden'))
  }

  const refreshHistory = async () => {
    if (!isEdit || !id) return
    setHistoryLoading(true)
    try {
      const res = await workflowApi.executions(Number(id), { page: 1, pageSize: 50 })
      setHistory(res.list || [])
    } catch (error) {
      // 静默
    } finally {
      setHistoryLoading(false)
    }
  }

  const handleViewHistoryDetail = async (execution: WorkflowExecution) => {
    setDetailExecution(execution)
    setDetailVisible(true)
    setDetailLoading(true)
    try {
      const logs = await workflowApi.executionLogs(execution.id)
      setDetailLogs(logs || [])
    } catch (error) {
      message.error('加载执行详情失败')
    } finally {
      setDetailLoading(false)
    }
  }

  // 从列表页点击「执行」进入时自动触发一次执行
  useEffect(() => {
    const st = location.state as { autoRun?: boolean } | null
    if (isEdit && !loading && st?.autoRun && !autoRunHandledRef.current && !isExecuting) {
      autoRunHandledRef.current = true
      // 清除 history state，防止刷新页面重复触发
      navigate(location.pathname, { replace: true, state: {} })
      handleExecute()
    }
  }, [isEdit, loading, location.state, isExecuting, handleExecute])

  const onConnect = useCallback(
    (params: Connection) => {
      setEdges((eds) =>
        addEdge(
          {
            ...params,
            type: 'smoothstep',
            animated: true,
            style: EDGE_STYLE,
            markerEnd: EDGE_MARKER,
          },
          eds
        )
      )
    },
    [setEdges]
  )

  // 拖拽节点时计算对齐吸附并显示辅助线
  const handleNodesChange = useCallback(
    (changes: NodeChange[]) => {
      setHelperLineH(undefined)
      setHelperLineV(undefined)

      const change = changes[0]
      if (changes.length === 1 && change.type === 'position' && change.dragging && change.position) {
        const lines = getHelperLines(change as NodePositionChange, nodes)
        if (lines.snapX !== undefined) change.position.x = lines.snapX
        if (lines.snapY !== undefined) change.position.y = lines.snapY
        setHelperLineH(lines.horizontal)
        setHelperLineV(lines.vertical)
      }

      onNodesChange(changes)
    },
    [nodes, onNodesChange]
  )

  // 从右侧节点库开始拖拽：记录节点类型
  const onPaletteDragStart = (event: React.DragEvent, nodeType: string) => {
    event.dataTransfer.setData('application/reactflow', nodeType)
    event.dataTransfer.effectAllowed = 'move'
  }

  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }, [])

  // 拖放到画布：在落点位置创建节点
  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault()
      const nodeType = event.dataTransfer.getData('application/reactflow')
      if (!nodeType || !rfInstance) return
      const nodeConfig = NODE_REGISTRY[nodeType as keyof typeof NODE_REGISTRY]
      if (!nodeConfig) return

      const position = rfInstance.screenToFlowPosition({ x: event.clientX, y: event.clientY })
      const nodeId = `node_${Date.now()}`
      const newNode: Node = {
        id: nodeId,
        type: 'custom',
        position,
        data: { label: nodeId, type: nodeType, config: {} },
      }
      setNodes((nds) => nds.concat(newNode))
      message.success(`已添加 ${nodeConfig.label} 节点`)
    },
    [rfInstance, setNodes]
  )

  const handleNodeClick = useCallback((_event: React.MouseEvent, node: Node) => {
    setSelectedNode(node)
    nodeConfigForm.setFieldsValue({
      label: node.data.label,
      ...node.data.config,
    })
    setDrawerVisible(true)
  }, [])

  const handleNodeConfigSave = async () => {
    try {
      const values = await nodeConfigForm.validateFields()
      const { label, ...config } = values

      setNodes((nds) =>
        nds.map((node) => {
          if (node.id === selectedNode?.id) {
            return {
              ...node,
              data: {
                ...node.data,
                label,
                config,
              },
            }
          }
          return node
        })
      )

      setDrawerVisible(false)
      message.success('节点配置已更新')
    } catch (error) {
      // 验证失败
    }
  }

  const handleDeleteNode = () => {
    if (!selectedNode) return
    setNodes((nds) => nds.filter((node) => node.id !== selectedNode.id))
    setEdges((eds) => eds.filter((edge) => edge.source !== selectedNode.id && edge.target !== selectedNode.id))
    setDrawerVisible(false)
    message.success('节点已删除')
  }

  const handleSubmit = async (values: any) => {
    if (nodes.length === 0) {
      message.error('请至少添加一个节点')
      return
    }

    // 图结构校验：告警但不拦截
    const edgeSimple = edges.map((e) => ({ source: e.source, target: e.target }))
    const warnings = validateWorkflowGraph(nodes, edgeSimple)
    if (warnings.length > 0) {
      Modal.warning({
        title: '工作流结构提示',
        content: (
          <ul style={{ paddingLeft: 16, margin: 0 }}>
            {warnings.map((w, i) => <li key={i} style={{ marginBottom: 4 }}>{w}</li>)}
          </ul>
        ),
        okText: '继续保存',
      })
    }

    setSaving(true)
    try {
      // 根据触发类型组装 triggerConfig
      let triggerConfig: Record<string, any> = {}
      if (values.triggerType === 'schedule') {
        const cron = buildCronFromValues(values)
        if (!cron) {
          message.error('请完善定时配置')
          setSaving(false)
          return
        }
        triggerConfig = { cron }
      }

      // 转换为后端格式
      const workflowNodes = nodes.map((node) => ({
        id: node.id,
        type: node.data.type,
        label: node.data.label,
        position: node.position,
        config: node.data.config || {},
      }))

      const workflowEdges = edges.map((edge) => ({
        id: edge.id,
        source: edge.source,
        target: edge.target,
      }))

      const payload: Partial<Workflow> = {
        name: values.name,
        description: values.description,
        status: values.status ? 1 : 0,
        triggerType: values.triggerType,
        triggerConfig,
        nodes: workflowNodes,
        edges: workflowEdges,
      }

      if (isEdit) {
        await workflowApi.update(Number(id), payload)
        message.success('更新成功')
      } else {
        const created = await workflowApi.create(payload)
        message.success('创建成功')
        navigate(`/workflows/${created.id}/edit`, { replace: true })
      }
    } catch (error: any) {
      message.error(error.message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const currentNodeType = selectedNode ? NODE_REGISTRY[selectedNode.data.type as keyof typeof NODE_REGISTRY] : null
  const watchedTriggerType   = Form.useWatch('triggerType',     form)
  const watchedScheduleMode  = Form.useWatch('tc_scheduleMode', form)
  const watchedFixedFreq     = Form.useWatch('tc_fixedFreq',    form)
  // 节点配置抽屉：监听 crawlerType / format，用于条件性显示字段
  const watchedNodeCrawlerType = Form.useWatch('crawlerType', nodeConfigForm)
  const watchedNodeFormat = Form.useWatch('format', nodeConfigForm)

  // 节点执行状态映射：nodeId → status，供画布节点渲染流光/徽章效果
  const nodeStatusMap = React.useMemo(() => {
    const map: Record<string, string> = {}
    nodeLogs.forEach((log) => { map[log.nodeId] = log.status })
    return map
  }, [nodeLogs])

  // 控制台实时日志行（工作流编排 + 爬虫实时日志合并）
  const consoleLines = buildConsoleLines({
    execution: currentExecution,
    nodeLogs,
    nodes,
    crawlerLogs: hasCrawlerNode ? crawlerLogs : [],
  })
  // 执行计划（线性顺序）
  const executionPlan = computePlan(nodes, edges.map((e) => ({ source: e.source, target: e.target })))

  const historyColumns: ColumnsType<WorkflowExecution> = [
    { title: '执行ID', dataIndex: 'id', width: 90 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 120,
      render: (status: string) => {
        const c = getExecStatusConfig(status)
        return <Tag color={c.color} icon={c.icon}>{c.label}</Tag>
      },
    },
    {
      title: '开始时间',
      dataIndex: 'startedAt',
      width: 170,
      render: (t: string) => t?.replace('T', ' ').slice(0, 19),
    },
    {
      title: '耗时',
      key: 'duration',
      width: 90,
      render: (_: any, r: WorkflowExecution) => {
        if (!r.finishedAt) return '-'
        const d = Math.round((new Date(r.finishedAt).getTime() - new Date(r.startedAt).getTime()) / 1000)
        return `${d}秒`
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 80,
      render: (_: any, r: WorkflowExecution) => (
        <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => handleViewHistoryDetail(r)}>
          详情
        </Button>
      ),
    },
  ]

  return (
    <div style={{ height: 'calc(100vh - 96px)', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <PageHeader
        title={replayExecId ? `执行记录 #${replayExecId}（只读）` : isEdit ? '编辑工作流' : '新建工作流'}
        extra={
          <Space>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              loading={saving}
              disabled={isLocked}
              onClick={() => form.submit()}
            >
              保存
            </Button>
            {isEdit &&
              (isExecuting ? (
                <Popconfirm
                  title="确定取消执行？"
                  description="正在执行的节点会在下一个检查点退出"
                  onConfirm={handleCancelExecution}
                  okText="取消执行"
                  cancelText="返回"
                >
                  <Button danger icon={<StopOutlined />}>
                    取消执行
                  </Button>
                </Popconfirm>
              ) : (
                <Button
                  type="primary"
                  ghost
                  icon={<PlayCircleOutlined />}
                  loading={executing}
                  onClick={handleExecute}
                >
                  执行
                </Button>
              ))}
            <Button
              icon={<CodeOutlined />}
              type={consoleMode === 'hidden' ? 'default' : 'primary'}
              ghost={consoleMode !== 'hidden'}
              onClick={toggleConsole}
            >
              {consoleMode === 'hidden' ? '控制台' : '隐藏控制台'}
            </Button>
            {replayExecId && !isExecuting && (
              <Button
                icon={<CloseOutlined />}
                onClick={() => { setReplayExecId(null); setNodeLogs([]); setCrawlerLogs([]) }}
              >
                退出记录
              </Button>
            )}
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/workflows')}>
              返回
            </Button>
          </Space>
        }
      />

      {isExecuting && (
        <Alert
          type="warning"
          showIcon
          banner
          message="工作流执行中，已锁定编辑。可在下方控制台查看实时执行日志。"
        />
      )}

      {/* 内容区：三栏行 + 控制台，共享剩余高度，内部自行分配 */}
      <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div style={{ flex: 1, minHeight: 0, display: 'flex', padding: '12px 16px 8px', gap: 16 }}>
        {/* 左侧配置面板 */}
        <Card style={{ width: 320, overflow: 'auto' }} bodyStyle={{ padding: '12px 16px' }} loading={loading}>
          <Form
            form={form}
            layout="vertical"
            onFinish={handleSubmit}
            disabled={isLocked}
            initialValues={{
              status: true,
              triggerType: 'manual',
              tc_scheduleMode: 'fixed',
              tc_fixedFreq: 'daily',
              tc_fixedHour: 8,
              tc_fixedMinute: 0,
              tc_fixedWeekday: 1,
              tc_fixedMonthDay: 1,
              tc_intervalValue: 1,
              tc_intervalUnit: 'hours',
            }}
          >
            <Form.Item
              label="工作流名称"
              name="name"
              rules={[{ required: true, message: '请输入工作流名称' }]}
            >
              <Input placeholder="例如：每日舆情分析" />
            </Form.Item>

            <Form.Item label="描述" name="description">
              <TextArea rows={2} placeholder="工作流用途说明" />
            </Form.Item>

            <Form.Item
              label="触发方式"
              name="triggerType"
              rules={[{ required: true, message: '请选择触发方式' }]}
            >
              <Select>
                <Select.Option value="manual">仅手动触发</Select.Option>
                <Select.Option value="schedule">自动触发（定时 / 周期）</Select.Option>
              </Select>
            </Form.Item>

            {/* 自动触发：定时 vs 周期 */}
            {watchedTriggerType === 'schedule' && (
              <>
                <Form.Item label="触发模式" name="tc_scheduleMode">
                  <Select>
                    <Select.Option value="fixed">定时触发（指定时间点）</Select.Option>
                    <Select.Option value="interval">周期触发（每隔 N 时间）</Select.Option>
                  </Select>
                </Form.Item>

                {/* 定时触发 */}
                {watchedScheduleMode === 'fixed' && (
                  <>
                    <Form.Item label="重复频率" name="tc_fixedFreq">
                      <Select>
                        <Select.Option value="daily">每天</Select.Option>
                        <Select.Option value="weekly">每周</Select.Option>
                        <Select.Option value="monthly">每月</Select.Option>
                      </Select>
                    </Form.Item>
                    {watchedFixedFreq === 'weekly' && (
                      <Form.Item label="星期几" name="tc_fixedWeekday">
                        <Select>
                          {['周一','周二','周三','周四','周五','周六','周日'].map((d, i) => (
                            <Select.Option key={i+1} value={i+1}>{d}</Select.Option>
                          ))}
                        </Select>
                      </Form.Item>
                    )}
                    {watchedFixedFreq === 'monthly' && (
                      <Form.Item label="每月第几天" name="tc_fixedMonthDay" rules={[{ required: true }]}>
                        <InputNumber min={1} max={28} style={{ width: '100%' }} addonAfter="号" />
                      </Form.Item>
                    )}
                    <Form.Item label="执行时间">
                      <Space>
                        <Form.Item name="tc_fixedHour" noStyle>
                          <InputNumber min={0} max={23} style={{ width: 80 }} addonAfter="时" />
                        </Form.Item>
                        <Form.Item name="tc_fixedMinute" noStyle>
                          <InputNumber min={0} max={59} style={{ width: 80 }} addonAfter="分" />
                        </Form.Item>
                      </Space>
                    </Form.Item>
                  </>
                )}

                {/* 周期触发 */}
                {watchedScheduleMode === 'interval' && (
                  <Form.Item label="执行间隔">
                    <Space>
                      <span>每</span>
                      <Form.Item name="tc_intervalValue" noStyle rules={[{ required: true }]}>
                        <InputNumber min={1} max={999} style={{ width: 80 }} />
                      </Form.Item>
                      <Form.Item name="tc_intervalUnit" noStyle>
                        <Select style={{ width: 90 }}>
                          <Select.Option value="minutes">分钟</Select.Option>
                          <Select.Option value="hours">小时</Select.Option>
                          <Select.Option value="days">天</Select.Option>
                        </Select>
                      </Form.Item>
                      <span>执行一次</span>
                    </Space>
                  </Form.Item>
                )}
              </>
            )}


            <Form.Item label="状态" name="status" valuePropName="checked">
              <Switch checkedChildren="启用" unCheckedChildren="禁用" />
            </Form.Item>
          </Form>
        </Card>

        {/* 中间画布 */}
        <Card style={{ flex: 1 }} bodyStyle={{ padding: 0, height: '100%' }}>
          <div
            ref={reactFlowWrapper}
            style={{ width: '100%', height: '100%' }}
            onDrop={isLocked ? undefined : onDrop}
            onDragOver={isLocked ? undefined : onDragOver}
          >
            <ExecutionStatusContext.Provider value={nodeStatusMap}>
            <ReactFlow
              nodes={nodes}
              edges={edges}
              onNodesChange={isLocked ? undefined : handleNodesChange}
              onEdgesChange={isLocked ? undefined : onEdgesChange}
              onConnect={isLocked ? undefined : onConnect}
              onNodeClick={handleNodeClick}
              onNodeContextMenu={handleNodeContextMenu}
              onPaneClick={() => setContextMenu(null)}
              onInit={setRfInstance}
              nodeTypes={nodeTypes}
              nodesDraggable={!isLocked}
              nodesConnectable={!isLocked}
              elementsSelectable={!isLocked}
              fitView
            >
              <Background />
              <Controls />
              <FitOnLoad trigger={fitTrigger} />
              <HelperLines horizontal={helperLineH} vertical={helperLineV} />

            </ReactFlow>
            </ExecutionStatusContext.Provider>
          </div>
          {/* 节点右键菜单 */}
          {contextMenu && (
            <div
              style={{
                position: 'fixed', top: contextMenu.y, left: contextMenu.x, zIndex: 9999,
                background: '#fff', border: '1px solid #d9d9d9', borderRadius: 6,
                boxShadow: '0 4px 12px rgba(0,0,0,0.12)', minWidth: 160, overflow: 'hidden',
              }}
            >
              <div style={{ padding: '6px 12px', fontSize: 12, color: '#666', borderBottom: '1px solid #f0f0f0', fontWeight: 500 }}>
                {contextMenu.node.data?.label || contextMenu.node.id}
              </div>
              <div
                role="button"
                style={{ padding: '8px 12px', cursor: 'pointer', fontSize: 13 }}
                onMouseEnter={(e) => (e.currentTarget.style.background = '#f5f5f5')}
                onMouseLeave={(e) => (e.currentTarget.style.background = '')}
                onClick={() => handleContextMenuRerun(contextMenu.node)}
              >
                ▶ 从此节点重跑
              </div>
            </div>
          )}
        </Card>

        {/* 右侧节点库（小方块、一排两个、拖拽添加，可滚动） */}
        <div
          style={{
            width: 240,
            height: '100%',
            display: 'flex',
            flexDirection: 'column',
            background: '#fff',
            borderRadius: 8,
            border: '1px solid #f0f0f0',
            overflow: 'hidden',
          }}
        >
          <div style={{ padding: '12px 14px', borderBottom: '1px solid #f0f0f0', fontWeight: 600 }}>
            节点库
          </div>
          <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '8px 14px 14px' }}>
            {([
              { category: 'source'  as const, label: '起始节点', desc: '数据来源' },
              { category: 'sync'    as const, label: '数据同步', desc: '平台数据接入' },
              { category: 'process' as const, label: '中间节点', desc: '数据处理与分析' },
              { category: 'ops'     as const, label: '运维节点', desc: '配置与外部调用' },
            ]).map(({ category, label, desc }) => {
              const items = Object.entries(NODE_REGISTRY).filter(([key]) => NODE_CATEGORY[key] === category)
              if (items.length === 0) return null
              return (
                <div key={category} style={{ marginBottom: 16 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
                    <span style={{ fontSize: 12, fontWeight: 600, color: '#595959', whiteSpace: 'nowrap' }}>{label}</span>
                    <span style={{ fontSize: 11, color: '#bfbfbf' }}>{desc}</span>
                    <div style={{ flex: 1, height: 1, background: '#f0f0f0' }} />
                  </div>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                    {items.map(([key, config]) => {
                      const cat = NODE_CATEGORY[key] || 'process'
                      return (
                        <div
                          key={key}
                          draggable={!isExecuting}
                          onDragStart={(e) => !isExecuting && onPaletteDragStart(e, key)}
                          title={config.description}
                          style={{
                            display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6,
                            cursor: isExecuting ? 'not-allowed' : 'grab',
                            opacity: isExecuting ? 0.5 : 1,
                            textAlign: 'center', padding: '4px 0',
                          }}
                        >
                          {cat === 'source' ? (
                            <div style={{
                              width: 52, height: 52, position: 'relative',
                              display: 'flex', alignItems: 'center', justifyContent: 'center',
                              borderRadius: '16px 3px 3px 16px',
                              background: '#1f2937',
                              boxShadow: '0 3px 8px rgba(0,0,0,0.22)',
                              fontSize: 24,
                            }}>
                              <span style={{ filter: `drop-shadow(0 0 4px ${config.color})`, color: config.color }}>{config.icon}</span>
                            </div>
                          ) : cat === 'sync' ? (
                            <div style={{ width: 52, height: 52, position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                              <div style={{
                                width: 36, height: 36,
                                transform: 'rotate(45deg)',
                                background: '#fff',
                                border: `2px solid ${config.color}`,
                                boxShadow: '0 2px 6px rgba(0,0,0,0.08)',
                              }} />
                              <span style={{ position: 'absolute', fontSize: 20, pointerEvents: 'none' }}>{config.icon}</span>
                            </div>
                          ) : cat === 'ops' ? (
                            <div style={{
                              width: 52, height: 52,
                              display: 'flex', alignItems: 'center', justifyContent: 'center',
                              borderRadius: '50%',
                              background: '#fff',
                              border: `2.5px solid ${config.color}`,
                              boxShadow: '0 2px 6px rgba(0,0,0,0.08)',
                              fontSize: 24,
                            }}>
                              {config.icon}
                            </div>
                          ) : (
                            <div style={{
                              width: 52, height: 52,
                              borderRadius: 12,
                              background: '#fff',
                              border: `1.5px solid ${config.color}`,
                              boxShadow: '0 2px 6px rgba(0,0,0,0.06)',
                              display: 'flex', alignItems: 'center', justifyContent: 'center',
                              fontSize: 24,
                            }}>
                              {config.icon}
                            </div>
                          )}
                          <span style={{ fontSize: 11, fontWeight: 500, lineHeight: 1.2, color: '#434343' }}>
                            {config.label}
                          </span>
                        </div>
                      )
                    })}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      </div>

      {/* 底部控制台 / 历史面板 */}
      {consoleMode === 'hidden' ? (
        <div
          onClick={handleOpenHistory}
          style={{
            flexShrink: 0,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '6px 16px',
            margin: '0 16px 8px',
            background: '#fafafa',
            border: '1px solid #f0f0f0',
            borderRadius: 8,
            cursor: 'pointer',
          }}
        >
          <Space>
            <HistoryOutlined />
            <span style={{ fontWeight: 500 }}>日志 / 历史记录</span>
            {isExecuting && <Tag color="processing" icon={<SyncOutlined spin />}>执行中</Tag>}
          </Space>
          <span style={{ color: '#999', fontSize: 12 }}>点击展开 ▲</span>
        </div>
      ) : (
        <div
          style={{
            flexShrink: 0,
            height: 300,
            margin: '0 16px 8px',
            border: '1px solid #f0f0f0',
            borderRadius: 8,
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
            background: '#fff',
          }}
        >
          {/* 面板头部 */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '6px 12px',
              borderBottom: '1px solid #f0f0f0',
              background: '#fafafa',
            }}
          >
            <Space size={8}>
              <span style={{ fontWeight: 600 }}>
                {consoleMode === 'console' ? '控制台' : '历史记录'}
              </span>
              {consoleMode === 'console' && currentExecution && (
                <Tag
                  color={getExecStatusConfig(currentExecution.status).color}
                  icon={getExecStatusConfig(currentExecution.status).icon}
                >
                  {getExecStatusConfig(currentExecution.status).label}
                </Tag>
              )}
              {consoleMode === 'console' && currentExecution && (
                <span style={{ color: '#999', fontSize: 12 }}>执行 #{currentExecution.id}</span>
              )}
            </Space>
            <Space size={4}>
              {consoleMode === 'console' ? (
                <>
                  <Button size="small" type="text" icon={<HistoryOutlined />} onClick={handleOpenHistory}>
                    历史
                  </Button>
                  <Button
                    size="small"
                    type="text"
                    icon={<ReloadOutlined />}
                    disabled={isExecuting}
                    onClick={() => {
                      setNodeLogs([])
                      setCrawlerLogs([])
                      setCurrentExecution(null)
                      currentExecIdRef.current = null
                      if (id) clearConsoleCache(id)
                    }}
                  >
                    清空
                  </Button>
                </>
              ) : (
                <>
                  {currentExecution && (
                    <Button size="small" type="text" icon={<UnlockOutlined />} onClick={() => setConsoleMode('console')}>
                      返回实时
                    </Button>
                  )}
                  <Button
                    size="small"
                    type="text"
                    icon={<ReloadOutlined />}
                    loading={historyLoading}
                    onClick={refreshHistory}
                  >
                    刷新
                  </Button>
                </>
              )}
              <Tooltip title={consoleMode === 'console' ? '关闭（查看历史记录）' : '收起面板'}>
                <Button size="small" type="text" icon={<CloseOutlined />} onClick={handleCloseConsole} />
              </Tooltip>
            </Space>
          </div>

          {/* 面板内容 */}
          <div ref={panelContentRef} style={{ flex: 1, minHeight: 0, overflow: 'auto' }}>
            {consoleMode === 'console' ? (
              <div
                style={{
                  minHeight: '100%',
                  background: '#1e1e1e',
                  padding: 12,
                  fontFamily: 'Consolas, Monaco, monospace',
                  fontSize: 12,
                  lineHeight: 1.7,
                }}
              >
                {executionPlan.length > 0 && (
                  <div style={{ color: '#888', marginBottom: 8, whiteSpace: 'pre-wrap' }}>
                    执行计划：{executionPlan.map((n) => nodeDisplayLabel(n.data)).join('  →  ')}
                  </div>
                )}
                {consoleLines.length === 0 ? (
                  <span style={{ color: '#666' }}>
                    暂无执行日志，点击右上角「执行」开始运行工作流
                  </span>
                ) : (
                  consoleLines.map((l) => (
                    <div key={l.key}>
                      {l.time && <span style={{ color: '#6a9955', marginRight: 8 }}>{l.time}</span>}
                      <Tag
                        color={consoleLevelColor(l.level)}
                        style={{ marginRight: 6, minWidth: 44, textAlign: 'center', fontSize: 10, padding: '0 4px', lineHeight: '16px' }}
                      >
                        {l.level.toUpperCase()}
                      </Tag>
                      {l.tag && <span style={{ color: '#569cd6', marginRight: 8 }}>[{l.tag}]</span>}
                      <span style={{ color: '#e0e0e0' }}>{l.message}</span>
                    </div>
                  ))
                )}
                <div ref={consoleEndRef} />
              </div>
            ) : history.length === 0 ? (
              <Empty
                style={{ marginTop: 40 }}
                description="暂无执行历史"
                image={Empty.PRESENTED_IMAGE_SIMPLE}
              />
            ) : (
              <Table
                size="small"
                columns={historyColumns}
                dataSource={history}
                rowKey="id"
                loading={historyLoading}
                pagination={false}
                style={{ padding: '0 8px' }}
                onRow={(r) => ({
                  onDoubleClick: () => { if (r.status !== 'running') handleReplayExecution(r) },
                  style: { cursor: r.status !== 'running' ? 'pointer' : 'default' },
                })}
              />
            )}
          </div>
        </div>
      )}

      </div>{/* /内容区 flex column */}

      {/* 历史执行详情 */}
      <Modal
        title={`执行详情 #${detailExecution?.id}`}
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={760}
      >
        {detailLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}>
            <Spin size="large" />
          </div>
        ) : (
          <div>
            <Card size="small" style={{ marginBottom: 16 }}>
              <Space direction="vertical" style={{ width: '100%' }}>
                <div>
                  <strong>状态：</strong>
                  {detailExecution && (
                    <Tag
                      color={getExecStatusConfig(detailExecution.status).color}
                      icon={getExecStatusConfig(detailExecution.status).icon}
                    >
                      {getExecStatusConfig(detailExecution.status).label}
                    </Tag>
                  )}
                </div>
                <div>
                  <strong>开始时间：</strong>
                  {detailExecution?.startedAt?.replace('T', ' ').slice(0, 19)}
                </div>
                <div>
                  <strong>结束时间：</strong>
                  {detailExecution?.finishedAt
                    ? detailExecution.finishedAt.replace('T', ' ').slice(0, 19)
                    : '运行中'}
                </div>
                {detailExecution?.errorMsg && (
                  <div>
                    <strong>错误信息：</strong>
                    <div
                      style={{
                        color: '#ff4d4f',
                        marginTop: 8,
                        padding: 8,
                        background: '#fff2f0',
                        borderRadius: 4,
                      }}
                    >
                      {detailExecution.errorMsg}
                    </div>
                  </div>
                )}
              </Space>
            </Card>

            {/* 分析报告下载区 */}
            {detailLogs.filter(l => l.output?.reportId).map(l => (
              <Alert
                key={l.output!.reportId}
                style={{ marginBottom: 12 }}
                icon={<FileTextOutlined />}
                showIcon
                type="success"
                message={`AI 分析报告已生成（${l.output!.reportFormat || 'markdown'} 格式）`}
                description={`ID: ${String(l.output!.reportId).slice(0, 8)}...`}
                action={
                  <Space direction="vertical" size={4}>
                    <Button
                      type="primary"
                      size="small"
                      icon={<DownloadOutlined />}
                      onClick={() => downloadReport(l.output!.reportId, l.output!.reportFormat)}
                    >
                      下载报告
                    </Button>
                    <Button
                      size="small"
                      icon={<ReloadOutlined />}
                      loading={regenerating}
                      onClick={handleRegenerateReport}
                    >
                      重新生成
                    </Button>
                  </Space>
                }
              />
            ))}

            <Card size="small" title="节点执行计划与日志">
              {detailLogs.length === 0 ? (
                <Empty description="暂无节点执行日志" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              ) : (
                <Timeline
                  items={detailLogs.map((log) => {
                    const c = getExecStatusConfig(log.status)
                    const data = nodes.find((n) => n.id === log.nodeId)?.data
                    const label = data ? nodeDisplayLabel(data) : log.nodeId
                    const summary = summarizeNodeOutput(data?.type, log.output)
                    return {
                      color: c.color === 'processing' ? 'blue' : c.color === 'success' ? 'green' : c.color === 'error' ? 'red' : 'gray',
                      dot: c.icon,
                      children: (
                        <div>
                          <div style={{ marginBottom: 4 }}>
                            <strong>{label}</strong>
                            <Tag color={c.color} style={{ marginLeft: 8 }}>
                              {c.label}
                            </Tag>
                          </div>
                          {summary && (log.status === 'success' || log.status === 'partial_success') && (
                            <div style={{ marginTop: 4, marginBottom: 4, fontSize: 13, color: '#262626' }}>
                              处理结果：{summary}
                            </div>
                          )}
                          <div style={{ fontSize: 12, color: '#666' }}>
                            开始: {log.startedAt?.replace('T', ' ').slice(0, 19)}
                            {log.finishedAt && ` ｜ 结束: ${log.finishedAt.replace('T', ' ').slice(0, 19)}`}
                          </div>
                          {log.errorMsg && (
                            <div style={{ marginTop: 6, color: '#ff4d4f', fontSize: 12 }}>
                              错误: {log.errorMsg}
                            </div>
                          )}
                          {detailExecution && detailExecution.status !== 'running' && (
                            <Button
                              type="link"
                              size="small"
                              icon={<PlayCircleOutlined />}
                              style={{ padding: '2px 0', marginTop: 4 }}
                              onClick={() => handleExecuteFromNode(log.nodeId, detailExecution.id)}
                            >
                              从此节点重跑
                            </Button>
                          )}
                        </div>
                      ),
                    }
                  })}
                />
              )}
              <div style={{ marginTop: 8, fontSize: 12, color: '#999' }}>
                注：爬虫节点的实时输出仅在执行时于控制台展示，不做持久化保存。
              </div>
            </Card>
          </div>
        )}
      </Modal>

      {/* 节点配置抽屉 */}
      <Drawer
        title={`配置节点: ${currentNodeType?.label}`}
        placement="right"
        onClose={() => setDrawerVisible(false)}
        open={drawerVisible}
        width={400}
        extra={
          isExecuting ? (
            <Tag color="processing">执行中（只读）</Tag>
          ) : (
            <Space>
              <Button danger onClick={handleDeleteNode}>
                删除节点
              </Button>
              <Button type="primary" onClick={handleNodeConfigSave}>
                保存
              </Button>
            </Space>
          )
        }
      >
        {selectedNode && (
          <Form form={nodeConfigForm} layout="vertical" disabled={isLocked}>
            <Form.Item label="节点 ID">
              <Input value={selectedNode.id} disabled />
            </Form.Item>

            <Form.Item
              label="节点标签"
              name="label"
              rules={[{ required: true, message: '请输入节点标签' }]}
            >
              <Input placeholder="节点显示名称" />
            </Form.Item>

            {currentNodeType?.configSchema.map((field) => {
              const fieldConfig = field as any
              const isNumberField = field.type === 'number'
              const isBooleanField = field.type === 'boolean'
              const isTagsField = field.type === 'tags'
              const isSelectMultiple = field.type === 'select-multiple'
              const isSelect = field.type === 'select'
              const isTextArea = field.type === 'textarea'

              // showIf 条件渲染：支持 { field, value } 或 { field, values[] }
              if (fieldConfig.showIf) {
                const { field: depField, value: depValue, values: depValues } = fieldConfig.showIf
                const watchedValues: Record<string, any> = {
                  crawlerType: watchedNodeCrawlerType,
                  format: watchedNodeFormat,
                }
                const cur = watchedValues[depField]
                if (depValues !== undefined) {
                  if (!depValues.includes(cur) && !depValues.includes(cur ?? '')) return null
                } else if (depValue !== undefined) {
                  if (cur !== depValue) return null
                }
              }

              // 分组标题
              if (field.type === 'group-label') {
                return (
                  <div key={fieldConfig.name} style={{ fontSize: 12, fontWeight: 600, color: '#8c8c8c', letterSpacing: 1, marginBottom: 4, marginTop: 8, borderBottom: '1px solid #f0f0f0', paddingBottom: 4 }}>
                    {fieldConfig.label}
                  </div>
                )
              }

              // 一排两个数字输入
              if (field.type === 'number-pair') {
                return (
                  <div key={fieldConfig.name} style={{ display: 'flex', gap: 12 }}>
                    {(fieldConfig.items as any[]).map((item: any) => (
                      <Form.Item
                        key={item.name}
                        name={item.name}
                        label={item.label}
                        initialValue={item.default}
                        style={{ flex: 1, marginBottom: 16 }}
                      >
                        <InputNumber min={item.min} max={item.max} placeholder={item.placeholder} style={{ width: '100%' }} />
                      </Form.Item>
                    ))}
                  </div>
                )
              }

              // 双开关一行（boolean-pair）
              if (field.type === 'boolean-pair') {
                return (
                  <div key={fieldConfig.name} style={{ display: 'flex', gap: 16 }}>
                    {(fieldConfig.items as any[]).map((item: any) => (
                      <Form.Item
                        key={item.name}
                        name={item.name}
                        label={item.label}
                        valuePropName="checked"
                        style={{ flex: 1, marginBottom: 16 }}
                      >
                        <Switch />
                      </Form.Item>
                    ))}
                  </div>
                )
              }

              // 告警规则多选（选项来自现有规则，留空=全部启用规则）
              if (field.type === 'alert-rules-select') {
                return (
                  <Form.Item
                    key={field.name}
                    label={field.label}
                    name={field.name}
                    tooltip="留空则评估全部启用规则"
                  >
                    <Select
                      mode="multiple"
                      allowClear
                      placeholder="留空 = 全部启用规则"
                      style={{ width: '100%' }}
                      options={alertRules.map((r) => ({ label: r.name, value: r.id }))}
                      optionFilterProp="label"
                    />
                  </Form.Item>
                )
              }

              // 条件规则表格（field / operator / value 多行）
              if (field.type === 'condition-rules') {
                return (
                  <Form.Item key={field.name} label={field.label}>
                    <Form.List name={field.name}>
                      {(rows, { add, remove }) => (
                        <div>
                          {rows.map(({ key, name, ...restField }) => (
                            <Space key={key} align="baseline" style={{ display: 'flex', marginBottom: 8 }}>
                              <Form.Item
                                {...restField}
                                name={[name, 'field']}
                                rules={[{ required: true, message: '字段' }]}
                                style={{ marginBottom: 0 }}
                              >
                                <Select
                                  showSearch
                                  optionFilterProp="label"
                                  options={CONDITION_FIELD_SUGGESTIONS}
                                  placeholder="选择字段"
                                  style={{ width: 160 }}
                                />
                              </Form.Item>
                              <Form.Item {...restField} name={[name, 'op']} initialValue=">" style={{ marginBottom: 0 }}>
                                <Select style={{ width: 70 }} options={CONDITION_OPERATORS} />
                              </Form.Item>
                              <Form.Item
                                {...restField}
                                name={[name, 'value']}
                                rules={[{ required: true, message: '值' }]}
                                style={{ marginBottom: 0 }}
                              >
                                <InputNumber placeholder="数值" style={{ width: 110 }} />
                              </Form.Item>
                              <MinusCircleOutlined onClick={() => remove(name)} />
                            </Space>
                          ))}
                          <Button type="dashed" onClick={() => add({ op: '>' })} block icon={<PlusOutlined />}>
                            添加条件
                          </Button>
                          <div style={{ fontSize: 12, color: '#999', marginTop: 6 }}>
                            不填任何条件时，默认判断「上游是否有文章」
                          </div>
                        </div>
                      )}
                    </Form.List>
                  </Form.Item>
                )
              }

              return (
                <Form.Item
                  key={field.name}
                  label={field.label}
                  name={field.name}
                  rules={[{ required: field.required, message: `请输入${field.label}` }]}
                  valuePropName={isBooleanField ? 'checked' : 'value'}
                  tooltip={
                    field.name === 'maxNotesCount' ? `当前后台配置的最大爬取上限为 ${crawlerMaxLimit} 篇` :
                    field.name === 'maxCommentsCount' ? `当前后台配置的最大一级评论上限为 ${crawlerMaxCommentsLimit} 条` :
                    field.name === 'maxSubCommentsCount' ? `当前后台配置的最大二级评论上限为 ${crawlerMaxSubCommentsLimit} 条` :
                    undefined
                  }
                  extra={
                    field.name === 'maxNotesCount' && nodeConfigForm.getFieldValue('maxNotesCount') > crawlerMaxLimit ? (
                      <span style={{ color: '#ff4d4f' }}>⚠️ 超出后台管理配置的最大上限（{crawlerMaxLimit} 篇），将被限制为 {crawlerMaxLimit} 篇</span>
                    ) : field.name === 'maxCommentsCount' && nodeConfigForm.getFieldValue('maxCommentsCount') > crawlerMaxCommentsLimit ? (
                      <span style={{ color: '#ff4d4f' }}>⚠️ 超出后台管理配置的最大上限（{crawlerMaxCommentsLimit} 条），将被限制为 {crawlerMaxCommentsLimit} 条</span>
                    ) : field.name === 'maxSubCommentsCount' && nodeConfigForm.getFieldValue('maxSubCommentsCount') > crawlerMaxSubCommentsLimit ? (
                      <span style={{ color: '#ff4d4f' }}>⚠️ 超出后台管理配置的最大上限（{crawlerMaxSubCommentsLimit} 条），将被限制为 {crawlerMaxSubCommentsLimit} 条</span>
                    ) : undefined
                  }
                >
                  {isNumberField ? (
                    <InputNumber
                      min={fieldConfig.min}
                      max={
                        field.name === 'maxNotesCount' ? crawlerMaxLimit :
                        field.name === 'maxCommentsCount' ? crawlerMaxCommentsLimit :
                        field.name === 'maxSubCommentsCount' ? crawlerMaxSubCommentsLimit :
                        fieldConfig.max
                      }
                      placeholder={
                        field.name === 'maxNotesCount' ? `留空则使用后台管理的默认值（上限 ${crawlerMaxLimit} 篇）` :
                        field.name === 'maxCommentsCount' ? `留空则使用后台管理的默认值（上限 ${crawlerMaxCommentsLimit} 条）` :
                        field.name === 'maxSubCommentsCount' ? `留空则使用后台管理的默认值（上限 ${crawlerMaxSubCommentsLimit} 条）` :
                        fieldConfig.placeholder
                      }
                      style={{ width: '100%' }}
                    />
                  ) : isBooleanField ? (
                    <Switch />
                  ) : isTagsField ? (
                    <Select
                      mode="tags"
                      style={{ width: '100%' }}
                      placeholder={fieldConfig.placeholder}
                      tokenSeparators={[',']}
                      options={fieldConfig.name === 'topics' ? topicOptions : undefined}
                      filterOption={(input, option) =>
                        (option?.label as string)?.toLowerCase().includes(input.toLowerCase())
                      }
                    />
                  ) : isSelectMultiple ? (
                    <Select
                      mode="multiple"
                      style={{ width: '100%' }}
                      placeholder={fieldConfig.placeholder}
                      options={fieldConfig.options}
                    />
                  ) : isSelect ? (
                    <Select
                      style={{ width: '100%' }}
                      placeholder={fieldConfig.placeholder}
                      options={fieldConfig.options}
                    />
                  ) : isTextArea ? (
                    <TextArea rows={4} placeholder={fieldConfig.placeholder} />
                  ) : (
                    <Input placeholder={fieldConfig.placeholder} />
                  )}
                </Form.Item>
              )
            })}
          </Form>
        )}
      </Drawer>
    </div>
  )
}

export default WorkflowEditorPage

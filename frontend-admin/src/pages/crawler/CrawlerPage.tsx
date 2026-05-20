import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button, Card, DatePicker, Form, InputNumber, Modal, Progress, Select, Space, Switch, Table, Tag,
  Tooltip, Typography, message,
} from 'antd'
import {
  CloudSyncOutlined, FilterOutlined, PlayCircleOutlined, ReloadOutlined, SaveOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs, { Dayjs } from 'dayjs'
import { crawlerApi } from '@/api/crawler'
import type { CrawlerRunFilter, CrawlerRunLog, CrawlerRunProgress, CrawlerSpiderConfig } from '@/types'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'

const { Paragraph, Text } = Typography
const { RangePicker } = DatePicker

const SPIDER_OPTIONS = [
  { label: '新闻收集 + AI关键词提取', value: 'broad-topic' },
  { label: '社交平台深度爬取', value: 'deep-sentiment' },
]

type AdvancedFormValues = {
  spiders: string[]
  keywords?: string[]
  topics?: string[]
  range?: [Dayjs, Dayjs] | null
}

function parseParams(raw: string): CrawlerRunFilter | null {
  if (!raw) return null
  try {
    const v = JSON.parse(raw) as CrawlerRunFilter
    return v && typeof v === 'object' ? v : null
  } catch { return null }
}

function summariseParams(p: CrawlerRunFilter | null): React.ReactNode {
  if (!p) return <Text type="secondary">—</Text>
  const parts: string[] = []
  if (p.keywords?.length) parts.push(`关键词:${p.keywords.join('/')}`)
  if (p.topics?.length) parts.push(`话题:${p.topics.join('/')}`)
  if (p.startAt || p.endAt) {
    const fmt = (s?: string) => (s ? dayjs(s).format('YYYY-MM-DD HH:mm') : '∞')
    parts.push(`时间:${fmt(p.startAt)} ~ ${fmt(p.endAt)}`)
  }
  if (!parts.length) return <Text type="secondary">—</Text>
  const text = parts.join('  ')
  return (
    <Tooltip title={text}>
      <Text ellipsis style={{ maxWidth: 280, display: 'inline-block' }}>{text}</Text>
    </Tooltip>
  )
}

function formatProgressDetail(detail: CrawlerRunProgress['detail'], raw?: string): string {
  let d = detail
  if (!d && raw) { try { d = JSON.parse(raw) as CrawlerRunProgress['detail'] } catch { return raw.length > 200 ? `${raw.slice(0, 200)}…` : raw } }
  if (!d) return '准备中…'
  const parts: string[] = []
  const phaseMap: Record<string, string> = {
    queued: '排队等待', starting: '正在启动', running: '运行中',
    collecting_news: '收集新闻', syncing_articles: '同步文章', extracting_keywords: 'AI提取关键词',
    deep_sentiment_starting: '深度爬取启动', deep_sentiment_done: '深度爬取完成',
    deep_sentiment_failed: '深度爬取失败', done: '已完成', closing: '收尾…',
    finished: '已完成', failed: '失败',
  }
  if (d.phase) parts.push(`阶段: ${phaseMap[d.phase] ?? d.phase}`)
  if (d.totalSpiders != null) parts.push(`计划: ${d.totalSpiders}`)
  if (d.currentSpider) parts.push(`当前: ${d.currentSpider}`)
  if (d.completedSpiders?.length) parts.push(`已完成: ${d.completedSpiders.join(', ')}`)
  if (d.itemsInSpider != null) parts.push(`条目: ${d.itemsInSpider}`)
  return parts.length ? parts.join(' · ') : '运行中…'
}

const CrawlerPage: React.FC = () => {
  const [spiders, setSpiders] = useState<CrawlerSpiderConfig[]>([])
  const [runs, setRuns] = useState<CrawlerRunLog[]>([])
  const [runTotal, setRunTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [pendingRunId, setPendingRunId] = useState<number | null>(null)
  const [activeProgress, setActiveProgress] = useState<CrawlerRunProgress | null>(null)
  const [progressModalId, setProgressModalId] = useState<number | null>(null)
  const [modalProgress, setModalProgress] = useState<CrawlerRunProgress | null>(null)
  const [logModal, setLogModal] = useState<{ title: string; body: string } | null>(null)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [advancedSubmitting, setAdvancedSubmitting] = useState(false)
  const [advancedForm] = Form.useForm<AdvancedFormValues>()
  const [statusFilter, setStatusFilter] = useState<string>('')

  const fetchSpiders = useCallback(async () => {
    setLoading(true)
    try { setSpiders(await crawlerApi.listSpiders()) } finally { setLoading(false) }
  }, [])

  const fetchRuns = useCallback(async (p = 1) => {
    const res = await crawlerApi.listRuns({ page: p, pageSize: 15 })
    setRuns(res.list); setRunTotal(res.total)
  }, [])

  useEffect(() => { void fetchSpiders(); void fetchRuns(1) }, [fetchSpiders, fetchRuns])

  useEffect(() => {
    if (progressModalId == null) { setModalProgress(null); return }
    let cancelled = false
    const tick = async () => {
      try {
        const p = await crawlerApi.getRunProgress(progressModalId)
        if (!cancelled) setModalProgress(p)
        if (p.status !== 'running') void fetchRuns(1)
      } catch { /* ignore */ }
    }
    void tick()
    const timer = window.setInterval(() => void tick(), 1000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [progressModalId, fetchRuns])

  const pollRunUntilDone = useCallback((id: number) => {
    let fullCount = 0
    const tick = async () => {
      try {
        const p = await crawlerApi.getRunProgress(id)
        setActiveProgress(p); await fetchRuns(1)
        if (p.status !== 'running') {
          setPendingRunId(null); setActiveProgress(null)
          if (p.status === 'success') void message.success('爬取任务已完成')
          else void message.warning('爬取任务失败，请查看运行记录')
        } else if (p.progress >= 99) {
          fullCount += 1
          if (fullCount >= 3) { setPendingRunId(null); setActiveProgress(null); void fetchRuns(1); void message.success('爬取任务已完成') }
          else window.setTimeout(() => void tick(), 1500)
        } else { fullCount = 0; window.setTimeout(() => void tick(), 1000) }
      } catch { setPendingRunId(null); setActiveProgress(null); void fetchRuns(1) }
    }
    void tick()
  }, [fetchRuns])

  const handleSave = async () => {
    setSaving(true)
    try {
      const payload = spiders.map((s) => ({ spiderKey: s.spiderKey, intervalMinutes: s.intervalMinutes, enabled: s.enabled }))
      setSpiders(await crawlerApi.putSpiders(payload))
      void message.success('定时配置已保存')
    } finally { setSaving(false) }
  }

  const handleRun = async (keys?: string[]) => {
    try {
      const { id } = await crawlerApi.runNow(keys)
      setPendingRunId(id); void message.info('任务已提交'); pollRunUntilDone(id); void fetchRuns(1)
    } catch { setPendingRunId(null) }
  }

  const handleRetry = async (run: CrawlerRunLog) => {
    try {
      const spiderArr = run.spiders ? run.spiders.split(',').map((s) => s.trim()) : undefined
      const { id } = await crawlerApi.runNow(spiderArr)
      setPendingRunId(id); void message.info('重试任务已提交'); pollRunUntilDone(id); void fetchRuns(1)
    } catch { /* handled */ }
  }

  const handleAdvancedSubmit = async () => {
    const values = await advancedForm.validateFields()
    const keywords = (values.keywords ?? []).map((s) => s.trim()).filter(Boolean)
    const topics = (values.topics ?? []).map((s) => s.trim()).filter(Boolean)
    if (!keywords.length && !topics.length) { void message.warning('至少填写一个关键词或话题'); return }
    setAdvancedSubmitting(true)
    try {
      const [start, end] = values.range ?? [null, null]
      const { id } = await crawlerApi.runAdvanced({
        spiders: values.spiders, keywords, topics,
        startAt: start ? start.toISOString() : undefined,
        endAt: end ? end.toISOString() : undefined,
      })
      setAdvancedOpen(false); setPendingRunId(id)
      void message.info('定向抓取任务已提交'); pollRunUntilDone(id); void fetchRuns(1)
    } finally { setAdvancedSubmitting(false) }
  }

  const updateRow = (key: string, patch: Partial<CrawlerSpiderConfig>) =>
    setSpiders((prev) => prev.map((r) => (r.spiderKey === key ? { ...r, ...patch } : r)))

  const spiderColumns: ColumnsType<CrawlerSpiderConfig> = useMemo(() => [
    { title: '名称', dataIndex: 'displayName', width: 160 },
    { title: '标识', dataIndex: 'spiderKey', width: 120 },
    {
      title: '间隔（分钟）', dataIndex: 'intervalMinutes', width: 160,
      render: (_, r) => (
        <InputNumber min={1} max={10080} value={r.intervalMinutes}
          onChange={(v) => updateRow(r.spiderKey, { intervalMinutes: typeof v === 'number' ? v : r.intervalMinutes })} />
      ),
    },
    {
      title: '启用定时', dataIndex: 'enabled', width: 100,
      render: (_, r) => (
        <Switch checked={r.enabled === 1} onChange={(c) => updateRow(r.spiderKey, { enabled: c ? 1 : 0 })} />
      ),
    },
    {
      title: '立即执行', key: 'run', width: 100,
      render: (_, r) => (
        <Button type="link" size="small" icon={<PlayCircleOutlined />}
          disabled={pendingRunId !== null} loading={pendingRunId !== null}
          onClick={() => void handleRun([r.spiderKey])}>运行</Button>
      ),
    },
  ], [pendingRunId])

  const filteredRuns = statusFilter ? runs.filter((r) => r.status === statusFilter) : runs

  const runColumns: ColumnsType<CrawlerRunLog> = [
    { title: 'ID', dataIndex: 'id', width: 65 },
    {
      title: '类型', dataIndex: 'mode', width: 80,
      render: (m: string) => m === 'advanced' ? <Tag color="purple">定向</Tag> : <Tag color="blue">基础</Tag>,
    },
    { title: '爬虫', dataIndex: 'spiders', width: 160, ellipsis: true },
    { title: '过滤条件', dataIndex: 'params', render: (raw: string) => summariseParams(parseParams(raw)) },
    {
      title: '进度', key: 'progress', width: 180,
      render: (_, r) => (
        <Space size={4}>
          <Progress percent={Number(r.progress ?? 0)} size="small" style={{ width: 80 }}
            status={r.status === 'running' ? 'active' : 'normal'} />
          {r.status === 'running' && (
            <Button type="link" size="small" onClick={() => setProgressModalId(r.id)}>查询</Button>
          )}
        </Space>
      ),
    },
    {
      title: '状态', dataIndex: 'status', width: 90,
      render: (s: string) => {
        const color = s === 'success' ? 'green' : s === 'failed' ? 'red' : 'processing'
        const label = s === 'running' ? '运行中' : s === 'success' ? '成功' : '失败'
        return <Tag color={color}>{label}</Tag>
      },
    },
    {
      title: '开始', dataIndex: 'startedAt', width: 150,
      render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss'),
    },
    {
      title: '操作', key: 'ops', width: 120,
      render: (_, r) => (
        <Space size={0}>
          {r.status === 'failed' && (
            <Tooltip title={r.message ? r.message.slice(0, 300) : '无日志'}>
              <Button type="link" size="small" danger
                disabled={pendingRunId !== null}
                onClick={() => void handleRetry(r)}>重试</Button>
            </Tooltip>
          )}
          <Button type="link" size="small" disabled={!r.message}
            onClick={() => setLogModal({ title: `任务 #${r.id}`, body: r.message })}>日志</Button>
        </Space>
      ),
    },
  ]

  return (
    <div className={ui.pageShell}>
      <PageHeader
        title="爬虫运维"
        subtitle="管理定时爬虫配置、立即执行与运行记录"
        icon={<CloudSyncOutlined />}
      />

      <Alert type="info" showIcon className={ui.infoBanner}
        message="定时任务由本机 scheduler.py 执行，间隔来自数据库；「立即执行」由后端拉起子进程（需配置 Python venv）。" />

      {pendingRunId !== null && activeProgress && (
        <Card bordered={false} className={ui.panelCard} title={`任务 #${activeProgress.id} 进度`}
          extra={<Button size="small" onClick={() => { setPendingRunId(null); setActiveProgress(null) }}>取消等待</Button>}>
          <Progress percent={activeProgress.progress}
            status={activeProgress.status === 'running' ? 'active' : activeProgress.status === 'failed' ? 'exception' : 'success'} />
          <Paragraph type="secondary" style={{ marginBottom: 0, marginTop: 8 }}>
            {formatProgressDetail(activeProgress.detail, activeProgress.progressDetail)}
          </Paragraph>
        </Card>
      )}

      <Space style={{ marginBottom: 12 }} wrap>
        <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={() => void handleSave()}>保存定时配置</Button>
        <Button icon={<PlayCircleOutlined />} disabled={pendingRunId !== null} loading={pendingRunId !== null} onClick={() => void handleRun()}>立即运行全部</Button>
        <Button icon={<FilterOutlined />} disabled={pendingRunId !== null} onClick={() => { advancedForm.resetFields(); advancedForm.setFieldsValue({ spiders: ['broad-topic'] }); setAdvancedOpen(true) }}>按关键词抓取…</Button>
        <Button icon={<ReloadOutlined />} onClick={() => { void fetchSpiders(); void fetchRuns(1) }}>刷新</Button>
      </Space>

      <Card bordered={false} className={`${ui.panelCard} ${ui.tableWrap}`} title="爬虫配置">
      <Table<CrawlerSpiderConfig> rowKey="spiderKey" loading={loading} columns={spiderColumns}
        dataSource={spiders} pagination={false} size="middle" />
      </Card>

      <Card bordered={false} className={`${ui.panelCard} ${ui.tableWrap}`}
        title="运行记录"
        extra={
          <Select allowClear placeholder="状态筛选" style={{ width: 140 }} value={statusFilter || undefined}
            onChange={(v) => setStatusFilter(v ?? '')}
            options={[{ label: '成功', value: 'success' }, { label: '失败', value: 'failed' }, { label: '运行中', value: 'running' }]} />
        }>
      <Table<CrawlerRunLog> rowKey="id" columns={runColumns} dataSource={filteredRuns}
        pagination={{ total: runTotal, pageSize: 15, showSizeChanger: false, onChange: (p) => void fetchRuns(p) }}
        size="middle" scroll={{ x: 1200 }}
        rowClassName={(r) => r.status === 'failed' ? 'ant-table-row-error' : ''} />
      </Card>

      {/* 进度 modal */}
      <Modal title={progressModalId != null ? `任务 #${progressModalId} 进度` : ''} open={progressModalId !== null}
        onCancel={() => setProgressModalId(null)} footer={null} width={480} destroyOnClose>
        {modalProgress ? (
          <>
            <Progress percent={modalProgress.progress}
              status={modalProgress.status === 'running' ? 'active' : modalProgress.status === 'failed' ? 'exception' : 'success'} />
            <Paragraph type="secondary" style={{ marginTop: 12 }}>
              {formatProgressDetail(modalProgress.detail, modalProgress.progressDetail)}
            </Paragraph>
          </>
        ) : <Paragraph type="secondary">加载中…</Paragraph>}
      </Modal>

      {/* 高级筛选 modal */}
      <Modal title="按关键词 / 话题定向抓取" open={advancedOpen} onCancel={() => setAdvancedOpen(false)}
        onOk={() => void handleAdvancedSubmit()} confirmLoading={advancedSubmitting} okText="提交" cancelText="取消" width={600} destroyOnClose>
        <Form<AdvancedFormValues> form={advancedForm} layout="vertical" initialValues={{ spiders: ['broad-topic'] }}>
          <Form.Item label="爬虫" name="spiders" rules={[{ required: true, message: '至少选择一个爬虫' }]}>
            <Select mode="multiple" options={SPIDER_OPTIONS} />
          </Form.Item>
          <Form.Item label="关键词" name="keywords">
            <Select mode="tags" tokenSeparators={[',', '，']} placeholder="回车确认" />
          </Form.Item>
          <Form.Item label="话题" name="topics">
            <Select mode="tags" tokenSeparators={[',', '，']} placeholder="回车确认" />
          </Form.Item>
          <Form.Item label="时间范围（可选）" name="range">
            <RangePicker showTime style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 日志 modal */}
      <Modal title={logModal?.title} open={logModal !== null} onCancel={() => setLogModal(null)} footer={null} width={720}>
        <pre style={{ maxHeight: 400, overflow: 'auto', whiteSpace: 'pre-wrap', fontSize: 12 }}>{logModal?.body}</pre>
      </Modal>
    </div>
  )
}

export default CrawlerPage

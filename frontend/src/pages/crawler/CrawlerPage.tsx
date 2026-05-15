import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button, Card, DatePicker, Form, InputNumber, Modal, Progress, Select, Space, Switch, Table, Tag,
  Tooltip, Typography, message,
} from 'antd'
import {
  CloudSyncOutlined, FilterOutlined, PlayCircleOutlined, SaveOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs, { Dayjs } from 'dayjs'
import { crawlerApi } from '@/api/crawler'
import type {
  CrawlerRunFilter, CrawlerRunLog, CrawlerRunProgress, CrawlerSpiderConfig,
} from '@/types'

const { Title, Paragraph, Text } = Typography
const { RangePicker } = DatePicker

const SPIDER_OPTIONS = [
  { label: 'RSS 新闻', value: 'rss' },
  { label: '知乎热榜', value: 'zhihu' },
  { label: '贴吧热榜', value: 'tieba' },
  { label: '搜索（百度新闻）', value: 'search' },
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
  } catch {
    return null
  }
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
  if (parts.length === 0) return <Text type="secondary">—</Text>
  const text = parts.join('  ')
  return (
    <Tooltip title={text}>
      <Text ellipsis style={{ maxWidth: 320, display: 'inline-block' }}>{text}</Text>
    </Tooltip>
  )
}

function formatProgressDetail(
  detail: CrawlerRunProgress['detail'],
  raw?: string,
): string {
  let d = detail
  if (!d && raw) {
    try {
      d = JSON.parse(raw) as CrawlerRunProgress['detail']
    } catch {
      return raw.length > 200 ? `${raw.slice(0, 200)}…` : raw
    }
  }
  if (!d) return '准备中…'
  const parts: string[] = []
  if (d.phase === 'closing') parts.push('阶段: 爬虫已结束，正在收尾…')
  else if (d.phase === 'finished') parts.push('阶段: 已完成')
  else if (d.phase === 'failed') parts.push('阶段: 失败（见运行日志）')
  else if (d.phase) parts.push(`阶段: ${d.phase}`)
  if (d.totalSpiders != null) parts.push(`计划爬虫: ${d.totalSpiders}`)
  if (d.currentSpider) parts.push(`当前: ${d.currentSpider}`)
  if (d.completedSpiders?.length) parts.push(`已完成: ${d.completedSpiders.join(', ')}`)
  if (d.itemsInSpider != null) parts.push(`本段条目: ${d.itemsInSpider}`)
  return parts.length ? parts.join(' · ') : '运行中…'
}

const CrawlerPage: React.FC = () => {
  const [spiders, setSpiders] = useState<CrawlerSpiderConfig[]>([])
  const [runs, setRuns] = useState<CrawlerRunLog[]>([])
  const [runTotal, setRunTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  /** 正在轮询等待完成的「立即执行」任务 id；非空时禁用新的立即执行 */
  const [pendingRunId, setPendingRunId] = useState<number | null>(null)
  /** 当前待完成任务最近一次进度（来自 GET /api/crawler/progress/:id） */
  const [activeProgress, setActiveProgress] = useState<CrawlerRunProgress | null>(null)
  /** 从表格打开「进度详情」弹窗时的任务 id */
  const [progressModalId, setProgressModalId] = useState<number | null>(null)
  const [modalProgress, setModalProgress] = useState<CrawlerRunProgress | null>(null)
  const [logModal, setLogModal] = useState<{ title: string; body: string } | null>(null)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [advancedSubmitting, setAdvancedSubmitting] = useState(false)
  const [advancedForm] = Form.useForm<AdvancedFormValues>()

  const fetchSpiders = useCallback(async () => {
    setLoading(true)
    try {
      const data = await crawlerApi.listSpiders()
      setSpiders(data)
    } finally {
      setLoading(false)
    }
  }, [])

  const fetchRuns = useCallback(async (page = 1) => {
    const res = await crawlerApi.listRuns({ page, pageSize: 15 })
    setRuns(res.list)
    setRunTotal(res.total)
  }, [])

  useEffect(() => {
    void fetchSpiders()
    void fetchRuns(1)
  }, [fetchSpiders, fetchRuns])

  useEffect(() => {
    if (progressModalId == null) {
      setModalProgress(null)
      return
    }
    let cancelled = false
    const tick = async () => {
      try {
        const p = await crawlerApi.getRunProgress(progressModalId)
        if (!cancelled) setModalProgress(p)
        if (p.status !== 'running') {
          void fetchRuns(1)
        }
      } catch {
        /* ignore */
      }
    }
    void tick()
    const timer = window.setInterval(() => void tick(), 1000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [progressModalId, fetchRuns])

  const pollRunUntilDone = useCallback(
    (id: number) => {
      let fullProgressCount = 0
      const tick = async () => {
        try {
          const p = await crawlerApi.getRunProgress(id)
          setActiveProgress(p)
          await fetchRuns(1)
          if (p.status !== 'running') {
            setPendingRunId(null)
            setActiveProgress(null)
            if (p.status === 'success') {
              message.success('爬取任务已完成')
            } else {
              message.warning('爬取任务失败，请查看运行记录中的日志')
            }
          } else if (p.progress >= 99) {
            // Python writes 99% when all spiders close; Go hasn't written final status yet.
            // After a few extra polls with no status change, treat the run as done.
            fullProgressCount += 1
            if (fullProgressCount >= 3) {
              setPendingRunId(null)
              setActiveProgress(null)
              void fetchRuns(1)
              message.success('爬取任务已完成')
            } else {
              window.setTimeout(() => void tick(), 1500)
            }
          } else {
            fullProgressCount = 0
            window.setTimeout(() => void tick(), 1000)
          }
        } catch {
          setPendingRunId(null)
          setActiveProgress(null)
          void fetchRuns(1)
        }
      }
      void tick()
    },
    [fetchRuns],
  )

  const handleSave = async () => {
    setSaving(true)
    try {
      const payload = spiders.map((s) => ({
        spiderKey: s.spiderKey,
        intervalMinutes: s.intervalMinutes,
        enabled: s.enabled,
      }))
      const next = await crawlerApi.putSpiders(payload)
      setSpiders(next)
      message.success('定时配置已保存（长驻 scheduler 约 2 分钟内同步）')
    } finally {
      setSaving(false)
    }
  }

  const handleRun = async (keys?: string[]) => {
    try {
      const { id } = await crawlerApi.runNow(keys)
      setPendingRunId(id)
      message.info('任务已提交，正在后台执行')
      pollRunUntilDone(id)
      void fetchRuns(1)
    } catch {
      setPendingRunId(null)
    }
  }

  const openAdvanced = () => {
    advancedForm.resetFields()
    advancedForm.setFieldsValue({ spiders: ['search'] })
    setAdvancedOpen(true)
  }

  const handleAdvancedSubmit = async () => {
    const values = await advancedForm.validateFields()
    const keywords = (values.keywords ?? []).map((s) => s.trim()).filter(Boolean)
    const topics = (values.topics ?? []).map((s) => s.trim()).filter(Boolean)
    if (keywords.length === 0 && topics.length === 0) {
      message.warning('请至少填写一个关键词或话题')
      return
    }
    setAdvancedSubmitting(true)
    try {
      const [start, end] = values.range ?? [null, null]
      const { id } = await crawlerApi.runAdvanced({
        spiders: values.spiders,
        keywords,
        topics,
        startAt: start ? start.toISOString() : undefined,
        endAt: end ? end.toISOString() : undefined,
      })
      setAdvancedOpen(false)
      setPendingRunId(id)
      message.info('定向抓取任务已提交')
      pollRunUntilDone(id)
      void fetchRuns(1)
    } finally {
      setAdvancedSubmitting(false)
    }
  }

  const updateRow = (key: string, patch: Partial<CrawlerSpiderConfig>) => {
    setSpiders((prev) => prev.map((r) => (r.spiderKey === key ? { ...r, ...patch } : r)))
  }

  const spiderColumns: ColumnsType<CrawlerSpiderConfig> = useMemo(() => [
    { title: '名称', dataIndex: 'displayName', width: 120 },
    { title: '标识', dataIndex: 'spiderKey', width: 100 },
    {
      title: '定时（分钟）',
      dataIndex: 'intervalMinutes',
      width: 160,
      render: (_, r) => (
        <InputNumber
          min={1}
          max={10080}
          value={r.intervalMinutes}
          onChange={(v) => updateRow(r.spiderKey, { intervalMinutes: typeof v === 'number' ? v : r.intervalMinutes })}
        />
      ),
    },
    {
      title: '启用定时',
      dataIndex: 'enabled',
      width: 120,
      render: (_, r) => (
        <Switch
          checked={r.enabled === 1}
          onChange={(checked) => updateRow(r.spiderKey, { enabled: checked ? 1 : 0 })}
        />
      ),
    },
    {
      title: '立即执行',
      key: 'run',
      width: 140,
      render: (_, r) => (
        <Button
          type="link"
          size="small"
          icon={<PlayCircleOutlined />}
          disabled={pendingRunId !== null}
          loading={pendingRunId !== null}
          onClick={() => void handleRun([r.spiderKey])}
        >
          运行
        </Button>
      ),
    },
  ], [pendingRunId])

  const runColumns: ColumnsType<CrawlerRunLog> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    {
      title: '类型',
      dataIndex: 'mode',
      width: 90,
      render: (m: CrawlerRunLog['mode']) =>
        m === 'advanced' ? <Tag color="purple">定向</Tag> : <Tag color="blue">基础</Tag>,
    },
    { title: '爬虫', dataIndex: 'spiders', width: 160, ellipsis: true },
    {
      title: '过滤条件',
      dataIndex: 'params',
      render: (raw: string) => summariseParams(parseParams(raw)),
    },
    {
      title: '进度',
      key: 'progress',
      width: 200,
      render: (_, r) => (
        <Space size={4}>
          <Progress
            percent={Number(r.progress ?? 0)}
            size="small"
            style={{ width: 88 }}
            status={r.status === 'running' ? 'active' : 'normal'}
          />
          {r.status === 'running' && (
            <Button type="link" size="small" onClick={() => setProgressModalId(r.id)}>
              查询
            </Button>
          )}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (s: CrawlerRunLog['status']) => {
        const color = s === 'success' ? 'green' : s === 'failed' ? 'red' : 'processing'
        const label = s === 'running' ? '运行中' : s === 'success' ? '成功' : '失败'
        return <Tag color={color}>{label}</Tag>
      },
    },
    {
      title: '开始时间',
      dataIndex: 'startedAt',
      width: 170,
      render: (t: string) => dayjs(t).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: '结束时间',
      dataIndex: 'finishedAt',
      width: 170,
      render: (t: string | undefined) => (t ? dayjs(t).format('YYYY-MM-DD HH:mm:ss') : '—'),
    },
    {
      title: '日志',
      key: 'log',
      width: 80,
      render: (_, r) => (
        <Button
          type="link"
          size="small"
          disabled={!r.message}
          onClick={() => setLogModal({ title: `任务 #${r.id}`, body: r.message })}
        >
          查看
        </Button>
      ),
    },
  ]

  return (
    <div>
      <Title level={4} style={{ marginTop: 0 }}>
        <CloudSyncOutlined style={{ marginRight: 8 }} />
        爬虫调度
      </Title>
      <Paragraph type="secondary" style={{ marginBottom: 16 }}>
        定时任务由本机常驻进程 <Text code>scheduler.py</Text> 执行，间隔从数据库读取；
        修改下方配置后保存，约 2 分钟内会自动重载。「立即执行」与「按关键词抓取」都由后端拉起一次性子进程
        （需本机已配置 Python 虚拟环境）。
      </Paragraph>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message="单次抓取可能要几分钟；外网慢时最长约 15 分钟会自动结束（见运行日志）。运行中可查看下方进度条，或在表格中点「查询」轮询进度接口。"
      />

      {pendingRunId !== null && activeProgress && (
        <Card
          size="small"
          title={`任务 #${activeProgress.id} 进度`}
          style={{ marginBottom: 16 }}
          extra={
            <Button
              size="small"
              onClick={() => { setPendingRunId(null); setActiveProgress(null) }}
            >
              取消等待
            </Button>
          }
        >
          <Progress
            percent={activeProgress.progress}
            status={
              activeProgress.status === 'running'
                ? 'active'
                : activeProgress.status === 'failed'
                  ? 'exception'
                  : 'success'
            }
          />
          <Paragraph type="secondary" style={{ marginBottom: 0, marginTop: 8 }}>
            {formatProgressDetail(activeProgress.detail, activeProgress.progressDetail)}
          </Paragraph>
        </Card>
      )}

      <Space style={{ marginBottom: 12 }} wrap>
        <Button
          type="primary"
          icon={<SaveOutlined />}
          loading={saving}
          onClick={() => void handleSave()}
        >
          保存定时配置
        </Button>
        <Button
          icon={<PlayCircleOutlined />}
          disabled={pendingRunId !== null}
          loading={pendingRunId !== null}
          onClick={() => void handleRun()}
        >
          立即运行全部（基础）
        </Button>
        <Button
          icon={<FilterOutlined />}
          disabled={pendingRunId !== null}
          onClick={openAdvanced}
        >
          按关键词抓取…
        </Button>
        <Button onClick={() => { void fetchSpiders(); void fetchRuns(1) }}>刷新</Button>
      </Space>

      <Table<CrawlerSpiderConfig>
        rowKey="spiderKey"
        loading={loading}
        columns={spiderColumns}
        dataSource={spiders}
        pagination={false}
        size="middle"
        style={{ marginBottom: 32 }}
      />

      <Title level={5}>最近运行记录</Title>
      <Table<CrawlerRunLog>
        rowKey="id"
        columns={runColumns}
        dataSource={runs}
        pagination={{
          total: runTotal,
          pageSize: 15,
          showSizeChanger: false,
          onChange: (p) => void fetchRuns(p),
        }}
        size="middle"
        scroll={{ x: 1300 }}
      />

      <Modal
        title={progressModalId != null ? `任务 #${progressModalId} 进度` : '进度'}
        open={progressModalId !== null}
        onCancel={() => setProgressModalId(null)}
        footer={null}
        width={520}
        destroyOnClose
      >
        {modalProgress ? (
          <>
            <Progress
              percent={modalProgress.progress}
              status={
                modalProgress.status === 'running'
                  ? 'active'
                  : modalProgress.status === 'failed'
                    ? 'exception'
                    : 'success'
              }
            />
            <Paragraph type="secondary" style={{ marginBottom: 0, marginTop: 12 }}>
              {formatProgressDetail(modalProgress.detail, modalProgress.progressDetail)}
            </Paragraph>
          </>
        ) : (
          <Paragraph type="secondary">加载中…</Paragraph>
        )}
      </Modal>

      <Modal
        title="按关键词 / 话题定向抓取"
        open={advancedOpen}
        onCancel={() => setAdvancedOpen(false)}
        onOk={() => void handleAdvancedSubmit()}
        confirmLoading={advancedSubmitting}
        okText="提交"
        cancelText="取消"
        width={640}
        destroyOnClose
      >
        <Paragraph type="secondary" style={{ marginTop: 0 }}>
          「搜索」爬虫会按关键词调用百度新闻搜索；其余爬虫拉取自己源后，再按关键词 / 时间范围过滤入库。
          至少填写一个关键词或话题。
        </Paragraph>
        <Form<AdvancedFormValues>
          form={advancedForm}
          layout="vertical"
          initialValues={{ spiders: ['search'] }}
        >
          <Form.Item label="爬虫" name="spiders" rules={[{ required: true, message: '至少选择一个爬虫' }]}>
            <Select
              mode="multiple"
              options={SPIDER_OPTIONS}
              placeholder="选择参与本次抓取的爬虫"
            />
          </Form.Item>
          <Form.Item
            label="关键词（回车确认，多关键词为「或」关系）"
            name="keywords"
            tooltip="title/content 命中任一关键词或话题才会入库"
          >
            <Select mode="tags" tokenSeparators={[',', '，']} placeholder="例如：赛尔号" />
          </Form.Item>
          <Form.Item label="话题（与关键词等价，仅作语义区分）" name="topics">
            <Select mode="tags" tokenSeparators={[',', '，']} placeholder="例如：赛尔号" />
          </Form.Item>
          <Form.Item label="发布时间范围（可选）" name="range">
            <RangePicker showTime style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={logModal?.title}
        open={logModal !== null}
        onCancel={() => setLogModal(null)}
        footer={null}
        width={720}
      >
        <pre style={{ maxHeight: 420, overflow: 'auto', whiteSpace: 'pre-wrap', fontSize: 12 }}>
          {logModal?.body}
        </pre>
      </Modal>
    </div>
  )
}

export default CrawlerPage

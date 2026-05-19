import React, { useCallback, useEffect, useState } from 'react'
import {
  Badge,
  Button,
  Card,
  Col,
  Descriptions,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  DatabaseOutlined,
  KeyOutlined,
  ReloadOutlined,
  SaveOutlined,
  SyncOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { adminRagApi } from '@/api/admin-rag'
import { adminSystemApi } from '@/api/admin-system'
import type {
  RagConfig,
  RagStatus,
  RagSyncLog,
  SystemConfigResponse,
  SystemHealth,
  ConfigSnapshot,
  RagSnapshotConfig,
  TaggerSnapshotConfig,
  TaggerConfig,
  UpdateTaggerPayload,
  UpdateRagConfigPayload,
} from '@/types'
import dayjs from 'dayjs'

const { Title, Text } = Typography

// Provider presets ─ any OpenAI-compatible endpoint
const PRESETS = [
  { label: 'DeepSeek',  baseUrl: 'https://api.deepseek.com',                          model: 'deepseek-chat' },
  { label: 'OpenAI',    baseUrl: 'https://api.openai.com',                            model: 'gpt-4o' },
  { label: '百炼/Qwen', baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1', model: 'qwen-plus' },
  { label: 'Kimi',      baseUrl: 'https://api.moonshot.cn/v1',                        model: 'moonshot-v1-8k' },
  { label: '智谱/GLM',  baseUrl: 'https://open.bigmodel.cn/api/paas/v4',              model: 'glm-4-flash' },
]

interface FormValues {
  enabled: boolean
  llmModel: string
  llmBaseUrl: string
  llmApiKey: string
  intervalSeconds: number
  batchSize: number
  maxPerTick: number
}

const EMBED_API_PRESETS = [
  { label: 'OpenAI', baseUrl: 'https://api.openai.com/v1', model: 'text-embedding-3-small' },
  { label: 'Jina', baseUrl: 'https://api.jina.ai/v1', model: 'jina-embeddings-v3' },
  { label: '百炼/Qwen', baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1', model: 'text-embedding-v3' },
  { label: 'DeepSeek', baseUrl: 'https://api.deepseek.com/v1', model: 'deepseek-embedding' },
]

interface RagFormValues {
  embed_provider: 'local' | 'api'
  embed_model: string
  embed_api_base: string
  embed_api_key: string
  chunk_max_chars: number
  chunk_overlap: number
  sync_interval_sec: number
  sync_batch: number
}

const EMBED_PRESETS = [
  { label: '多语言 MiniLM（默认）', model: 'paraphrase-multilingual-MiniLM-L12-v2' },
  { label: 'BGE 中文 small', model: 'BAAI/bge-small-zh-v1.5' },
  { label: 'BGE 中文 base', model: 'BAAI/bge-base-zh-v1.5' },
  { label: 'M3E base', model: 'moka-ai/m3e-base' },
]

const SystemPage: React.FC = () => {
  const [health, setHealth] = useState<SystemHealth | null>(null)
  const [cfg, setCfg] = useState<SystemConfigResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [ragStatus, setRagStatus] = useState<RagStatus | null>(null)
  const [ragRuns, setRagRuns] = useState<RagSyncLog[]>([])
  const [ragTotal, setRagTotal] = useState(0)
  const [ragPage, setRagPage] = useState(1)
  const [ragLoading, setRagLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [ragSyncEnabled, setRagSyncEnabled] = useState<boolean>(true)
  const [syncToggling, setSyncToggling] = useState(false)
  const [ragSaving, setRagSaving] = useState(false)
  const [ragEnvLocks, setRagEnvLocks] = useState<string[]>([])
  const [ragApiKeySet, setRagApiKeySet] = useState(false)
  const [ragHistory, setRagHistory] = useState<ConfigSnapshot[]>([])
  const [ragHistTotal, setRagHistTotal] = useState(0)
  const [ragHistPage, setRagHistPage] = useState(1)
  const [taggerHistory, setTaggerHistory] = useState<ConfigSnapshot[]>([])
  const [taggerHistTotal, setTaggerHistTotal] = useState(0)
  const [taggerHistPage, setTaggerHistPage] = useState(1)
  const [form] = Form.useForm<FormValues>()
  const [ragForm] = Form.useForm<RagFormValues>()
  const ragProvider = Form.useWatch('embed_provider', ragForm) ?? 'local'

  const loadRagHistory = useCallback(async (page: number) => {
    const r = await adminSystemApi.settingHistory({ domain: 'rag', page, pageSize: 8 })
    setRagHistory(r.list)
    setRagHistTotal(r.total)
    setRagHistPage(page)
  }, [])

  const loadTaggerHistory = useCallback(async (page: number) => {
    const r = await adminSystemApi.settingHistory({ domain: 'tagger', page, pageSize: 8 })
    setTaggerHistory(r.list)
    setTaggerHistTotal(r.total)
    setTaggerHistPage(page)
  }, [])

  const refreshRagConfigForm = useCallback(async () => {
    const cfg2 = await adminRagApi.getConfig().catch(() => null)
    if (cfg2 != null) {
      setRagSyncEnabled(cfg2.sync_enabled)
      setRagEnvLocks(cfg2.env_overrides ?? [])
      setRagApiKeySet(cfg2.api_key_set ?? false)
      ragForm.setFieldsValue({
        embed_provider: (cfg2.embed_provider === 'api' ? 'api' : 'local') as 'local' | 'api',
        embed_model: cfg2.embed_model,
        embed_api_base: cfg2.embed_api_base ?? '',
        embed_api_key: '',
        chunk_max_chars: cfg2.chunk_max_chars,
        chunk_overlap: cfg2.chunk_overlap,
        sync_interval_sec: cfg2.sync_interval_sec,
        sync_batch: cfg2.sync_batch,
      })
    }
  }, [ragForm])

  const handleDeleteHistory = async (id: number, reload: () => Promise<void>) => {
    try {
      await adminSystemApi.deleteSettingHistory(id)
      message.success('已删除历史记录')
      await reload()
    } catch (e) {
      console.error(e)
    }
  }

  const handleReapplyHistory = async (id: number, kind: 'rag' | 'tagger') => {
    try {
      const resp = await adminSystemApi.reapplySettingHistory(id)
      if (resp.warning) {
        message.warning(resp.message)
      } else {
        message.success(resp.message)
      }
      if (kind === 'rag') {
        await Promise.all([loadRagHistory(ragHistPage), refreshRagConfigForm()])
        void adminRagApi.status().then(setRagStatus).catch(() => undefined)
      } else {
        await loadTaggerHistory(taggerHistPage)
        const c = await adminSystemApi.config()
        setCfg(c)
        if (c?.tagger) applyToForm(c.tagger)
        void adminSystemApi.health().then(setHealth)
      }
    } catch (e) {
      console.error(e)
    }
  }

  const historyActionColumn = (kind: 'rag' | 'tagger', reload: () => Promise<void>) => ({
    title: '操作',
    key: 'actions',
    width: 100,
    fixed: 'right' as const,
    render: (_: unknown, row: ConfigSnapshot) => (
      <Space size={4} wrap>
        <Button type="link" size="small" onClick={() => void handleReapplyHistory(row.id, kind)}>
          应用
        </Button>
        <Popconfirm
          title="确定删除这条历史记录？"
          onConfirm={() => void handleDeleteHistory(row.id, reload)}
        >
          <Button type="link" size="small" danger>
            删除
          </Button>
        </Popconfirm>
      </Space>
    ),
  })

  const ragSnapshotColumns = [
    {
      title: '嵌入来源',
      width: 100,
      render: (_: unknown, row: ConfigSnapshot) => {
        const c = row.config as RagSnapshotConfig
        return c.embed_provider === 'api'
          ? <Tag color="purple">API</Tag>
          : <Tag color="blue">本地</Tag>
      },
    },
    { title: '模型', width: 160, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).embed_model || '-' },
    { title: 'API URL', width: 200, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).embed_api_base || '-' },
    { title: 'API Key', width: 120, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).embed_api_key || '-' },
    { title: '切块', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).chunk_max_chars },
    { title: '重叠', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).chunk_overlap },
    { title: '同步间隔', width: 88, render: (_: unknown, row: ConfigSnapshot) => `${(row.config as RagSnapshotConfig).sync_interval_sec}s` },
    { title: '批量', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).sync_batch },
    {
      title: '定时同步',
      width: 88,
      render: (_: unknown, row: ConfigSnapshot) => (
        (row.config as RagSnapshotConfig).sync_enabled
          ? <Tag color="success">开</Tag>
          : <Tag>关</Tag>
      ),
    },
    { title: '操作者', width: 88, dataIndex: 'updatedByName', render: (v: string) => v || '-' },
    {
      title: '时间',
      width: 128,
      dataIndex: 'createdAt',
      render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss'),
    },
    historyActionColumn('rag', () => loadRagHistory(ragHistPage)),
  ]

  const taggerSnapshotColumns = [
    {
      title: '启用',
      width: 72,
      render: (_: unknown, row: ConfigSnapshot) => (
        (row.config as TaggerSnapshotConfig).enabled
          ? <Tag color="success">是</Tag>
          : <Tag>否</Tag>
      ),
    },
    { title: '模型', width: 140, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).llm_model || '-' },
    { title: 'API URL', width: 200, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).llm_base_url || '-' },
    { title: 'API Key', width: 120, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).llm_api_key || '-' },
    { title: '轮询间隔', width: 88, render: (_: unknown, row: ConfigSnapshot) => `${(row.config as TaggerSnapshotConfig).interval_seconds}s` },
    { title: '批次', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).batch_size },
    { title: '上限', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).max_per_tick },
    { title: '操作者', width: 88, dataIndex: 'updatedByName', render: (v: string) => v || '-' },
    {
      title: '时间',
      width: 128,
      dataIndex: 'createdAt',
      render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss'),
    },
    historyActionColumn('tagger', () => loadTaggerHistory(taggerHistPage)),
  ]

  const loadRagRuns = useCallback(async (page: number) => {
    setRagLoading(true)
    try {
      const r = await adminRagApi.runs({ page, pageSize: 10 })
      setRagTotal(r.total)
      setRagRuns(r.list)
      setRagPage(page)
    } finally {
      setRagLoading(false)
    }
  }, [])

  const applyToForm = useCallback((t: TaggerConfig) => {
    form.setFieldsValue({
      enabled: t.enabled,
      llmModel: t.llmModel,
      llmBaseUrl: t.llmBaseUrl,
      llmApiKey: '',
      intervalSeconds: t.intervalSeconds,
      batchSize: t.batchSize,
      maxPerTick: t.maxPerTick,
    })
  }, [form])

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      const [h, c, rs] = await Promise.all([
        adminSystemApi.health(),
        adminSystemApi.config(),
        adminRagApi.status().catch(() => null),
      ])
      setHealth(h)
      setCfg(c)
      if (c?.tagger) applyToForm(c.tagger)
      setRagStatus(rs as RagStatus | null)
      const cfg2 = await adminRagApi.getConfig().catch(() => null)
      if (cfg2 != null) {
        setRagSyncEnabled(cfg2.sync_enabled)
        setRagEnvLocks(cfg2.env_overrides ?? [])
        setRagApiKeySet(cfg2.api_key_set ?? false)
        ragForm.setFieldsValue({
          embed_provider: (cfg2.embed_provider === 'api' ? 'api' : 'local') as 'local' | 'api',
          embed_model: cfg2.embed_model,
          embed_api_base: cfg2.embed_api_base ?? '',
          embed_api_key: '',
          chunk_max_chars: cfg2.chunk_max_chars,
          chunk_overlap: cfg2.chunk_overlap,
          sync_interval_sec: cfg2.sync_interval_sec,
          sync_batch: cfg2.sync_batch,
        })
      } else if ((rs as RagStatus | null)?.syncEnabled != null) {
        setRagSyncEnabled((rs as RagStatus).syncEnabled!)
      }
      await loadRagRuns(1)
      await Promise.all([loadRagHistory(1), loadTaggerHistory(1)])
    } finally {
      setLoading(false)
    }
  }, [applyToForm, loadRagHistory, loadRagRuns, loadTaggerHistory, ragForm])

  useEffect(() => { void fetchAll() }, [fetchAll])

  useEffect(() => {
    const id = window.setInterval(() => {
      void loadRagRuns(ragPage)
      void adminRagApi.status().then(setRagStatus).catch(() => undefined)
    }, 12000)
    return () => window.clearInterval(id)
  }, [ragPage, loadRagRuns])

  const handleRagSyncToggle = async (checked: boolean) => {
    setSyncToggling(true)
    try {
      await adminRagApi.updateConfig({ sync_enabled: checked })
      setRagSyncEnabled(checked)
      message.success(checked ? '已启用定时同步' : '已暂停定时同步')
    } catch (e) {
      console.error(e)
      message.error('设置失败')
    } finally {
      setSyncToggling(false)
    }
  }

  const handleSave = async (values: FormValues) => {
    const payload: UpdateTaggerPayload = {
      enabled: values.enabled,
      llmModel: values.llmModel,
      llmBaseUrl: values.llmBaseUrl,
      intervalSeconds: values.intervalSeconds,
      batchSize: values.batchSize,
      maxPerTick: values.maxPerTick,
    }
    const trimmed = (values.llmApiKey ?? '').trim()
    if (trimmed) payload.llmApiKey = trimmed

    setSaving(true)
    try {
      const resp = await adminSystemApi.updateTagger(payload)
      setCfg(resp)
      if (resp?.tagger) applyToForm(resp.tagger)
      message.success('已保存，后台任务下一轮 tick 生效')
      void adminSystemApi.health().then(setHealth)
      void loadTaggerHistory(1)
    } catch (e) {
      console.error(e)
    } finally {
      setSaving(false)
    }
  }

  const applyPreset = (preset: typeof PRESETS[number]) => {
    form.setFieldsValue({ llmBaseUrl: preset.baseUrl, llmModel: preset.model })
  }

  const tagger = cfg?.tagger
  const apiKeyHint = tagger?.apiKeySet
    ? `已配置（${tagger.llmApiKey || '***'}），留空则保留`
    : '尚未配置，必须填写后台任务才能运行'

  const llmProbe = health?.llm

  const handleRagSync = async () => {
    setSyncing(true)
    try {
      await adminRagApi.triggerSync()
      message.success('已提交向量同步，可在下表查看进度')
      await loadRagRuns(ragPage)
      const rs = await adminRagApi.status().catch(() => null)
      setRagStatus(rs as RagStatus | null)
    } finally {
      setSyncing(false)
    }
  }

  const ragFieldLocked = (dbKey: string) => ragEnvLocks.includes(dbKey)

  const ragApiKeyHint = ragApiKeySet
    ? '已配置，留空则保留当前值'
    : '使用第三方 API 时必须填写'

  const handleSaveRagEmbed = async (values: RagFormValues) => {
    setRagSaving(true)
    try {
      const payload: UpdateRagConfigPayload = {
        embed_provider: values.embed_provider,
        embed_model: values.embed_model.trim(),
        chunk_max_chars: values.chunk_max_chars,
        chunk_overlap: values.chunk_overlap,
        sync_interval_sec: values.sync_interval_sec,
        sync_batch: values.sync_batch,
      }
      if (values.embed_provider === 'api') {
        payload.embed_api_base = (values.embed_api_base ?? '').trim()
      }
      const keyTrimmed = (values.embed_api_key ?? '').trim()
      if (keyTrimmed) payload.embed_api_key = keyTrimmed

      const resp = await adminRagApi.updateConfig(payload)
      setRagEnvLocks(resp.env_overrides ?? [])
      setRagApiKeySet(resp.api_key_set ?? ragApiKeySet)
      ragForm.setFieldValue('embed_api_key', '')
      if (resp.warnings?.length) {
        message.warning(resp.warnings.join('；'))
      } else if (resp.warning) {
        message.warning(resp.warning)
      } else if (resp.service_applied === false) {
        message.success('配置已保存到数据库，请重启 RAG 服务后生效')
      } else {
        message.success('Embedding 配置已保存（换模型后建议立即同步）')
      }
      const rs = await adminRagApi.status().catch(() => null)
      setRagStatus(rs as RagStatus | null)
      void loadRagHistory(1)
    } catch (e) {
      console.error(e)
      message.error('保存失败')
    } finally {
      setRagSaving(false)
    }
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4} style={{ marginTop: 0, marginBottom: 0 }}>
          <ThunderboltOutlined style={{ marginRight: 8 }} />系统状态
        </Title>
        <Button icon={<ReloadOutlined />} onClick={() => void fetchAll()} loading={loading}>
          刷新
        </Button>
      </div>

      {loading && !health ? (
        <Spin />
      ) : (
        <>
          <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
            {/* DB probe */}
            <Col xs={24} sm={12} md={6} style={{ display: 'flex' }}>
              <Card size="small" style={{ flex: 1 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <Text strong>数据库 (MySQL)</Text>
                  {health?.database ? (
                    health.database.ok ? (
                      <Tag icon={<CheckCircleOutlined />} color="success">正常 {health.database.latencyMs}ms</Tag>
                    ) : (
                      <Tooltip title={health.database.message}>
                        <Tag icon={<CloseCircleOutlined />} color="error">异常</Tag>
                      </Tooltip>
                    )
                  ) : <Tag>-</Tag>}
                </div>
                {health?.database?.message && !health.database.ok && (
                  <Text type="danger" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
                    {health.database.message}
                  </Text>
                )}
              </Card>
            </Col>

            {/* LLM config summary + probe */}
            <Col xs={24} sm={24} md={12} style={{ display: 'flex' }}>
              <Card
                size="small"
                style={{ flex: 1 }}
                styles={{ body: { padding: '12px 16px' } }}
                extra={
                  llmProbe ? (
                    llmProbe.ok ? (
                      <Tag icon={<CheckCircleOutlined />} color="success">
                        连通 {llmProbe.latencyMs}ms
                      </Tag>
                    ) : (
                      <Tooltip title={llmProbe.message}>
                        <Tag icon={<CloseCircleOutlined />} color="error">异常</Tag>
                      </Tooltip>
                    )
                  ) : <Tag>-</Tag>
                }
                title={<Text strong>大模型 API</Text>}
              >
                {tagger ? (
                  <Descriptions size="small" column={2} style={{ marginBottom: 0 }}>
                    <Descriptions.Item label="模型">
                      <Text code>{tagger.llmModel || '-'}</Text>
                    </Descriptions.Item>
                    <Descriptions.Item label="状态">
                      {tagger.enabled
                        ? <Badge status="processing" text="后台任务运行中" />
                        : <Badge status="default" text="后台任务已停用" />
                      }
                    </Descriptions.Item>
                    <Descriptions.Item label="Base URL" span={2}>
                      <Text type="secondary" style={{ fontSize: 12, wordBreak: 'break-all' }}>
                        {tagger.llmBaseUrl || '-'}
                      </Text>
                    </Descriptions.Item>
                    <Descriptions.Item label="API Key">
                      {tagger.apiKeySet
                        ? <Tag icon={<KeyOutlined />} color="blue">已配置 {tagger.llmApiKey}</Tag>
                        : <Tag color="warning">未配置</Tag>
                      }
                    </Descriptions.Item>
                    <Descriptions.Item label="轮询间隔">
                      {tagger.intervalSeconds}s &nbsp;|&nbsp; 批量 {tagger.batchSize} 条
                    </Descriptions.Item>
                  </Descriptions>
                ) : (
                  <Text type="secondary">加载中…</Text>
                )}
                {llmProbe?.message && !llmProbe.ok && (
                  <Text type="danger" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
                    {llmProbe.message}
                  </Text>
                )}
              </Card>
            </Col>

            {/* Pending tagging */}
            <Col xs={12} sm={12} md={3} style={{ display: 'flex' }}>
              <Card size="small" style={{ flex: 1 }}>
                <Statistic title="待打标文章" value={health?.pendingTagging ?? '-'} suffix="篇" />
              </Card>
            </Col>

            {/* Last crawler run */}
            <Col xs={12} sm={12} md={3} style={{ display: 'flex' }}>
              <Card size="small" style={{ flex: 1 }} title="最近爬取">
                {health?.lastCrawlerRun ? (
                  <div style={{ fontSize: 12 }}>
                    <div><Text type="secondary">{health.lastCrawlerRun.spiders}</Text></div>
                    <div>
                      <Badge
                        status={
                          health.lastCrawlerRun.status === 'success'
                            ? 'success'
                            : health.lastCrawlerRun.status === 'failed'
                              ? 'error'
                              : 'processing'
                        }
                        text={health.lastCrawlerRun.status}
                      />
                    </div>
                    <div style={{ marginTop: 4, color: '#888' }}>
                      {dayjs(health.lastCrawlerRun.startedAt).format('MM-DD HH:mm')}
                    </div>
                  </div>
                ) : (
                  <Text type="secondary">无记录</Text>
                )}
              </Card>
            </Col>
          </Row>

          <Card
            style={{ marginBottom: 24 }}
            title={
              <span>
                <DatabaseOutlined style={{ marginRight: 8 }} />
                向量知识库（RAG 同步）
              </span>
            }
            extra={
              <Space>
                <Space size={4}>
                  <Text style={{ fontSize: 13 }}>定时同步</Text>
                  <Switch
                    size="small"
                    checked={ragSyncEnabled}
                    loading={syncToggling}
                    onChange={(v) => void handleRagSyncToggle(v)}
                    checkedChildren="开"
                    unCheckedChildren="关"
                  />
                </Space>
                <Button
                  type="primary"
                  icon={<SyncOutlined />}
                  loading={syncing}
                  disabled={!ragStatus?.embeddingServiceUrl}
                  onClick={() => void handleRagSync()}
                >
                  立即同步
                </Button>
                <Button size="small" onClick={() => { void loadRagRuns(ragPage) }} loading={ragLoading}>
                  刷新记录
                </Button>
              </Space>
            }
          >
            {ragStatus?.note && (
              <Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 12 }}>
                {ragStatus.note}
              </Text>
            )}
            <Descriptions size="small" column={{ xs: 1, sm: 2, md: 3 }} style={{ marginBottom: 16 }}>
              <Descriptions.Item label="开关">
                {ragStatus?.ragEnabled
                  ? <Tag color="blue">已启用检索</Tag>
                  : <Tag>未启用</Tag>}
              </Descriptions.Item>
              <Descriptions.Item label="句向量来源">
                {ragStatus?.embedProvider === 'api'
                  ? <Tag color="purple">第三方 API</Tag>
                  : <Tag color="blue">本地模型</Tag>}
              </Descriptions.Item>
              <Descriptions.Item label="句向量模型">
                {ragStatus?.embedModel ? <Text code>{ragStatus.embedModel}</Text> : <Text type="secondary">-</Text>}
              </Descriptions.Item>
              <Descriptions.Item label="向量维度">{ragStatus?.embedDim ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="Milvus 集合">{ragStatus?.collection || '-'}</Descriptions.Item>
              <Descriptions.Item label="RAG 服务">
                {ragStatus?.serviceReachable
                  ? <Tag icon={<CheckCircleOutlined />} color="success">可达</Tag>
                  : <Tag icon={<CloseCircleOutlined />} color="warning">不可达或未配置</Tag>}
              </Descriptions.Item>
              <Descriptions.Item label="同步周期（参考）">
                {ragStatus?.syncIntervalSecondsHint != null ? `${ragStatus.syncIntervalSecondsHint}s` : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="服务地址" span={3}>
                <Text type="secondary" style={{ fontSize: 12, wordBreak: 'break-all' }}>
                  {ragStatus?.embeddingServiceUrl || '未配置 config.rag.embedding_service_url'}
                </Text>
              </Descriptions.Item>
              {ragStatus?.serviceError && (
                <Descriptions.Item label="健康检查" span={3}>
                  <Text type="danger" style={{ fontSize: 12 }}>{ragStatus.serviceError}</Text>
                </Descriptions.Item>
              )}
            </Descriptions>

            <Card
              size="small"
              type="inner"
              title="Embedding 配置（句向量模型，非对话 LLM）"
              style={{ marginBottom: 16 }}
              extra={
                <Button
                  type="primary"
                  size="small"
                  icon={<SaveOutlined />}
                  loading={ragSaving}
                  onClick={() => ragForm.submit()}
                >
                  保存
                </Button>
              }
            >
              {ragEnvLocks.length > 0 && (
                <Text type="warning" style={{ display: 'block', fontSize: 12, marginBottom: 12 }}>
                  以下项被 RAG 进程环境变量锁定，后台修改无效：{ragEnvLocks.join('、')}
                </Text>
              )}
              {!ragStatus?.serviceReachable && (
                <Text type="warning" style={{ display: 'block', fontSize: 12, marginBottom: 12 }}>
                  RAG 服务当前不可达，仍可保存配置到数据库；保存后请重启 rag_service 使配置生效。
                </Text>
              )}
              <Form
                form={ragForm}
                layout="vertical"
                onFinish={(v) => void handleSaveRagEmbed(v)}
                initialValues={{
                  embed_provider: 'local',
                  embed_model: 'paraphrase-multilingual-MiniLM-L12-v2',
                  embed_api_base: '',
                  embed_api_key: '',
                  chunk_max_chars: 420,
                  chunk_overlap: 72,
                  sync_interval_sec: 120,
                  sync_batch: 100,
                }}
              >
                <Row gutter={16}>
                  <Col xs={24} md={8}>
                    <Form.Item
                      label="嵌入来源"
                      name="embed_provider"
                      rules={[{ required: true }]}
                      extra="local=本地 Sentence-Transformers；api=OpenAI 兼容 Embedding API"
                    >
                      <Select
                        disabled={ragFieldLocked('rag.embed_provider')}
                        options={[
                          { value: 'local', label: '本地模型（Sentence-Transformers）' },
                          { value: 'api', label: '第三方 API（OpenAI 兼容）' },
                        ]}
                      />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={16}>
                    <Form.Item
                      label={ragProvider === 'api' ? 'Embedding 模型名（API model）' : '句向量模型（HuggingFace id）'}
                      name="embed_model"
                      rules={[{ required: true, message: '请输入模型名' }]}
                      extra="更换模型后请「立即同步」重建向量；若维度变化可能需重建 Milvus 集合"
                    >
                      <Input
                        disabled={ragFieldLocked('rag.embed_model')}
                        placeholder={ragProvider === 'api' ? 'text-embedding-3-small' : 'paraphrase-multilingual-MiniLM-L12-v2'}
                      />
                    </Form.Item>
                  </Col>
                  {ragProvider === 'local' && (
                    <Col xs={24}>
                      <Space wrap style={{ marginBottom: 16 }}>
                        {EMBED_PRESETS.map((p) => (
                          <Button
                            key={p.model}
                            size="small"
                            disabled={ragFieldLocked('rag.embed_model')}
                            onClick={() => ragForm.setFieldValue('embed_model', p.model)}
                          >
                            {p.label}
                          </Button>
                        ))}
                      </Space>
                    </Col>
                  )}
                  {ragProvider === 'api' && (
                    <>
                      <Col xs={24} md={14}>
                        <Form.Item
                          label="Embedding API Base URL"
                          name="embed_api_base"
                          rules={[{ required: true, message: '请输入 API Base URL' }]}
                        >
                          <Input
                            disabled={ragFieldLocked('rag.embed_api_base')}
                            placeholder="https://api.openai.com/v1"
                          />
                        </Form.Item>
                        <Space wrap style={{ marginBottom: 16 }}>
                          {EMBED_API_PRESETS.map((p) => (
                            <Button
                              key={p.label}
                              size="small"
                              disabled={ragFieldLocked('rag.embed_api_base')}
                              onClick={() => ragForm.setFieldsValue({ embed_api_base: p.baseUrl, embed_model: p.model })}
                            >
                              {p.label}
                            </Button>
                          ))}
                        </Space>
                      </Col>
                      <Col xs={24} md={10}>
                        <Form.Item
                          label="Embedding API Key"
                          name="embed_api_key"
                          extra={ragApiKeyHint}
                        >
                          <Input.Password
                            disabled={ragFieldLocked('rag.embed_api_key')}
                            autoComplete="new-password"
                            placeholder={ragApiKeySet ? '留空则保留当前值' : 'sk-...'}
                          />
                        </Form.Item>
                      </Col>
                    </>
                  )}
                  <Col xs={24} md={10}>
                    <Form.Item label="切块最大字符" name="chunk_max_chars">
                      <InputNumber
                        min={128}
                        max={2000}
                        style={{ width: '100%' }}
                        disabled={ragFieldLocked('rag.chunk_max_chars')}
                      />
                    </Form.Item>
                    <Form.Item label="切块重叠字符" name="chunk_overlap">
                      <InputNumber
                        min={0}
                        max={500}
                        style={{ width: '100%' }}
                        disabled={ragFieldLocked('rag.chunk_overlap')}
                      />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Item label="定时同步间隔（秒）" name="sync_interval_sec">
                      <InputNumber
                        min={30}
                        max={86400}
                        style={{ width: '100%' }}
                        disabled={ragFieldLocked('rag.sync_interval_sec')}
                      />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Item label="单次同步文章数上限" name="sync_batch">
                      <InputNumber
                        min={1}
                        max={500}
                        style={{ width: '100%' }}
                        disabled={ragFieldLocked('rag.sync_batch')}
                      />
                    </Form.Item>
                  </Col>
                </Row>
              </Form>

              <Table<ConfigSnapshot>
                size="small"
                title={() => <Text type="secondary" style={{ fontSize: 12 }}>Embedding 配置变更历史（每次保存一条完整快照）</Text>}
                rowKey="id"
                style={{ marginBottom: 16 }}
                scroll={{ x: 1400 }}
                dataSource={ragHistory}
                pagination={{
                  current: ragHistPage,
                  total: ragHistTotal,
                  pageSize: 8,
                  showSizeChanger: false,
                  onChange: (p) => void loadRagHistory(p),
                }}
                columns={ragSnapshotColumns}
              />
            </Card>

            <Table<RagSyncLog>
              size="small"
              rowKey="id"
              loading={ragLoading}
              dataSource={ragRuns}
              scroll={{ x: 960 }}
              pagination={{
                current: ragPage,
                total: ragTotal,
                pageSize: 10,
                showSizeChanger: false,
                onChange: (p) => void loadRagRuns(p),
              }}
              columns={[
                { title: 'ID', dataIndex: 'id', width: 72 },
                {
                  title: '状态',
                  dataIndex: 'status',
                  width: 92,
                  render: (s: RagSyncLog['status']) => (
                    <Tag color={s === 'success' ? 'success' : s === 'failed' ? 'error' : 'processing'}>{s}</Tag>
                  ),
                },
                { title: '方式', dataIndex: 'mode', width: 100 },
                {
                  title: '进度',
                  dataIndex: 'progress',
                  width: 72,
                  render: (p: number) => `${p}%`,
                },
                { title: '文章数', dataIndex: 'articlesProcessed', width: 88 },
                { title: '写入块', dataIndex: 'chunksUpserted', width: 88 },
                { title: '清理', dataIndex: 'chunksDeleted', width: 72 },
                {
                  title: '开始',
                  dataIndex: 'startedAt',
                  width: 128,
                  render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss'),
                },
                {
                  title: '结束',
                  dataIndex: 'finishedAt',
                  width: 128,
                  render: (t: string | undefined) => (t ? dayjs(t).format('MM-DD HH:mm:ss') : '-'),
                },
                {
                  title: '详情',
                  dataIndex: 'progressDetail',
                  ellipsis: true,
                  render: (t: string) => t || '-',
                },
              ]}
            />
          </Card>

          {/* LLM config edit form */}
          <Card
            title="大模型配置（AI 自动打标）"
            extra={
              <Text type="secondary" style={{ fontSize: 12 }}>
                修改后立即生效并持久化到数据库
              </Text>
            }
          >
            <div style={{ marginBottom: 16 }}>
              <Text type="secondary" style={{ marginRight: 8, fontSize: 12 }}>快速填入：</Text>
              <Space wrap size="small">
                {PRESETS.map(p => (
                  <Button key={p.label} size="small" onClick={() => applyPreset(p)}>
                    {p.label}
                  </Button>
                ))}
              </Space>
            </div>

            <Form
              form={form}
              layout="vertical"
              onFinish={handleSave}
              initialValues={{
                enabled: false,
                llmModel: 'deepseek-chat',
                llmBaseUrl: 'https://api.deepseek.com',
                llmApiKey: '',
                intervalSeconds: 120,
                batchSize: 20,
                maxPerTick: 200,
              }}
            >
              <Row gutter={24}>
                <Col xs={24} md={8}>
                  <Form.Item
                    label="启用后台任务"
                    name="enabled"
                    valuePropName="checked"
                    tooltip="关闭后，后台轮询会跳过本轮；现有数据不受影响"
                  >
                    <Switch checkedChildren="启用" unCheckedChildren="停用" />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item
                    label="API Base URL"
                    name="llmBaseUrl"
                    rules={[{ required: true, message: '请输入 API Base URL' }]}
                    tooltip="兼容 OpenAI 接口规范的任意服务地址"
                  >
                    <Input placeholder="https://api.deepseek.com" />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item
                    label="模型名"
                    name="llmModel"
                    rules={[{ required: true, message: '请输入模型名' }]}
                  >
                    <Input placeholder="deepseek-chat" />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item
                label="API Key"
                name="llmApiKey"
                tooltip={apiKeyHint}
                extra={apiKeyHint}
              >
                <Input.Password
                  autoComplete="new-password"
                  placeholder={tagger?.apiKeySet ? '留空则保留当前值' : 'sk-...'}
                />
              </Form.Item>

              <Row gutter={24}>
                <Col xs={24} md={8}>
                  <Form.Item label="轮询间隔（秒）" name="intervalSeconds" rules={[{ required: true }]}>
                    <InputNumber min={10} max={86400} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item label="单次 LLM 请求条数" name="batchSize" rules={[{ required: true }]}>
                    <InputNumber min={1} max={100} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item label="单次轮询最多处理" name="maxPerTick" rules={[{ required: true }]}>
                    <InputNumber min={1} max={10000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item style={{ marginBottom: 0 }}>
                <Space>
                  <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={saving}>
                    保存
                  </Button>
                  <Button onClick={() => { if (cfg?.tagger) applyToForm(cfg.tagger) }}>
                    重置
                  </Button>
                  {cfg?.note && (
                    <Text type="secondary" style={{ fontSize: 12 }}>{cfg.note}</Text>
                  )}
                </Space>
              </Form.Item>
            </Form>

            <Table<ConfigSnapshot>
              size="small"
              title={() => <Text type="secondary" style={{ fontSize: 12 }}>大模型配置变更历史（每次保存一条完整快照）</Text>}
              rowKey="id"
              scroll={{ x: 1200 }}
              dataSource={taggerHistory}
              pagination={{
                current: taggerHistPage,
                total: taggerHistTotal,
                pageSize: 8,
                showSizeChanger: false,
                onChange: (p) => void loadTaggerHistory(p),
              }}
              columns={taggerSnapshotColumns}
            />
          </Card>
        </>
      )}
    </div>
  )
}

export default SystemPage

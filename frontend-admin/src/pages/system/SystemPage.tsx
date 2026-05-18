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
  Row,
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
  RagStatus,
  RagSyncLog,
  SystemConfigResponse,
  SystemHealth,
  TaggerConfig,
  UpdateTaggerPayload,
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
  const [form] = Form.useForm<FormValues>()

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
      if ((rs as RagStatus | null)?.serviceReachable) {
        const cfg2 = await adminRagApi.getConfig().catch(() => null)
        if (cfg2 != null) setRagSyncEnabled(cfg2.sync_enabled)
      } else if ((rs as RagStatus | null)?.syncEnabled != null) {
        setRagSyncEnabled((rs as RagStatus).syncEnabled!)
      }
      await loadRagRuns(1)
    } finally {
      setLoading(false)
    }
  }, [applyToForm, loadRagRuns])

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
                    disabled={!ragStatus?.serviceReachable}
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
              <Descriptions.Item label="句向量模型（非对话 LLM）">
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
          </Card>
        </>
      )}
    </div>
  )
}

export default SystemPage

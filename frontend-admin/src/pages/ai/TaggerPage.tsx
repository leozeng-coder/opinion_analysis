import React, { useCallback, useEffect, useRef, useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  Descriptions,
  Divider,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Row,
  Space,
  Statistic,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd'
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  RobotOutlined,
  SaveOutlined,
  SyncOutlined,
} from '@ant-design/icons'
import { adminSystemApi } from '@/api/admin-system'
import { adminAuditApi } from '@/api/admin-audit'
import { taggerApi } from '@/api/tagger'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'
import type {
  AuditLog,
  ConfigSnapshot,
  SystemConfigResponse,
  TaggerConfig,
  TaggerSnapshotConfig,
  UpdateTaggerPayload,
} from '@/types'
import dayjs from 'dayjs'

const { Text } = Typography

const PRESETS = [
  { label: 'DeepSeek', baseUrl: 'https://api.deepseek.com', model: 'deepseek-chat' },
  { label: 'OpenAI', baseUrl: 'https://api.openai.com', model: 'gpt-4o' },
  { label: '百炼/Qwen', baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1', model: 'qwen-plus' },
  { label: 'Kimi', baseUrl: 'https://api.moonshot.cn/v1', model: 'moonshot-v1-8k' },
  { label: '智谱/GLM', baseUrl: 'https://open.bigmodel.cn/api/paas/v4', model: 'glm-4-flash' },
]

interface TaggerFormValues {
  enabled: boolean
  llmModel: string
  llmBaseUrl: string
  llmApiKey: string
  intervalSeconds: number
  batchSize: number
  maxPerTick: number
}

// ─── 配置 Tab ────────────────────────────────────────────────────────────────
const TaggerConfigTab: React.FC = () => {
  const [cfg, setCfg] = useState<SystemConfigResponse | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm<TaggerFormValues>()

  const [history, setHistory] = useState<ConfigSnapshot[]>([])
  const [histTotal, setHistTotal] = useState(0)
  const [histPage, setHistPage] = useState(1)

  const applyToForm = useCallback((t: TaggerConfig) => {
    form.setFieldsValue({
      enabled: t.enabled, llmModel: t.llmModel, llmBaseUrl: t.llmBaseUrl,
      llmApiKey: '', intervalSeconds: t.intervalSeconds,
      batchSize: t.batchSize, maxPerTick: t.maxPerTick,
    })
  }, [form])

  const loadHistory = useCallback(async (page: number) => {
    const r = await adminSystemApi.settingHistory({ domain: 'tagger', page, pageSize: 8 })
    setHistory(r.list); setHistTotal(r.total); setHistPage(page)
  }, [])

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      const c = await adminSystemApi.config()
      setCfg(c)
      if (c?.tagger) applyToForm(c.tagger)
      await loadHistory(1)
    } finally { setLoading(false) }
  }, [applyToForm, loadHistory])

  useEffect(() => { void fetchAll() }, [fetchAll])

  const handleDeleteHistory = async (id: number) => {
    try {
      await adminSystemApi.deleteSettingHistory(id)
      message.success('已删除历史记录')
      await loadHistory(histPage)
    } catch { /* ignore */ }
  }

  const handleReapplyHistory = async (id: number) => {
    try {
      const resp = await adminSystemApi.reapplySettingHistory(id)
      resp.warning ? message.warning(resp.message) : message.success(resp.message)
      await loadHistory(histPage)
      const c = await adminSystemApi.config()
      setCfg(c)
      if (c?.tagger) applyToForm(c.tagger)
    } catch { /* ignore */ }
  }

  const handleSave = async (values: TaggerFormValues) => {
    const payload: UpdateTaggerPayload = {
      enabled: values.enabled, llmModel: values.llmModel, llmBaseUrl: values.llmBaseUrl,
      intervalSeconds: values.intervalSeconds, batchSize: values.batchSize, maxPerTick: values.maxPerTick,
    }
    const trimmed = (values.llmApiKey ?? '').trim()
    if (trimmed) payload.llmApiKey = trimmed
    setSaving(true)
    try {
      const resp = await adminSystemApi.updateTagger(payload)
      setCfg(resp)
      if (resp?.tagger) applyToForm(resp.tagger)
      message.success('已保存，后台任务下一轮 tick 生效')
      void loadHistory(1)
    } catch { /* ignore */ }
    finally { setSaving(false) }
  }

  const tagger = cfg?.tagger
  const apiKeyHint = tagger?.apiKeySet
    ? `已配置（${tagger.llmApiKey || '***'}），留空则保留`
    : '尚未配置，必须填写后台任务才能运行'

  const columns = [
    {
      title: '启用', width: 64,
      render: (_: unknown, row: ConfigSnapshot) =>
        (row.config as TaggerSnapshotConfig).enabled ? <Tag color="success">是</Tag> : <Tag>否</Tag>,
    },
    { title: '模型', width: 140, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).llm_model || '-' },
    { title: 'API URL', width: 200, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).llm_base_url || '-' },
    { title: 'API Key', width: 120, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).llm_api_key || '-' },
    { title: '轮询间隔', width: 88, render: (_: unknown, row: ConfigSnapshot) => `${(row.config as TaggerSnapshotConfig).interval_seconds}s` },
    { title: '批次', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).batch_size },
    { title: '上限', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).max_per_tick },
    { title: '操作者', width: 88, dataIndex: 'updatedByName', render: (v: string) => v || '-' },
    { title: '时间', width: 128, dataIndex: 'createdAt', render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss') },
    {
      title: '操作', key: 'actions', width: 100, fixed: 'right' as const,
      render: (_: unknown, row: ConfigSnapshot) => (
        <Space size={4} wrap>
          <Button type="link" size="small" onClick={() => void handleReapplyHistory(row.id)}>应用</Button>
          <Popconfirm title="确定删除这条历史记录？" onConfirm={() => void handleDeleteHistory(row.id)}>
            <Button type="link" size="small" danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card bordered={false} className={ui.panelCard} loading={loading} title={<span><RobotOutlined style={{ marginRight: 8 }} />大模型配置</span>}
      extra={<Text type="secondary" style={{ fontSize: 12 }}>修改后立即生效并持久化到数据库</Text>}
    >
      <div style={{ marginBottom: 16 }}>
        <Text type="secondary" style={{ marginRight: 8, fontSize: 12 }}>快速填入：</Text>
        <Space wrap size="small">
          {PRESETS.map((p) => (
            <Button key={p.label} size="small" onClick={() => form.setFieldsValue({ llmBaseUrl: p.baseUrl, llmModel: p.model })}>
              {p.label}
            </Button>
          ))}
        </Space>
      </div>

      <Form form={form} layout="vertical" onFinish={handleSave}
        initialValues={{ enabled: false, llmModel: 'deepseek-chat', llmBaseUrl: 'https://api.deepseek.com', llmApiKey: '', intervalSeconds: 120, batchSize: 20, maxPerTick: 200 }}
      >
        <Row gutter={24}>
          <Col xs={24} md={8}>
            <Form.Item label="启用后台任务" name="enabled" valuePropName="checked">
              <Switch checkedChildren="启用" unCheckedChildren="停用" />
            </Form.Item>
          </Col>
          <Col xs={24} md={8}>
            <Form.Item label="API Base URL" name="llmBaseUrl" rules={[{ required: true }]}>
              <Input placeholder="https://api.deepseek.com" />
            </Form.Item>
          </Col>
          <Col xs={24} md={8}>
            <Form.Item label="模型名" name="llmModel" rules={[{ required: true }]}>
              <Input placeholder="deepseek-chat" />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item label="API Key" name="llmApiKey" extra={apiKeyHint}>
          <Input.Password autoComplete="new-password" placeholder={tagger?.apiKeySet ? '留空则保留当前值' : 'sk-...'} />
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
            <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={saving}>保存</Button>
            <Button onClick={() => { if (cfg?.tagger) applyToForm(cfg.tagger) }}>重置</Button>
            {cfg?.note && <Text type="secondary" style={{ fontSize: 12 }}>{cfg.note}</Text>}
          </Space>
        </Form.Item>
      </Form>

      <Table<ConfigSnapshot>
        size="small"
        title={() => <Text type="secondary" style={{ fontSize: 12 }}>大模型配置变更历史</Text>}
        rowKey="id"
        style={{ marginTop: 16 }}
        scroll={{ x: 1200 }}
        dataSource={history}
        pagination={{ current: histPage, total: histTotal, pageSize: 8, showSizeChanger: false, onChange: (p) => void loadHistory(p) }}
        columns={columns}
      />
    </Card>
  )
}

// ─── 任务监控 Tab ─────────────────────────────────────────────────────────────
const TaggerMonitorTab: React.FC = () => {
  const [pending, setPending] = useState<number | null>(null)
  const [lastRun, setLastRun] = useState<AuditLog | null>(null)
  const [taggerCfg, setTaggerCfg] = useState<TaggerConfig | null>(null)
  const [triggering, setTriggering] = useState(false)
  const [loading, setLoading] = useState(false)
  const refreshRef = useRef<ReturnType<typeof window.setInterval> | null>(null)

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      await Promise.all([
        taggerApi.pending().then((r) => setPending(r.pending)).catch(() => {}),
        adminAuditApi.list({ resource: 'tagger', action: 'run', pageSize: 1 })
          .then((r) => { if (r.list.length > 0) setLastRun(r.list[0]) })
          .catch(() => {}),
        adminSystemApi.config()
          .then((r) => { if (r?.tagger) setTaggerCfg(r.tagger) })
          .catch(() => {}),
      ])
    } finally { setLoading(false) }
  }, [])

  useEffect(() => {
    void fetchAll()
    refreshRef.current = window.setInterval(() => {
      void taggerApi.pending().then((r) => setPending(r.pending)).catch(() => {})
    }, 12_000)
    return () => { if (refreshRef.current) window.clearInterval(refreshRef.current) }
  }, [fetchAll])

  const handleTrigger = async () => {
    setTriggering(true)
    try {
      const res = await taggerApi.run()
      void message.success(res.message)
      window.setTimeout(() => void fetchAll(), 2000)
    } finally { setTriggering(false) }
  }

  const isEnabled = taggerCfg?.enabled ?? false

  return (
    <Card bordered={false} className={ui.panelCard} title={<span><SyncOutlined style={{ marginRight: 8 }} />AI 打标任务监控</span>}>
      <Row gutter={[16, 16]} style={{ marginBottom: 20 }}>
        <Col xs={24} sm={8}>
          <Card size="small" style={{ textAlign: 'center' }}>
            <Statistic
              title="当前待打标文章"
              value={pending ?? '-'}
              suffix="篇"
              loading={loading}
              valueStyle={{ color: pending && pending > 0 ? '#E8A84A' : '#42C48C', fontSize: 32 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small" style={{ textAlign: 'center' }}>
            <Statistic
              title="后台自动轮询"
              value={taggerCfg ? `${taggerCfg.intervalSeconds}s` : '-'}
              prefix={<ClockCircleOutlined />}
              valueStyle={{ fontSize: 28 }}
            />
            <div style={{ marginTop: 4 }}>
              {isEnabled
                ? <Badge status="processing" text={<Text type="secondary" style={{ fontSize: 12 }}>运行中</Text>} />
                : <Badge status="default" text={<Text type="secondary" style={{ fontSize: 12 }}>已停用</Text>} />}
            </div>
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small" style={{ textAlign: 'center' }}>
            <Statistic
              title="单次批量"
              value={taggerCfg?.batchSize ?? '-'}
              suffix="条/批"
              valueStyle={{ fontSize: 28 }}
            />
            <div style={{ marginTop: 4 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>上限 {taggerCfg?.maxPerTick ?? '-'} 条/轮</Text>
            </div>
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} md={10}>
          <Card size="small" title={<span><SyncOutlined style={{ marginRight: 6 }} />手动触发</span>} style={{ height: '100%' }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <Text type="secondary" style={{ fontSize: 13 }}>
                后台每 {taggerCfg?.intervalSeconds ?? 120} 秒自动轮询一次；点击下方按钮可立即执行一轮，在后台异步运行。
              </Text>
              {!isEnabled && (
                <Tag className={ui.softTagAmber} style={{ width: 'fit-content' }}>后台任务已停用，手动触发仍可执行</Tag>
              )}
              {!taggerCfg?.apiKeySet && (
                <Tag className={ui.softTagRose} style={{ width: 'fit-content' }}>API Key 未配置，请先到「配置」tab 完成设置</Tag>
              )}
              <Button
                type="primary" size="large" icon={<SyncOutlined spin={triggering} />}
                loading={triggering} disabled={!taggerCfg?.apiKeySet}
                onClick={() => void handleTrigger()} style={{ alignSelf: 'flex-start' }}
              >
                立即触发一轮打标
              </Button>
              <Text type="secondary" style={{ fontSize: 12 }}>待打标数量每 12 秒自动刷新</Text>
            </div>
          </Card>
        </Col>
        <Col xs={24} md={14}>
          <Card size="small" title={<span><CheckCircleOutlined style={{ marginRight: 6 }} />最近一次手动触发记录</span>} style={{ height: '100%' }}>
            {lastRun ? (
              <>
                <Descriptions column={2} size="small" labelStyle={{ color: '#888', width: 80 }}>
                  <Descriptions.Item label="触发时间" span={2}>
                    <Text>{dayjs(lastRun.createdAt).format('YYYY-MM-DD HH:mm:ss')}</Text>
                  </Descriptions.Item>
                  <Descriptions.Item label="操作人">{lastRun.actorName}</Descriptions.Item>
                  <Descriptions.Item label="用户 ID">{lastRun.actorId}</Descriptions.Item>
                  <Descriptions.Item label="来源 IP">{lastRun.ip || '-'}</Descriptions.Item>
                  <Descriptions.Item label="路径">
                    <Text code style={{ fontSize: 11 }}>{lastRun.method} {lastRun.path}</Text>
                  </Descriptions.Item>
                </Descriptions>
                <Divider style={{ margin: '12px 0' }} />
                <Text type="secondary" style={{ fontSize: 12 }}>
                  完整历史请查看 <a href="/audit">审计日志</a>
                </Text>
              </>
            ) : (
              <div style={{ textAlign: 'center', padding: '32px 0', color: '#bbb' }}>
                <RobotOutlined style={{ fontSize: 32, marginBottom: 8 }} />
                <div>暂无手动触发记录</div>
              </div>
            )}
          </Card>
        </Col>
      </Row>
    </Card>
  )
}

// ─── 主页面 ────────────────────────────────────────────────────────────────────
const TaggerPage: React.FC = () => (
  <div className={ui.pageShell}>
    <PageHeader
      title="打标任务"
      subtitle="大模型配置与 AI 打标任务监控"
      icon={<RobotOutlined />}
    />
    <Tabs
      items={[
        { key: 'config', label: '配置', children: <TaggerConfigTab /> },
        { key: 'monitor', label: '任务监控', children: <TaggerMonitorTab /> },
      ]}
    />
  </div>
)

export default TaggerPage

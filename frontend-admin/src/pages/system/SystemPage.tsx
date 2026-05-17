import React, { useCallback, useEffect, useState } from 'react'
import {
  Badge,
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  Row,
  Space,
  Spin,
  Statistic,
  Switch,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  ReloadOutlined,
  SaveOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { adminSystemApi } from '@/api/admin-system'
import type { SystemConfigResponse, SystemHealth, TaggerConfig, UpdateTaggerPayload } from '@/types'
import dayjs from 'dayjs'

const { Title, Text } = Typography

interface ProbeCardProps {
  title: string
  probe?: { ok: boolean; message?: string; latencyMs: number }
}

const ProbeCard: React.FC<ProbeCardProps> = ({ title, probe }) => (
  <Card size="small">
    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
      <Text strong>{title}</Text>
      {probe ? (
        probe.ok ? (
          <Tag icon={<CheckCircleOutlined />} color="success">正常 {probe.latencyMs}ms</Tag>
        ) : (
          <Tooltip title={probe.message}>
            <Tag icon={<CloseCircleOutlined />} color="error">异常</Tag>
          </Tooltip>
        )
      ) : (
        <Tag>-</Tag>
      )}
    </div>
    {probe?.message && !probe.ok && (
      <Text type="danger" style={{ fontSize: 12 }}>{probe.message}</Text>
    )}
  </Card>
)

interface FormValues {
  enabled: boolean
  model: string
  deepseekBaseUrl: string
  deepseekApiKey: string
  intervalSeconds: number
  batchSize: number
  maxPerTick: number
}

const SystemPage: React.FC = () => {
  const [health, setHealth] = useState<SystemHealth | null>(null)
  const [cfg, setCfg] = useState<SystemConfigResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<FormValues>()

  const applyToForm = useCallback((t: TaggerConfig) => {
    form.setFieldsValue({
      enabled: t.enabled,
      model: t.model,
      deepseekBaseUrl: t.deepseekBaseUrl,
      // API Key 字段后端只回脱敏值；编辑表单留空，由 placeholder 暗示「不填则保留现有」
      deepseekApiKey: '',
      intervalSeconds: t.intervalSeconds,
      batchSize: t.batchSize,
      maxPerTick: t.maxPerTick,
    })
  }, [form])

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      const [h, c] = await Promise.all([adminSystemApi.health(), adminSystemApi.config()])
      setHealth(h)
      setCfg(c)
      if (c?.tagger) applyToForm(c.tagger)
    } finally {
      setLoading(false)
    }
  }, [applyToForm])

  useEffect(() => { void fetchAll() }, [fetchAll])

  const handleSave = async (values: FormValues) => {
    const payload: UpdateTaggerPayload = {
      enabled: values.enabled,
      model: values.model,
      deepseekBaseUrl: values.deepseekBaseUrl,
      intervalSeconds: values.intervalSeconds,
      batchSize: values.batchSize,
      maxPerTick: values.maxPerTick,
    }
    // 只有用户在输入框里填了内容才传 apiKey（避免把脱敏占位符写回）
    const trimmed = (values.deepseekApiKey ?? '').trim()
    if (trimmed) payload.deepseekApiKey = trimmed

    setSaving(true)
    try {
      const resp = await adminSystemApi.updateTagger(payload)
      setCfg(resp)
      if (resp?.tagger) applyToForm(resp.tagger)
      message.success('已保存，后台任务下一轮 tick 生效')
      void adminSystemApi.health().then(setHealth)
    } catch (e) {
      // request 拦截器一般已弹错，这里保底
      console.error(e)
    } finally {
      setSaving(false)
    }
  }

  const tagger = cfg?.tagger
  const apiKeyHint = tagger?.apiKeySet
    ? `已配置（${tagger.deepseekApiKey || '***'}），留空则保留`
    : '尚未配置，必须填写后台任务才能运行'

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
            <Col xs={24} sm={12} md={6}>
              <ProbeCard title="数据库 (MySQL)" probe={health?.database} />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <ProbeCard title="DeepSeek API" probe={health?.deepseek} />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Card size="small">
                <Statistic title="待打标文章" value={health?.pendingTagging ?? '-'} suffix="篇" />
              </Card>
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Card size="small" title="最近爬取">
                {health?.lastCrawlerRun ? (
                  <div style={{ fontSize: 12 }}>
                    <div>
                      <Text type="secondary">{health.lastCrawlerRun.spiders}</Text>
                    </div>
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
            title="大模型配置（AI 自动打标）"
            extra={
              <Text type="secondary" style={{ fontSize: 12 }}>
                修改后立即生效并持久化到数据库
              </Text>
            }
          >
            <Form
              form={form}
              layout="vertical"
              onFinish={handleSave}
              initialValues={{
                enabled: false,
                model: 'deepseek-chat',
                deepseekBaseUrl: 'https://api.deepseek.com',
                deepseekApiKey: '',
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
                    label="模型名"
                    name="model"
                    rules={[{ required: true, message: '请输入模型名' }]}
                  >
                    <Input placeholder="deepseek-chat" />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item
                    label="API Base URL"
                    name="deepseekBaseUrl"
                    rules={[{ required: true, message: '请输入 API Base URL' }]}
                  >
                    <Input placeholder="https://api.deepseek.com" />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item
                label="DeepSeek API Key"
                name="deepseekApiKey"
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
                  <Form.Item
                    label="轮询间隔（秒）"
                    name="intervalSeconds"
                    rules={[{ required: true }]}
                  >
                    <InputNumber min={10} max={86400} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item
                    label="单次 LLM 请求条数"
                    name="batchSize"
                    rules={[{ required: true }]}
                  >
                    <InputNumber min={1} max={100} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item
                    label="单次轮询最多处理"
                    name="maxPerTick"
                    rules={[{ required: true }]}
                  >
                    <InputNumber min={1} max={10000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item style={{ marginBottom: 0 }}>
                <Space>
                  <Button
                    type="primary"
                    htmlType="submit"
                    icon={<SaveOutlined />}
                    loading={saving}
                  >
                    保存
                  </Button>
                  <Button
                    onClick={() => {
                      if (cfg?.tagger) applyToForm(cfg.tagger)
                    }}
                  >
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

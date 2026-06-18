import React, { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  Row,
  Space,
  Switch,
  Typography,
  message,
} from 'antd'
import { GlobalOutlined, SaveOutlined } from '@ant-design/icons'
import { adminSystemApi } from '@/api/admin-system'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'
import type { SystemConfigResponse, TaggerConfig, UpdateTaggerPayload } from '@/types'

const { Text, Link: ALink } = Typography

interface WebSearchFormValues {
  webSearchEnabled: boolean
  webSearchApiKey: string
  webSearchCount: number
}

const WebSearchPage: React.FC = () => {
  const [cfg, setCfg] = useState<SystemConfigResponse | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm<WebSearchFormValues>()

  const applyToForm = useCallback((t: TaggerConfig) => {
    form.setFieldsValue({
      webSearchEnabled: t.webSearchEnabled,
      webSearchApiKey: '',
      webSearchCount: t.webSearchCount,
    })
  }, [form])

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      const c = await adminSystemApi.config()
      setCfg(c)
      if (c?.tagger) applyToForm(c.tagger)
    } finally {
      setLoading(false)
    }
  }, [applyToForm])

  useEffect(() => { void fetchAll() }, [fetchAll])

  const handleSave = async (values: WebSearchFormValues) => {
    const webKeyTrimmed = (values.webSearchApiKey ?? '').trim()
    // 开启联网搜索但既无已存 Key、本次也没填：拦截，避免“显示启用却静默不生效”。
    if (values.webSearchEnabled && !webKeyTrimmed && !tagger?.webSearchKeySet) {
      message.warning('开启联网搜索需先填写博查 API Key')
      return
    }
    const payload: UpdateTaggerPayload = {
      webSearchEnabled: values.webSearchEnabled,
      webSearchCount: values.webSearchCount,
    }
    if (webKeyTrimmed) payload.webSearchApiKey = webKeyTrimmed
    setSaving(true)
    try {
      const resp = await adminSystemApi.updateTagger(payload)
      setCfg(resp)
      if (resp?.tagger) applyToForm(resp.tagger)
      message.success('已保存，下一轮对话生效')
    } catch (e) {
      console.error(e)
    } finally {
      setSaving(false)
    }
  }

  const tagger = cfg?.tagger
  const keyHint = tagger?.webSearchKeySet
    ? `已配置（${tagger.webSearchApiKey || '***'}），留空则保留当前值`
    : '尚未配置，开启联网搜索需填写博查 API Key'

  return (
    <>
      <PageHeader
        title="联网搜索能力"
        subtitle="深度思考模式下，助手可调用博查（Bocha）联网搜索补充实时与外部信息"
      />
      <Card
        bordered={false}
        className={ui.panelCard}
        loading={loading}
        title={<span><GlobalOutlined style={{ marginRight: 8 }} />博查联网搜索</span>}
        extra={<Text type="secondary" style={{ fontSize: 12 }}>修改后保存即持久化到数据库</Text>}
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          message="作用范围与开关关系"
          description={
            <div style={{ fontSize: 13 }}>
              <div>· 仅在<strong>深度思考模式</strong>生效，普通对话不受影响。</div>
              <div>· 此处为<strong>全局总开关</strong>：开启并配置 Key 后，用户对话框才会出现“联网搜索”按钮。</div>
              <div>· 是否真正联网由用户每轮自行点选，可按需控制成本。</div>
              <div style={{ marginTop: 4 }}>
                API Key 获取：<ALink href="https://open.bocha.cn" target="_blank">博查 AI 开放平台</ALink>（需充值）
              </div>
            </div>
          }
        />

        <Form
          form={form}
          layout="vertical"
          onFinish={handleSave}
          initialValues={{ webSearchEnabled: false, webSearchApiKey: '', webSearchCount: 5 }}
        >
          <Row gutter={24}>
            <Col xs={24} md={8}>
              <Form.Item label="启用联网搜索" name="webSearchEnabled" valuePropName="checked">
                <Switch checkedChildren="启用" unCheckedChildren="停用" />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item label="单次返回结果数" name="webSearchCount" rules={[{ required: true }]}>
                <InputNumber min={1} max={20} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item label="博查 API Key" name="webSearchApiKey" extra={keyHint}>
            <Input.Password
              autoComplete="new-password"
              placeholder={tagger?.webSearchKeySet ? '留空则保留当前值' : 'sk-...'}
            />
          </Form.Item>
          <Form.Item style={{ marginBottom: 0 }}>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={saving}>
                保存
              </Button>
              <Button onClick={() => { if (cfg?.tagger) applyToForm(cfg.tagger) }}>
                重置
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>
    </>
  )
}

export default WebSearchPage

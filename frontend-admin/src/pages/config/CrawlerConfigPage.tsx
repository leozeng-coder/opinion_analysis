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
import { BellOutlined, MailOutlined, SaveOutlined } from '@ant-design/icons'
import { adminSmtpApi, alertApi } from '@/api/admin-smtp'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'

const { Text } = Typography

interface SmtpFormValues {
  host: string
  port: number
  username: string
  password: string
  from: string
  useTls: boolean
  onCrawl: boolean
}

const CrawlerConfigPage: React.FC = () => {
  const [smtpForm] = Form.useForm<SmtpFormValues>()
  const smtpPort = Form.useWatch('port', smtpForm) ?? 465
  const smtpUsername = Form.useWatch('username', smtpForm) ?? ''
  const smtpFrom = Form.useWatch('from', smtpForm) ?? ''
  const [smtpSaving, setSmtpSaving] = useState(false)
  const [smtpPasswordSet, setSmtpPasswordSet] = useState(false)
  const [evaluating, setEvaluating] = useState(false)
  const [testEmail, setTestEmail] = useState('')
  const [testingMail, setTestingMail] = useState(false)
  const [loading, setLoading] = useState(false)

  const loadSmtpConfig = useCallback(async () => {
    setLoading(true)
    try {
      const cfg = await adminSmtpApi.getConfig().catch(() => null)
      if (!cfg) return
      setSmtpPasswordSet(cfg.passwordSet)
      smtpForm.setFieldsValue({
        host: cfg.host,
        port: cfg.port,
        username: cfg.username,
        from: cfg.from,
        useTls: cfg.useTls,
        onCrawl: cfg.onCrawl,
        password: '',
      })
    } finally {
      setLoading(false)
    }
  }, [smtpForm])

  useEffect(() => {
    void loadSmtpConfig()
  }, [loadSmtpConfig])

  const handleSaveSmtp = async (values: SmtpFormValues) => {
    setSmtpSaving(true)
    try {
      const payload: Parameters<typeof adminSmtpApi.updateConfig>[0] = {
        host: values.host.trim(),
        port: values.port,
        username: values.username.trim(),
        from: values.from.trim(),
        useTls: values.useTls,
        onCrawl: values.onCrawl,
      }
      const pwd = (values.password ?? '').trim()
      if (!pwd && !smtpPasswordSet) {
        message.warning('首次配置请填写 SMTP 密码（邮箱授权码）')
        return
      }
      if (pwd) payload.password = pwd
      await adminSmtpApi.updateConfig(payload)
      message.success('告警邮件配置已保存')
      smtpForm.setFieldValue('password', '')
      void loadSmtpConfig()
    } finally {
      setSmtpSaving(false)
    }
  }

  const handleTestSmtp = async () => {
    const to = testEmail.trim()
    if (!to) {
      message.warning('请填写测试收件邮箱')
      return
    }
    let values: SmtpFormValues
    try {
      values = await smtpForm.validateFields(['host', 'port', 'username'])
    } catch {
      message.warning('请先填写 SMTP 服务器和用户名')
      return
    }
    const pwd = (smtpForm.getFieldValue('password') ?? '').trim()
    if (!pwd && !smtpPasswordSet) {
      message.warning('请填写 SMTP 密码（邮箱授权码），首次配置必填')
      return
    }
    setTestingMail(true)
    try {
      const savePayload: Parameters<typeof adminSmtpApi.updateConfig>[0] = {
        host: values.host.trim(),
        port: values.port,
        username: values.username.trim(),
        from: (values.from ?? '').trim(),
        useTls: values.useTls,
        onCrawl: smtpForm.getFieldValue('onCrawl'),
      }
      if (pwd) savePayload.password = pwd
      await adminSmtpApi.updateConfig(savePayload)
      setSmtpPasswordSet(true)
      smtpForm.setFieldValue('password', '')

      const res = await adminSmtpApi.test(to)
      message.success(res.message || '测试邮件已发送')
    } catch (e) {
      console.error(e)
    } finally {
      setTestingMail(false)
    }
  }

  const handleEvaluateAlerts = async () => {
    setEvaluating(true)
    try {
      const res = await alertApi.evaluate(true)
      if ('triggered' in res) {
        message.success(`评估完成：${res.evaluated} 条规则，触发 ${res.triggered} 条，跳过 ${res.skipped} 条`)
      } else {
        message.success(res.message)
      }
    } finally {
      setEvaluating(false)
    }
  }

  const smtpFromMismatch = (() => {
    const u = smtpUsername.trim()
    const f = smtpFrom.trim()
    if (!u || !f) return false
    const ud = u.includes('@') ? u.split('@').pop()?.toLowerCase() : ''
    const fd = f.includes('@') ? f.split('@').pop()?.toLowerCase() : ''
    return ud && fd && ud !== fd
  })()

  const handlePortChange = (port: number | null) => {
    if (port === 465) smtpForm.setFieldValue('useTls', false)
    if (port === 587) smtpForm.setFieldValue('useTls', true)
  }

  return (
    <div className={ui.pageShell}>
      <PageHeader
        title="爬虫配置"
        subtitle="告警邮件、SMTP 服务器配置"
        icon={<BellOutlined />}
      />

      <Card
        bordered={false}
        className={ui.panelCard}
        title={
          <span>
            <BellOutlined style={{ marginRight: 8 }} />
            告警邮件与触发
          </span>
        }
        extra={
          <Button icon={<BellOutlined />} loading={evaluating} onClick={() => void handleEvaluateAlerts()}>
            立即评估告警
          </Button>
        }
        loading={loading}
      >
        <Form
          form={smtpForm}
          layout="vertical"
          onFinish={(v) => void handleSaveSmtp(v)}
          initialValues={{ port: 465, useTls: false, onCrawl: true }}
        >
          <Row gutter={16}>
            <Col xs={24} md={8}>
              <Form.Item label="爬虫完成后自动评估" name="onCrawl" valuePropName="checked">
                <Switch checkedChildren="开" unCheckedChildren="关" />
              </Form.Item>
            </Col>
            <Col xs={24} md={16}>
              <Text type="secondary" style={{ fontSize: 12 }}>
                开启后，每次爬虫任务成功结束会自动检查预警规则；规则间隔与去重由每条规则的「检测间隔」控制。
              </Text>
            </Col>
          </Row>
          {smtpFromMismatch && (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
              message="发件人域名与 SMTP 账号不一致"
              description="163/QQ 等邮箱要求发件人与登录账号同域名。请留空发件人，或改为与用户名相同的 163 邮箱。"
            />
          )}
          <Row gutter={16}>
            <Col xs={24} md={8}>
              <Form.Item label="SMTP 服务器" name="host" rules={[{ required: true, message: '请输入 SMTP 地址' }]}>
                <Input placeholder="smtp.163.com" prefix={<MailOutlined />} />
              </Form.Item>
            </Col>
            <Col xs={24} md={4}>
              <Form.Item
                label="端口"
                name="port"
                rules={[{ required: true }]}
                extra={smtpPort === 465 ? '465=SSL' : '587=STARTTLS'}
              >
                <InputNumber min={1} max={65535} style={{ width: '100%' }} onChange={handlePortChange} />
              </Form.Item>
            </Col>
            <Col xs={24} md={6}>
              <Form.Item label="用户名" name="username" rules={[{ required: true, message: '请输入用户名' }]}>
                <Input placeholder="xxx@163.com" />
              </Form.Item>
            </Col>
            <Col xs={24} md={6}>
              <Form.Item
                label="密码"
                name="password"
                extra={smtpPasswordSet ? '已配置，留空则保留' : '163/QQ 请填邮箱授权码，不是登录密码'}
              >
                <Input.Password autoComplete="new-password" placeholder={smtpPasswordSet ? '留空保留' : '授权码'} />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item label="发件人（可选）" name="from" extra="须与用户名同域名，留空则自动使用用户名">
                <Input placeholder="留空 = 用户名" />
              </Form.Item>
            </Col>
            <Col xs={24} md={4}>
              <Form.Item
                label="STARTTLS"
                name="useTls"
                valuePropName="checked"
                extra={smtpPort === 465 ? '465 端口请关闭' : '587 端口请开启'}
              >
                <Switch checkedChildren="开" unCheckedChildren="关" disabled={smtpPort === 465} />
              </Form.Item>
            </Col>
          </Row>
          <Space wrap>
            <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={smtpSaving}>
              保存告警配置
            </Button>
            <Input
              placeholder="测试收件邮箱"
              value={testEmail}
              onChange={(e) => setTestEmail(e.target.value)}
              style={{ width: 220 }}
            />
            <Button loading={testingMail} onClick={() => void handleTestSmtp()}>
              发送测试邮件
            </Button>
          </Space>
        </Form>
      </Card>
    </div>
  )
}

export default CrawlerConfigPage

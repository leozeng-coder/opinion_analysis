import React, { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  Row,
  Space,
  Spin,
  Switch,
  Typography,
  message,
  Alert,
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

const NotifyPage: React.FC = () => {
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
        host: cfg.host, port: cfg.port, username: cfg.username,
        from: cfg.from, useTls: cfg.useTls, onCrawl: cfg.onCrawl, password: '',
      })
    } finally {
      setLoading(false)
    }
  }, [smtpForm])

  useEffect(() => { void loadSmtpConfig() }, [loadSmtpConfig])

  const handleSaveSmtp = async (values: SmtpFormValues) => {
    setSmtpSaving(true)
    try {
      await adminSmtpApi.updateConfig(values)
      message.success('SMTP 配置已保存')
      void loadSmtpConfig()
    } catch {
      message.error('保存失败')
    } finally {
      setSmtpSaving(false)
    }
  }

  const handleTestMail = async () => {
    if (!testEmail) { message.warning('请输入测试邮箱地址'); return }
    setTestingMail(true)
    try {
      await adminSmtpApi.test(testEmail)
      message.success('测试邮件已发送，请检查收件箱')
    } catch {
      message.error('发送失败，请检查配置')
    } finally {
      setTestingMail(false)
    }
  }

  const handleEvaluate = async () => {
    setEvaluating(true)
    try {
      await alertApi.evaluate()
      message.success('规则评估已触发')
    } catch {
      message.error('评估失败')
    } finally {
      setEvaluating(false)
    }
  }

  return (
    <div className={ui.pageShell}>
      <PageHeader
        title="通知与告警"
        subtitle="SMTP 邮件配置及告警规则手动评估"
        icon={<BellOutlined />}
      />

      <Card bordered={false} className={ui.panelCard} title={<><MailOutlined style={{ marginRight: 8 }} />SMTP 邮件配置</>}>
        <Spin spinning={loading}>
          <Form form={smtpForm} layout="vertical" onFinish={handleSaveSmtp}>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item label="SMTP 服务器" name="host" rules={[{ required: true }]}>
                  <Input placeholder="smtp.example.com" />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item label="端口" name="port" rules={[{ required: true }]}>
                  <InputNumber min={1} max={65535} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item label="用户名" name="username" rules={[{ required: true }]}>
                  <Input placeholder="user@example.com" />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  label={smtpPasswordSet ? '密码（留空保持不变）' : '密码'}
                  name="password"
                  rules={[{ required: !smtpPasswordSet }]}
                >
                  <Input.Password placeholder={smtpPasswordSet ? '••••••••' : '请输入密码'} />
                </Form.Item>
              </Col>
            </Row>
            <Form.Item label="发件人地址" name="from" rules={[{ required: true, type: 'email' }]}>
              <Input placeholder="noreply@example.com" />
            </Form.Item>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item label="启用 TLS" name="useTls" valuePropName="checked">
                  <Switch />
                </Form.Item>
                {smtpPort === 465 && (
                  <Text type="secondary" style={{ fontSize: 12 }}>端口 465 通常使用 SSL/TLS</Text>
                )}
              </Col>
              <Col span={12}>
                <Form.Item label="爬虫完成后发送邮件" name="onCrawl" valuePropName="checked">
                  <Switch />
                </Form.Item>
              </Col>
            </Row>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={smtpSaving}>
                保存配置
              </Button>
            </Space>
          </Form>

          <div style={{ marginTop: 24, paddingTop: 24, borderTop: '1px solid #f0f0f0' }}>
            <Space>
              <Input
                placeholder="输入测试邮箱"
                value={testEmail}
                onChange={(e) => setTestEmail(e.target.value)}
                style={{ width: 300 }}
                defaultValue={smtpUsername || smtpFrom}
              />
              <Button onClick={() => void handleTestMail()} loading={testingMail}>
                发送测试邮件
              </Button>
            </Space>
          </div>
        </Spin>
      </Card>

      <Card bordered={false} className={ui.panelCard} title={<><BellOutlined style={{ marginRight: 8 }} />告警规则评估</>}>
        <Alert
          message="手动触发告警规则评估"
          description="点击下方按钮可立即对所有文章执行告警规则评估，无需等待定时任务。"
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
        <Button type="primary" icon={<BellOutlined />} onClick={() => void handleEvaluate()} loading={evaluating}>
          立即评估
        </Button>
      </Card>
    </div>
  )
}

export default NotifyPage

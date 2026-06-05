import React, { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Col,
  Form,
  Input,
  InputNumber,
  Progress,
  Row,
  Space,
  Switch,
  Typography,
  message,
  Table,
  Tag,
  Spin,
} from 'antd'
import { BellOutlined, MailOutlined, SaveOutlined, SyncOutlined } from '@ant-design/icons'
import { adminSmtpApi, alertApi } from '@/api/admin-smtp'
import { platformSyncApi, type PlatformInfo, type PlatformSyncProgress } from '@/api/crawler'
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

  // 平台同步相关状态
  const [platforms, setPlatforms] = useState<PlatformInfo[]>([])
  const [selectedPlatforms, setSelectedPlatforms] = useState<string[]>([])
  const [syncProgress, setSyncProgress] = useState<{ [key: string]: PlatformSyncProgress }>({})
  const [syncing, setSyncing] = useState(false)
  const [progressInterval, setProgressInterval] = useState<ReturnType<typeof setInterval> | null>(null)

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

  // 加载平台列表
  const loadPlatforms = useCallback(async () => {
    try {
      const list = await platformSyncApi.getPlatformList()
      setPlatforms(list)
    } catch (error) {
      console.error('加载平台列表失败:', error)
    }
  }, [])

  // 加载同步进度
  const loadSyncProgress = useCallback(async () => {
    try {
      const progress = await platformSyncApi.getProgress()
      if (Array.isArray(progress)) {
        const progressMap: { [key: string]: PlatformSyncProgress } = {}
        progress.forEach((p) => {
          progressMap[p.platform] = p
        })
        setSyncProgress(progressMap)
      }
    } catch (error) {
      console.error('加载同步进度失败:', error)
    }
  }, [])

  useEffect(() => {
    void loadSmtpConfig()
    void loadPlatforms()
  }, [loadSmtpConfig, loadPlatforms])

  // 开始轮询进度
  const startProgressPolling = useCallback(() => {
    if (progressInterval) return
    const interval = setInterval(() => {
      void loadSyncProgress()
    }, 1000) // 每秒更新一次
    setProgressInterval(interval)
  }, [progressInterval, loadSyncProgress])

  // 停止轮询进度
  const stopProgressPolling = useCallback(() => {
    if (progressInterval) {
      clearInterval(progressInterval)
      setProgressInterval(null)
    }
  }, [progressInterval])

  // 清理定时器
  useEffect(() => {
    return () => {
      if (progressInterval) {
        clearInterval(progressInterval)
      }
    }
  }, [progressInterval])

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
    if (!testEmail) {
      message.warning('请输入测试邮箱地址')
      return
    }
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

  // 手动触发同步（支持多选）
  const handleSyncSelected = async () => {
    if (selectedPlatforms.length === 0) {
      message.warning('请至少选择一个平台')
      return
    }

    setSyncing(true)
    startProgressPolling()

    try {
      const results = await platformSyncApi.syncPlatforms(selectedPlatforms)

      // 等待一段时间让进度更新
      await new Promise(resolve => setTimeout(resolve, 1000))

      const summary = Object.entries(results).map(([platform, result]) => {
        const platformInfo = platforms.find(p => p.code === platform)
        return {
          platform: platformInfo?.name || platform,
          newCount: result.newCount,
          status: result.status,
        }
      })

      const totalNew = summary.reduce((sum, item) => sum + item.newCount, 0)
      const failed = summary.filter(item => item.status === 'failed').length

      if (failed > 0) {
        message.warning(`同步完成：新增 ${totalNew} 条数据，${failed} 个平台失败`)
      } else {
        message.success(`同步完成：新增 ${totalNew} 条数据`)
      }

      void loadPlatforms()
    } catch (error) {
      message.error('同步失败')
    } finally {
      setSyncing(false)
      setTimeout(() => {
        stopProgressPolling()
        setSyncProgress({})
      }, 2000)
    }
  }

  // 同步所有平台
  const handleSyncAll = async () => {
    setSyncing(true)
    startProgressPolling()

    try {
      const results = await platformSyncApi.syncAll()

      await new Promise(resolve => setTimeout(resolve, 1000))

      const summary = Object.entries(results).map(([platform, result]) => {
        const platformInfo = platforms.find(p => p.code === platform)
        return {
          platform: platformInfo?.name || platform,
          newCount: result.newCount,
        }
      })

      const totalNew = summary.reduce((sum, item) => sum + item.newCount, 0)
      message.success(`批量同步完成：共新增 ${totalNew} 条数据`)

      void loadPlatforms()
    } catch (error) {
      message.error('批量同步失败')
    } finally {
      setSyncing(false)
      setTimeout(() => {
        stopProgressPolling()
        setSyncProgress({})
      }, 2000)
    }
  }

  // 全选/取消全选
  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedPlatforms(platforms.map(p => p.code))
    } else {
      setSelectedPlatforms([])
    }
  }

  // 单选平台
  const handleSelectPlatform = (code: string, checked: boolean) => {
    if (checked) {
      setSelectedPlatforms([...selectedPlatforms, code])
    } else {
      setSelectedPlatforms(selectedPlatforms.filter(p => p !== code))
    }
  }

  // 获取进度百分比
  const getProgressPercent = (platform: string): number => {
    const progress = syncProgress[platform]
    if (!progress || progress.totalCount === 0) return 0
    return Math.round((progress.processedCount / progress.totalCount) * 100)
  }

  // 获取进度状态
  const getProgressStatus = (platform: string): 'success' | 'exception' | 'active' | 'normal' => {
    const progress = syncProgress[platform]
    if (!progress) return 'normal'
    if (progress.status === 'completed') return 'success'
    if (progress.status === 'failed') return 'exception'
    if (progress.status === 'running') return 'active'
    return 'normal'
  }

  // 平台同步表格列
  const syncColumns = [
    {
      title: (
        <Checkbox
          checked={selectedPlatforms.length === platforms.length && platforms.length > 0}
          indeterminate={selectedPlatforms.length > 0 && selectedPlatforms.length < platforms.length}
          onChange={(e) => handleSelectAll(e.target.checked)}
        >
          平台
        </Checkbox>
      ),
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: PlatformInfo) => (
        <Checkbox
          checked={selectedPlatforms.includes(record.code)}
          onChange={(e) => handleSelectPlatform(record.code, e.target.checked)}
        >
          {name}
        </Checkbox>
      ),
    },
    {
      title: '数据表',
      dataIndex: 'table',
      key: 'table',
    },
    {
      title: '最后同步时间',
      dataIndex: 'lastSyncTime',
      key: 'lastSyncTime',
      render: (time: string) => {
        if (!time) return <Text type="secondary">从未同步</Text>
        const date = new Date(time)
        const now = new Date()
        const diff = now.getTime() - date.getTime()
        const minutes = Math.floor(diff / 60000)

        if (minutes < 1) return '刚刚'
        if (minutes < 60) return `${minutes} 分钟前`
        if (minutes < 1440) return `${Math.floor(minutes / 60)} 小时前`
        return date.toLocaleString('zh-CN')
      },
    },
    {
      title: '同步进度',
      key: 'progress',
      render: (_: unknown, record: PlatformInfo) => {
        const progress = syncProgress[record.code]
        if (!progress || progress.status === 'pending') {
          return <Text type="secondary">-</Text>
        }

        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Progress
              percent={getProgressPercent(record.code)}
              status={getProgressStatus(record.code)}
              size="small"
            />
            <Text type="secondary" style={{ fontSize: 12 }}>
              {progress.status === 'running' && `处理中: ${progress.processedCount}/${progress.totalCount}`}
              {progress.status === 'completed' && `完成: 新增 ${progress.newCount}, 跳过 ${progress.skippedCount}`}
              {progress.status === 'failed' && `失败: ${progress.errorMessage}`}
            </Text>
          </Space>
        )
      },
    },
  ]

  return (
    <div className={ui.page}>
      <PageHeader title="爬虫配置" />

      <Row gutter={[16, 16]}>
        {/* 平台数据同步 */}
        <Col span={24}>
          <Card
            title="🔄 平台数据同步"
            extra={
              <Space>
                <Button
                  type="primary"
                  icon={<SyncOutlined />}
                  onClick={handleSyncSelected}
                  loading={syncing}
                  disabled={selectedPlatforms.length === 0}
                >
                  同步选中平台
                </Button>
                <Button
                  icon={<SyncOutlined />}
                  onClick={handleSyncAll}
                  loading={syncing}
                >
                  同步所有平台
                </Button>
              </Space>
            }
          >
            <Alert
              message="自动同步说明"
              description="MediaCrawler 爬虫完成后会自动触发数据同步，无需手动配置。此处提供手动同步功能用于紧急情况。支持多选平台批量同步，实时查看同步进度。"
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
            />

            <Table
              columns={syncColumns}
              dataSource={platforms}
              rowKey="code"
              pagination={false}
              loading={loading}
            />
          </Card>
        </Col>

        {/* SMTP 配置 */}
        <Col span={24}>
          <Card title={<><MailOutlined /> SMTP 邮件配置</>}>
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
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        端口 465 通常使用 SSL/TLS
                      </Text>
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
                  <Button onClick={handleTestMail} loading={testingMail}>
                    发送测试邮件
                  </Button>
                </Space>
              </div>
            </Spin>
          </Card>
        </Col>

        {/* 告警规则评估 */}
        <Col span={24}>
          <Card title={<><BellOutlined /> 告警规则评估</>}>
            <Alert
              message="手动触发告警规则评估"
              description="点击下方按钮可立即对所有文章执行告警规则评估，无需等待定时任务。"
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
            />
            <Button type="primary" icon={<BellOutlined />} onClick={handleEvaluate} loading={evaluating}>
              立即评估
            </Button>
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default CrawlerConfigPage

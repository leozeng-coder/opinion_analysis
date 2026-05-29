import React, { useState, useEffect, useRef } from 'react'
import {
  Card,
  Form,
  Select,
  Input,
  Button,
  Space,
  Alert,
  Tag,
  Switch,
  InputNumber,
  Divider,
  List,
  Typography,
  Spin,
  message,
  Row,
  Col,
} from 'antd'
import {
  PlayCircleOutlined,
  StopOutlined,
  ReloadOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  LoadingOutlined,
  SyncOutlined,
} from '@ant-design/icons'
import { crawlerApi } from '@/api/crawler'
import type {
  MediaCrawlerStartRequest,
  CrawlerStatus,
  CrawlerLog,
  Platform,
  ConfigOptions,
} from '@/types'
import PageHeader from '@/components/common/PageHeader'
import page from '@/styles/page.module.css'

const { Text } = Typography

const CrawlerPage: React.FC = () => {
  const [form] = Form.useForm()
  const [platforms, setPlatforms] = useState<Platform[]>([])
  const [options, setOptions] = useState<ConfigOptions | null>(null)
  const [status, setStatus] = useState<CrawlerStatus | null>(null)
  const [logs, setLogs] = useState<CrawlerLog[]>([])
  const [loading, setLoading] = useState(false)
  const [initializing, setInitializing] = useState(true)
  const logsEndRef = useRef<HTMLDivElement>(null)
  const pollTimerRef = useRef<number>()

  // 初始化：加载平台和配置选项
  useEffect(() => {
    const init = async () => {
      try {
        const [platformsRes, optionsRes] = await Promise.all([
          crawlerApi.getPlatforms(),
          crawlerApi.getOptions(),
        ])
        setPlatforms(platformsRes.platforms)
        setOptions(optionsRes)

        // 设置默认值
        form.setFieldsValue({
          platform: 'xhs',
          login_type: 'qrcode',
          crawler_type: 'search',
          save_option: 'db',
          enable_comments: true,
          enable_sub_comments: false,
          headless: false,
          start_page: 1,
        })
      } catch (error) {
        console.error('初始化失败:', error)
        message.error('加载配置失败')
      } finally {
        setInitializing(false)
      }
    }
    init()
  }, [form])

  // 轮询状态和日志
  useEffect(() => {
    const poll = async () => {
      try {
        const [statusRes, logsRes] = await Promise.all([
          crawlerApi.getStatus(),
          crawlerApi.getLogs(100),
        ])
        setStatus(statusRes)
        setLogs(logsRes.logs)
      } catch (error) {
        console.error('轮询失败:', error)
      }
    }

    poll() // 立即执行一次
    pollTimerRef.current = window.setInterval(poll, 2000) // 每2秒轮询

    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current)
      }
    }
  }, [])

  // 启动爬虫
  const handleStart = async (values: any) => {
    setLoading(true)
    try {
      await crawlerApi.start(values as MediaCrawlerStartRequest)
      message.success('爬虫启动成功')
    } catch (error: any) {
      message.error(error.response?.data?.detail || '启动失败')
    } finally {
      setLoading(false)
    }
  }

  // 停止爬虫
  const handleStop = async () => {
    try {
      await crawlerApi.stop()
      message.success('爬虫已停止')
    } catch (error: any) {
      message.error(error.response?.data?.detail || '停止失败')
    }
  }

  // 清空日志
  const handleClearLogs = () => {
    setLogs([])
    message.success('日志已清空')
  }

  // 根据爬虫类型显示不同的输入框
  const crawlerType = Form.useWatch('crawler_type', form)

  const getLogLevelColor = (level: string) => {
    switch (level) {
      case 'error':
        return 'red'
      case 'warning':
        return 'orange'
      case 'success':
        return 'green'
      case 'debug':
        return 'gray'
      default:
        return 'blue'
    }
  }

  const getStatusTag = () => {
    if (!status) return null

    switch (status.status) {
      case 'running':
        return (
          <Tag icon={<SyncOutlined spin />} color="processing">
            运行中
          </Tag>
        )
      case 'idle':
        return (
          <Tag icon={<CheckCircleOutlined />} color="success">
            空闲
          </Tag>
        )
      case 'error':
        return (
          <Tag icon={<CloseCircleOutlined />} color="error">
            错误
          </Tag>
        )
      case 'stopping':
        return (
          <Tag icon={<LoadingOutlined />} color="warning">
            停止中
          </Tag>
        )
      default:
        return <Tag>{status.status}</Tag>
    }
  }

  if (initializing) {
    return (
      <div style={{ textAlign: 'center', padding: 100 }}>
        <Spin size="large" tip="加载中..." />
      </div>
    )
  }

  return (
    <div className={page.container}>
      <PageHeader
        title="MediaCrawler 爬虫管理"
        subtitle="多平台社交媒体数据采集工具，支持小红书、抖音、B站、微博等7大平台"
      />

      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        {/* 状态卡片 */}
        <Card title="爬虫状态" extra={getStatusTag()}>
          {status && status.status === 'running' && (
            <Alert
              message={`正在爬取 ${status.platform} 平台 - ${status.crawler_type} 模式`}
              description={`开始时间: ${status.started_at}`}
              type="info"
              showIcon
            />
          )}
          {status && status.status === 'idle' && (
            <Alert message="爬虫空闲，可以启动新任务" type="success" showIcon />
          )}
          {status && status.error_message && (
            <Alert message={status.error_message} type="error" showIcon />
          )}
        </Card>

        {/* 配置表单 */}
        <Card title="爬虫配置">
          <Form
            form={form}
            layout="vertical"
            onFinish={handleStart}
            disabled={status?.status === 'running'}
          >
            <Form.Item
              name="platform"
              label="平台"
              rules={[{ required: true, message: '请选择平台' }]}
            >
              <Select
                placeholder="选择要爬取的平台"
                options={platforms.map((p) => ({
                  value: p.value,
                  label: p.label,
                }))}
              />
            </Form.Item>

            <Row gutter={16}>
              <Col span={12}>
                <Form.Item name="crawler_type" label="爬取类型" rules={[{ required: true }]}>
                  <Select options={options?.crawler_types} />
                </Form.Item>
              </Col>

              <Col span={12}>
                {crawlerType === 'search' && (
                  <Form.Item
                    name="keywords"
                    label="关键词"
                    rules={[{ required: true, message: '请输入关键词' }]}
                  >
                    <Input placeholder="多个关键词用逗号分隔，例如：赛尔号,摩尔庄园" />
                  </Form.Item>
                )}

                {crawlerType === 'detail' && (
                  <Form.Item
                    name="specified_ids"
                    label="指定ID"
                    rules={[{ required: true, message: '请输入ID' }]}
                  >
                    <Input placeholder="多个ID用逗号分隔" />
                  </Form.Item>
                )}

                {crawlerType === 'creator' && (
                  <Form.Item
                    name="creator_ids"
                    label="创作者ID"
                    rules={[{ required: true, message: '请输入创作者ID' }]}
                  >
                    <Input placeholder="多个ID用逗号分隔" />
                  </Form.Item>
                )}
              </Col>
            </Row>

            <Row gutter={16}>
              <Col span={8}>
                <Form.Item name="login_type" label="登录方式">
                  <Select options={options?.login_types} />
                </Form.Item>
              </Col>

              <Col span={8}>
                <Form.Item name="save_option" label="存储方式">
                  <Select options={options?.save_options} />
                </Form.Item>
              </Col>

              <Col span={8}>
                <Form.Item name="start_page" label="起始页">
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
            </Row>

            <Row gutter={16}>
              <Col span={8}>
                <Form.Item name="enable_comments" label="爬取评论" valuePropName="checked">
                  <Switch />
                </Form.Item>
              </Col>

              <Col span={8}>
                <Form.Item
                  name="enable_sub_comments"
                  label="爬取二级评论"
                  valuePropName="checked"
                >
                  <Switch />
                </Form.Item>
              </Col>

              <Col span={8}>
                <Form.Item name="headless" label="无头模式" valuePropName="checked">
                  <Switch />
                </Form.Item>
              </Col>
            </Row>

            <Divider />

            <Space>
              <Button
                type="primary"
                htmlType="submit"
                icon={<PlayCircleOutlined />}
                loading={loading}
                disabled={status?.status === 'running'}
              >
                启动爬虫
              </Button>
              <Button
                danger
                icon={<StopOutlined />}
                onClick={handleStop}
                disabled={status?.status !== 'running'}
              >
                停止爬虫
              </Button>
            </Space>
          </Form>
        </Card>

        {/* 实时日志 */}
        <Card
          title={
            <Space>
              <span>实时日志</span>
              <Text type="secondary">({logs.length} 条)</Text>
            </Space>
          }
          extra={
            <Button size="small" icon={<ReloadOutlined />} onClick={handleClearLogs}>
              清空
            </Button>
          }
        >
          <div
            style={{
              height: 400,
              overflow: 'auto',
              background: '#000',
              padding: 16,
              borderRadius: 4,
              fontFamily: 'Consolas, Monaco, monospace',
            }}
          >
            {logs.length === 0 ? (
              <Text style={{ color: '#666' }}>暂无日志</Text>
            ) : (
              <List
                dataSource={logs}
                renderItem={(log) => (
                  <div style={{ marginBottom: 8 }}>
                    <Text style={{ color: '#666', marginRight: 8 }}>{log.timestamp}</Text>
                    <Tag
                      color={getLogLevelColor(log.level)}
                      style={{ marginRight: 8, minWidth: 60, textAlign: 'center' }}
                    >
                      {log.level.toUpperCase()}
                    </Tag>
                    <Text style={{ color: '#fff' }}>{log.message}</Text>
                  </div>
                )}
              />
            )}
            <div ref={logsEndRef} />
          </div>
        </Card>
      </Space>
    </div>
  )
}

export default CrawlerPage

import React, { useCallback, useEffect, useState } from 'react'
import {
  Badge,
  Button,
  Card,
  Col,
  Descriptions,
  Row,
  Space,
  Spin,
  Statistic,
  Tag,
  Tooltip,
  Typography,
  Table,
} from 'antd'
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  DatabaseOutlined,
  KeyOutlined,
  LoadingOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
  CloudServerOutlined,
  ApiOutlined,
} from '@ant-design/icons'
import { adminRagApi } from '@/api/admin-rag'
import { adminSystemApi } from '@/api/admin-system'
import type { RagStatus, SystemHealth, TaggerConfig, PlatformDiff } from '@/types'
import { platformLabel } from '@/utils/platform'
import PageHeader from '@/components/common/PageHeader'
import page from '@/styles/page.module.css'
import dayjs from 'dayjs'

const { Text } = Typography

const StatusTag: React.FC<{ probe?: { ok: boolean; message?: string; latencyMs: number } | null; nullText?: string }> = ({ probe, nullText = '-' }) => {
  if (!probe) return <Tag>{nullText}</Tag>
  if (probe.ok) return <Tag icon={<CheckCircleOutlined />} color="success">正常 {probe.latencyMs}ms</Tag>
  return (
    <Tooltip title={probe.message}>
      <Tag icon={<CloseCircleOutlined />} color="error">{probe.message && probe.message.length < 20 ? probe.message : '异常'}</Tag>
    </Tooltip>
  )
}

const SystemPage: React.FC = () => {
  const [health, setHealth] = useState<SystemHealth | null>(null)
  const [tagger, setTagger] = useState<TaggerConfig | null>(null)
  const [ragStatus, setRagStatus] = useState<RagStatus | null>(null)
  const [loading, setLoading] = useState(false)

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      const [h, c, rs] = await Promise.all([
        adminSystemApi.health(),
        adminSystemApi.config().catch(() => null),
        adminRagApi.status().catch(() => null),
      ])
      setHealth(h)
      if (c?.tagger) setTagger(c.tagger)
      setRagStatus(rs as RagStatus | null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void fetchAll() }, [fetchAll])

  const milvusReachable = (ragStatus as any)?.milvusReachable as boolean | undefined
  const milvusError = (ragStatus as any)?.milvusError as string | undefined
  const collectionExists = (ragStatus as any)?.collectionExists as boolean | undefined
  const embedderReady = (ragStatus as any)?.embedderReady as boolean | undefined

  return (
    <div className={page.pageShell}>
      <PageHeader
        title="系统状态"
        subtitle="各服务健康状态、待处理队列及数据同步概览"
        icon={<ThunderboltOutlined />}
        extra={
          <Button icon={<ReloadOutlined />} onClick={() => void fetchAll()} loading={loading} className={page.ghostBtn}>
            刷新
          </Button>
        }
      />

      {loading && !health ? (
        <Spin style={{ display: 'block', margin: '40px auto' }} />
      ) : (
        <>
          {/* ── 基础设施健康 ─────────────────────────────────────────── */}
          <Row gutter={[20, 20]}>
            <Col xs={24} sm={8} md={8} className={page.colStretch}>
              <Card bordered={false} className={page.panelCard} style={{ flex: 1 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <Space>
                    <DatabaseOutlined style={{ color: '#1677ff' }} />
                    <Text strong>MySQL</Text>
                  </Space>
                  <StatusTag probe={health?.database} />
                </div>
                {health?.database?.message && !health.database.ok && (
                  <Text type="danger" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>{health.database.message}</Text>
                )}
              </Card>
            </Col>

            <Col xs={24} sm={8} md={8} className={page.colStretch}>
              <Card bordered={false} className={page.panelCard} style={{ flex: 1 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <Space>
                    <DatabaseOutlined style={{ color: '#fa8c16' }} />
                    <Text strong>Redis</Text>
                  </Space>
                  <StatusTag probe={health?.redis} nullText="未配置" />
                </div>
                {health?.redis?.message && !health.redis.ok && (
                  <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>{health.redis.message}</Text>
                )}
              </Card>
            </Col>

            <Col xs={24} sm={8} md={8} className={page.colStretch}>
              <Card bordered={false} className={page.panelCard} style={{ flex: 1 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <Space>
                    <CloudServerOutlined style={{ color: '#722ed1' }} />
                    <Text strong>Milvus</Text>
                  </Space>
                  {ragStatus === null ? <Tag>-</Tag>
                    : milvusReachable
                      ? <Tag icon={<CheckCircleOutlined />} color="success">
                          {collectionExists ? '正常（已就绪）' : '正常（未初始化）'}
                        </Tag>
                      : <Tooltip title={milvusError}>
                          <Tag icon={<CloseCircleOutlined />} color="error">不可达</Tag>
                        </Tooltip>
                  }
                </div>
                {milvusError && (
                  <Text type="danger" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>{milvusError}</Text>
                )}
              </Card>
            </Col>
          </Row>

          {/* ── 数据队列指标 ─────────────────────────────────────────── */}
          <Row gutter={[20, 20]}>
            <Col xs={12} sm={6} md={6} className={page.colStretch}>
              <Card bordered={false} className={`${page.panelCard} ${page.statCard}`} style={{ flex: 1 }}>
                <Statistic title="待 AI 打标" value={health?.pendingTagging ?? '-'} suffix="篇" />
              </Card>
            </Col>
            <Col xs={12} sm={6} md={6} className={page.colStretch}>
              <Card bordered={false} className={`${page.panelCard} ${page.statCard}`} style={{ flex: 1 }}>
                <Statistic title="待向量化" value={health?.pendingEmbed ?? '-'} suffix="篇" />
              </Card>
            </Col>
            <Col xs={12} sm={6} md={6} className={page.colStretch}>
              <Card bordered={false} className={`${page.panelCard} ${page.statCard}`} style={{ flex: 1 }}>
                <Statistic title="中心库文章" value={health?.totalArticles ?? '-'} suffix="篇" />
              </Card>
            </Col>
            <Col xs={12} sm={6} md={6} className={page.colStretch}>
              <Card bordered={false} className={page.panelCard} style={{ flex: 1 }} title="最近爬取">
                {health?.lastCrawlerRun ? (
                  <div style={{ fontSize: 12 }}>
                    <div><Text type="secondary">{health.lastCrawlerRun.spiders}</Text></div>
                    <div style={{ marginTop: 4 }}>
                      <Badge
                        status={health.lastCrawlerRun.status === 'success' ? 'success' : health.lastCrawlerRun.status === 'failed' ? 'error' : 'processing'}
                        text={health.lastCrawlerRun.status}
                      />
                    </div>
                    <div style={{ marginTop: 4, color: '#888' }}>{dayjs(health.lastCrawlerRun.startedAt).format('MM-DD HH:mm')}</div>
                  </div>
                ) : (
                  <Text type="secondary">无记录</Text>
                )}
              </Card>
            </Col>
          </Row>

          {/* ── 平台数据同步差异 ──────────────────────────────────────── */}
          {(health?.platformDiffs?.length ?? 0) > 0 && (
            <Card
              bordered={false}
              className={page.panelCard}
              title={<><DatabaseOutlined style={{ marginRight: 8 }} />平台数据同步差异</>}
              extra={<Text type="secondary" style={{ fontSize: 12 }}>源表 = MediaCrawler 原始数据；中心库 = articles 表</Text>}
            >
              <Table<PlatformDiff>
                dataSource={health!.platformDiffs}
                rowKey="code"
                size="small"
                pagination={false}
                columns={[
                  {
                    title: '平台',
                    dataIndex: 'code',
                    width: 100,
                    render: (code: string) => platformLabel(code),
                  },
                  {
                    title: '源表',
                    dataIndex: 'table',
                    width: 160,
                    render: (t: string) => <Text code style={{ fontSize: 12 }}>{t}</Text>,
                  },
                  {
                    title: '源表行数',
                    dataIndex: 'source',
                    align: 'right',
                    render: (v: number) => v < 0 ? <Text type="secondary">不可访问</Text> : v.toLocaleString(),
                  },
                  {
                    title: '中心库行数',
                    dataIndex: 'central',
                    align: 'right',
                    render: (v: number) => v.toLocaleString(),
                  },
                  {
                    title: '未同步',
                    dataIndex: 'diff',
                    align: 'right',
                    render: (v: number, r: PlatformDiff) => {
                      if (r.source < 0) return <Text type="secondary">-</Text>
                      if (v <= 0) return <Tag color="success">已同步</Tag>
                      return <Tag color={v > 1000 ? 'error' : 'warning'}>{v.toLocaleString()} 条</Tag>
                    },
                  },
                ]}
              />
            </Card>
          )}

          {/* ── 大模型 API ───────────────────────────────────────────── */}
          <Card
            bordered={false}
            className={page.panelCard}
            title={<><ApiOutlined style={{ marginRight: 8 }} />大模型 API</>}
            extra={<StatusTag probe={health?.llm} />}
          >
            {tagger ? (
              <Descriptions size="small" column={{ xs: 1, sm: 2, md: 4 }}>
                <Descriptions.Item label="模型">
                  <Text code>{tagger.llmModel || '-'}</Text>
                </Descriptions.Item>
                <Descriptions.Item label="打标任务">
                  {tagger.enabled
                    ? <Badge status="processing" text="运行中" />
                    : <Badge status="default" text="已停用" />}
                </Descriptions.Item>
                <Descriptions.Item label="API Key">
                  {tagger.apiKeySet
                    ? <Tag icon={<KeyOutlined />} color="blue">已配置 {tagger.llmApiKey}</Tag>
                    : <Tag color="warning">未配置</Tag>}
                </Descriptions.Item>
                <Descriptions.Item label="批处理">
                  {tagger.intervalSeconds}s 轮询 / 批量 {tagger.batchSize} 条
                </Descriptions.Item>
                <Descriptions.Item label="Base URL" span={4}>
                  <Text type="secondary" style={{ fontSize: 12, wordBreak: 'break-all' }}>{tagger.llmBaseUrl || '-'}</Text>
                </Descriptions.Item>
              </Descriptions>
            ) : (
              <Text type="secondary">加载中…</Text>
            )}
            {health?.llm?.message && !health.llm.ok && (
              <Text type="danger" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>{health.llm.message}</Text>
            )}
          </Card>

          {/* ── RAG 向量服务 ─────────────────────────────────────────── */}
          <Row gutter={[20, 20]}>
            <Col xs={24} sm={12} md={8} className={page.colStretch}>
              <Card
                bordered={false}
                className={page.panelCard}
                style={{ flex: 1 }}
                title="Embedding 服务"
                extra={
                  ragStatus?.serviceReachable
                    ? embedderReady
                      ? <Tag icon={<CheckCircleOutlined />} color="success">已就绪</Tag>
                      : <Tag icon={<LoadingOutlined spin />} color="processing">加载中</Tag>
                    : <Tag icon={<CloseCircleOutlined />} color="warning">不可达</Tag>
                }
              >
                <Descriptions size="small" column={1}>
                  <Descriptions.Item label="来源">
                    {ragStatus?.embedProvider === 'api'
                      ? <Tag color="purple">第三方 API</Tag>
                      : <Tag color="blue">本地模型</Tag>}
                  </Descriptions.Item>
                  <Descriptions.Item label="模型">
                    {ragStatus?.embedModel ? <Text code style={{ fontSize: 12 }}>{ragStatus.embedModel}</Text> : <Text type="secondary">-</Text>}
                  </Descriptions.Item>
                  <Descriptions.Item label="向量维度">
                    {ragStatus?.embedDim ? ragStatus.embedDim : <Text type="secondary">未加载</Text>}
                  </Descriptions.Item>
                  {ragStatus?.serviceError && (
                    <Descriptions.Item label="错误">
                      <Text type="danger" style={{ fontSize: 12 }}>{ragStatus.serviceError}</Text>
                    </Descriptions.Item>
                  )}
                </Descriptions>
              </Card>
            </Col>

            <Col xs={24} sm={12} md={8} className={page.colStretch}>
              <Card bordered={false} className={page.panelCard} style={{ flex: 1 }} title="Milvus 集合">
                <Descriptions size="small" column={1}>
                  <Descriptions.Item label="集合名">
                    <Text code style={{ fontSize: 12 }}>{ragStatus?.collection || '-'}</Text>
                  </Descriptions.Item>
                  <Descriptions.Item label="集合维度">
                    {(ragStatus as any)?.collectionDim
                      ? (ragStatus as any).collectionDim
                      : <Text type="secondary">-</Text>}
                  </Descriptions.Item>
                  <Descriptions.Item label="Milvus URI">
                    <Text type="secondary" style={{ fontSize: 12, wordBreak: 'break-all' }}>{ragStatus?.milvusUri || '-'}</Text>
                  </Descriptions.Item>
                </Descriptions>
              </Card>
            </Col>

            <Col xs={24} sm={12} md={8} className={page.colStretch}>
              <Card bordered={false} className={page.panelCard} style={{ flex: 1 }} title="同步配置">
                <Descriptions size="small" column={1}>
                  <Descriptions.Item label="RAG 检索">
                    {ragStatus?.ragEnabled
                      ? <Tag color="blue">已启用</Tag>
                      : <Tag>未启用</Tag>}
                  </Descriptions.Item>
                  <Descriptions.Item label="自动同步">
                    {(ragStatus as any)?.syncEnabled
                      ? <Badge status="processing" text="开启" />
                      : <Badge status="default" text="关闭" />}
                  </Descriptions.Item>
                  <Descriptions.Item label="同步周期">
                    {ragStatus?.syncIntervalSecondsHint != null ? `${ragStatus.syncIntervalSecondsHint}s` : '-'}
                  </Descriptions.Item>
                  {ragStatus?.processManaged && (
                    <Descriptions.Item label="进程">
                      <Space size={4}>
                        <Tag color="blue">Go 托管</Tag>
                        {ragStatus.processPid != null && ragStatus.processPid > 0 && (
                          <Text type="secondary" style={{ fontSize: 12 }}>PID {ragStatus.processPid}</Text>
                        )}
                      </Space>
                    </Descriptions.Item>
                  )}
                  <Descriptions.Item label="服务地址">
                    <Text type="secondary" style={{ fontSize: 12, wordBreak: 'break-all' }}>
                      {ragStatus?.embeddingServiceUrl || '未配置'}
                    </Text>
                  </Descriptions.Item>
                </Descriptions>
              </Card>
            </Col>
          </Row>
        </>
      )}
    </div>
  )
}

export default SystemPage

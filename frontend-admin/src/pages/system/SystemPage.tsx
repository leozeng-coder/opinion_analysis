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
} from 'antd'
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  DatabaseOutlined,
  KeyOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { adminRagApi } from '@/api/admin-rag'
import { adminSystemApi } from '@/api/admin-system'
import type { RagStatus, SystemHealth, TaggerConfig } from '@/types'
import dayjs from 'dayjs'

const { Title, Text } = Typography

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

  const llmProbe = health?.llm

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
          {/* ── 顶部状态卡 ────────────────────────────────────────────────── */}
          <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
            {/* DB */}
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

            {/* LLM */}
            <Col xs={24} sm={24} md={12} style={{ display: 'flex' }}>
              <Card
                size="small"
                style={{ flex: 1 }}
                styles={{ body: { padding: '12px 16px' } }}
                extra={
                  llmProbe ? (
                    llmProbe.ok ? (
                      <Tag icon={<CheckCircleOutlined />} color="success">连通 {llmProbe.latencyMs}ms</Tag>
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
                        : <Badge status="default" text="后台任务已停用" />}
                    </Descriptions.Item>
                    <Descriptions.Item label="Base URL" span={2}>
                      <Text type="secondary" style={{ fontSize: 12, wordBreak: 'break-all' }}>
                        {tagger.llmBaseUrl || '-'}
                      </Text>
                    </Descriptions.Item>
                    <Descriptions.Item label="API Key">
                      {tagger.apiKeySet
                        ? <Tag icon={<KeyOutlined />} color="blue">已配置 {tagger.llmApiKey}</Tag>
                        : <Tag color="warning">未配置</Tag>}
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

          {/* ── RAG 服务状态 ──────────────────────────────────────────────── */}
          <Card
            title={
              <span>
                <DatabaseOutlined style={{ marginRight: 8 }} />
                向量知识库（RAG 服务）
              </span>
            }
            extra={null}
          >
            {ragStatus?.note && (
              <Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 12 }}>
                {ragStatus.note}
              </Text>
            )}
            <Descriptions size="small" column={{ xs: 1, sm: 2, md: 3 }}>
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
              <Descriptions.Item label="模型向量维度">{ragStatus?.embedDim ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="Milvus 集合维度">
                {ragStatus?.collectionDim != null ? (
                  ragStatus.dimensionMismatch
                    ? <Text type="danger">{ragStatus.collectionDim}</Text>
                    : ragStatus.collectionDim
                ) : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="Milvus 集合">{ragStatus?.collection || '-'}</Descriptions.Item>
              <Descriptions.Item label="RAG 服务">
                {ragStatus?.serviceReachable
                  ? <Tag icon={<CheckCircleOutlined />} color="success">可达</Tag>
                  : <Tag icon={<CloseCircleOutlined />} color="warning">不可达或未配置</Tag>}
              </Descriptions.Item>
              {ragStatus?.processManaged && (
                <Descriptions.Item label="进程托管">
                  <Space size={4}>
                    <Tag color="blue">Go 托管</Tag>
                    {ragStatus.processPid != null && ragStatus.processPid > 0 && (
                      <Text type="secondary" style={{ fontSize: 12 }}>PID {ragStatus.processPid}</Text>
                    )}
                  </Space>
                </Descriptions.Item>
              )}
              <Descriptions.Item label="同步周期（参考）">
                {ragStatus?.syncIntervalSecondsHint != null ? `${ragStatus.syncIntervalSecondsHint}s` : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="服务地址" span={3}>
                <Text type="secondary" style={{ fontSize: 12, wordBreak: 'break-all' }}>
                  {ragStatus?.embeddingServiceUrl || '未配置 config.rag.embedding_service_url'}
                </Text>
              </Descriptions.Item>
              {ragStatus?.embedderError && (
                <Descriptions.Item label="嵌入模型" span={3}>
                  <Text type="warning" style={{ fontSize: 12 }}>{ragStatus.embedderError}</Text>
                </Descriptions.Item>
              )}
              {ragStatus?.serviceError && (
                <Descriptions.Item label="健康检查" span={3}>
                  <Text type="danger" style={{ fontSize: 12 }}>{ragStatus.serviceError}</Text>
                </Descriptions.Item>
              )}
            </Descriptions>
          </Card>
        </>
      )}
    </div>
  )
}

export default SystemPage

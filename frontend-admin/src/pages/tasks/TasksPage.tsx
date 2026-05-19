import React, { useCallback, useEffect, useRef, useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  Descriptions,
  Divider,
  Row,
  Space,
  Statistic,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  DatabaseOutlined,
  RobotOutlined,
  SyncOutlined,
} from '@ant-design/icons'
import { adminRagApi } from '@/api/admin-rag'
import { taggerApi } from '@/api/tagger'
import { adminAuditApi } from '@/api/admin-audit'
import { adminSystemApi } from '@/api/admin-system'
import type { AuditLog, RagStatus, RagSyncLog, TaggerConfig } from '@/types'
import dayjs from 'dayjs'

const { Title, Text } = Typography

const TasksPage: React.FC = () => {
  // tagger
  const [pending, setPending] = useState<number | null>(null)
  const [lastRun, setLastRun] = useState<AuditLog | null>(null)
  const [taggerCfg, setTaggerCfg] = useState<TaggerConfig | null>(null)
  const [triggering, setTriggering] = useState(false)
  const refreshRef = useRef<ReturnType<typeof window.setInterval> | null>(null)

  // RAG sync
  const [ragStatus, setRagStatus] = useState<RagStatus | null>(null)
  const [pendingEmbed, setPendingEmbed] = useState<number | null>(null)
  const [ragSyncEnabled, setRagSyncEnabled] = useState<boolean>(true)
  const [syncToggling, setSyncToggling] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [ragRuns, setRagRuns] = useState<RagSyncLog[]>([])
  const [ragTotal, setRagTotal] = useState(0)
  const [ragPage, setRagPage] = useState(1)
  const [ragLoading, setRagLoading] = useState(false)

  const [loading, setLoading] = useState(false)

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
        adminRagApi.status()
          .then((rs) => {
            setRagStatus(rs)
            if (rs.syncEnabled != null) setRagSyncEnabled(rs.syncEnabled)
          })
          .catch(() => {}),
        adminRagApi.listArticles({ synced: 'no', page_size: 1 })
          .then((r) => setPendingEmbed(r.total))
          .catch(() => {}),
        loadRagRuns(1),
      ])
    } finally {
      setLoading(false)
    }
  }, [loadRagRuns])

  useEffect(() => {
    void fetchAll()
    refreshRef.current = window.setInterval(() => {
      void taggerApi.pending().then((r) => setPending(r.pending)).catch(() => {})
      void adminRagApi.listArticles({ synced: 'no', page_size: 1 }).then((r) => setPendingEmbed(r.total)).catch(() => {})
      void loadRagRuns(ragPage)
      void adminRagApi.status().then(setRagStatus).catch(() => undefined)
    }, 12_000)
    return () => { if (refreshRef.current) window.clearInterval(refreshRef.current) }
  }, [fetchAll, loadRagRuns, ragPage])

  // ── tagger handlers ───────────────────────────────────────────────────────

  const handleTriggerTagger = async () => {
    setTriggering(true)
    try {
      const res = await taggerApi.run()
      void message.success(res.message)
      window.setTimeout(() => void fetchAll(), 2000)
    } finally {
      setTriggering(false)
    }
  }

  // ── RAG sync handlers ─────────────────────────────────────────────────────

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

  const handleRagSync = async () => {
    if (ragStatus?.dimensionMismatch) {
      message.warning('向量维度与 Milvus 集合不一致，请先到「系统配置」执行「重建向量库并同步」')
      return
    }
    setSyncing(true)
    try {
      await adminRagApi.triggerSync()
      message.success('已提交向量同步，可在下方列表查看进度')
      await loadRagRuns(1)
      void adminRagApi.status().then(setRagStatus).catch(() => undefined)
    } catch (e: unknown) {
      console.error(e)
      const err = e as { response?: { data?: { message?: string } }; message?: string }
      message.error(err.response?.data?.message || err.message || '同步提交失败')
    } finally {
      setSyncing(false)
    }
  }

  const isEnabled = taggerCfg?.enabled ?? false

  return (
    <div>
      <Title level={4} style={{ marginTop: 0, marginBottom: 24 }}>
        <SyncOutlined style={{ marginRight: 8 }} />任务管理
      </Title>

      {/* ── Section 1: AI 打标任务 ──────────────────────────────────────── */}
      <Card
        style={{ marginBottom: 24 }}
        title={
          <span>
            <RobotOutlined style={{ marginRight: 8 }} />
            AI 打标任务
          </span>
        }
      >
        <Row gutter={[16, 16]} style={{ marginBottom: 20 }}>
          <Col xs={24} sm={8}>
            <Card size="small" style={{ textAlign: 'center' }}>
              <Statistic
                title="当前待打标文章"
                value={pending ?? '-'}
                suffix="篇"
                loading={loading}
                valueStyle={{ color: pending && pending > 0 ? '#fa8c16' : '#52c41a', fontSize: 32 }}
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
                <Text type="secondary" style={{ fontSize: 12 }}>
                  上限 {taggerCfg?.maxPerTick ?? '-'} 条/轮
                </Text>
              </div>
            </Card>
          </Col>
        </Row>

        <Row gutter={[16, 16]}>
          <Col xs={24} md={10}>
            <Card
              size="small"
              title={<span><SyncOutlined style={{ marginRight: 6 }} />手动触发</span>}
              style={{ height: '100%' }}
            >
              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                <Text type="secondary" style={{ fontSize: 13 }}>
                  后台每 {taggerCfg?.intervalSeconds ?? 120} 秒自动轮询一次；
                  点击下方按钮可立即执行一轮，在后台异步运行，不影响当前页面。
                </Text>
                {!isEnabled && (
                  <Tag color="warning" style={{ width: 'fit-content' }}>
                    后台任务已停用，手动触发仍可执行
                  </Tag>
                )}
                {!taggerCfg?.apiKeySet && (
                  <Tag color="error" style={{ width: 'fit-content' }}>
                    API Key 未配置，请先到「系统配置」完成设置
                  </Tag>
                )}
                <Button
                  type="primary"
                  size="large"
                  icon={<SyncOutlined spin={triggering} />}
                  loading={triggering}
                  disabled={!taggerCfg?.apiKeySet}
                  onClick={() => void handleTriggerTagger()}
                  style={{ alignSelf: 'flex-start' }}
                >
                  立即触发一轮打标
                </Button>
                <Text type="secondary" style={{ fontSize: 12 }}>待打标数量每 12 秒自动刷新</Text>
              </div>
            </Card>
          </Col>

          <Col xs={24} md={14}>
            <Card
              size="small"
              title={<span><CheckCircleOutlined style={{ marginRight: 6 }} />最近一次手动触发记录</span>}
              style={{ height: '100%' }}
            >
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
                    完整历史请查看
                    <a href="/audit" style={{ marginLeft: 4 }}>审计日志</a>
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

      {/* ── Section 2: RAG 向量同步 ──────────────────────────────────────── */}
      <Card
        title={
          <span>
            <DatabaseOutlined style={{ marginRight: 8 }} />
            RAG 向量同步
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
                onChange={(v) => void handleRagSyncToggle(v)}
                checkedChildren="开"
                unCheckedChildren="关"
              />
            </Space>
            <Button
              type="primary"
              size="small"
              icon={<SyncOutlined />}
              loading={syncing}
              disabled={!ragStatus?.embeddingServiceUrl || !!ragStatus?.dimensionMismatch}
              onClick={() => void handleRagSync()}
            >
              立即同步
            </Button>
            <Button size="small" onClick={() => void loadRagRuns(ragPage)} loading={ragLoading}>
              刷新
            </Button>
          </Space>
        }
      >
        <Row gutter={[16, 16]} style={{ marginBottom: 20 }}>
          <Col xs={24} sm={8}>
            <Card size="small" style={{ textAlign: 'center' }}>
              <Statistic
                title="待向量化文章"
                value={pendingEmbed ?? '-'}
                suffix="篇"
                loading={loading}
                valueStyle={{ color: pendingEmbed && pendingEmbed > 0 ? '#fa8c16' : '#52c41a', fontSize: 28 }}
              />
              <div style={{ marginTop: 4 }}>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {pendingEmbed === 0 ? '全部已同步' : '等待向量化同步'}
                </Text>
              </div>
            </Card>
          </Col>
          <Col xs={24} sm={8}>
            <Card size="small" style={{ textAlign: 'center' }}>
              <Statistic
                title="同步周期"
                value={ragStatus?.syncIntervalSecondsHint != null ? `${ragStatus.syncIntervalSecondsHint}s` : '-'}
                prefix={<ClockCircleOutlined />}
                valueStyle={{ fontSize: 28 }}
              />
              <div style={{ marginTop: 4 }}>
                {ragSyncEnabled
                  ? <Badge status="processing" text={<Text type="secondary" style={{ fontSize: 12 }}>定时同步运行中</Text>} />
                  : <Badge status="default" text={<Text type="secondary" style={{ fontSize: 12 }}>定时同步已暂停</Text>} />}
              </div>
            </Card>
          </Col>
          <Col xs={24} sm={8}>
            <Card size="small" style={{ textAlign: 'center' }}>
              <Statistic
                title="RAG 服务"
                value={ragStatus?.serviceReachable ? '可达' : '不可达'}
                valueStyle={{ color: ragStatus?.serviceReachable ? '#52c41a' : '#ff4d4f', fontSize: 28 }}
              />
              {ragStatus?.embedModel && (
                <div style={{ marginTop: 4 }}>
                  <Text type="secondary" style={{ fontSize: 11 }} ellipsis>{ragStatus.embedModel}</Text>
                </div>
              )}
            </Card>
          </Col>
        </Row>

        {ragStatus?.dimensionMismatch && (
          <Alert
            type="error"
            showIcon
            style={{ marginBottom: 16 }}
            message="向量维度不一致"
            description={
              <>
                当前嵌入模型为 {ragStatus.embedDim ?? '-'} 维，Milvus 集合为{' '}
                {ragStatus.collectionDim ?? '-'} 维，无法写入向量。
                请前往<Text strong>「系统配置 → 重建向量库并同步」</Text>修复。
              </>
            }
          />
        )}
        {ragStatus?.embedderError && !ragStatus?.dimensionMismatch && (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
            message="嵌入模型未就绪"
            description={ragStatus.embedderError}
          />
        )}

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
              title: '状态', dataIndex: 'status', width: 92,
              render: (s: RagSyncLog['status']) => (
                <Tag color={s === 'success' ? 'success' : s === 'failed' ? 'error' : 'processing'}>{s}</Tag>
              ),
            },
            { title: '方式', dataIndex: 'mode', width: 100 },
            { title: '进度', dataIndex: 'progress', width: 72, render: (p: number) => `${p}%` },
            { title: '文章数', dataIndex: 'articlesProcessed', width: 88 },
            { title: '写入块', dataIndex: 'chunksUpserted', width: 88 },
            { title: '清理', dataIndex: 'chunksDeleted', width: 72 },
            {
              title: '开始', dataIndex: 'startedAt', width: 128,
              render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss'),
            },
            {
              title: '结束', dataIndex: 'finishedAt', width: 128,
              render: (t: string | undefined) => t ? dayjs(t).format('MM-DD HH:mm:ss') : '-',
            },
            { title: '详情', dataIndex: 'progressDetail', ellipsis: true, render: (t: string) => t || '-' },
          ]}
        />
      </Card>
    </div>
  )
}

export default TasksPage

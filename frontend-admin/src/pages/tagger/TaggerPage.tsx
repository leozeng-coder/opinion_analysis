import React, { useCallback, useEffect, useRef, useState } from 'react'
import {
  Badge,
  Button,
  Card,
  Col,
  Descriptions,
  Divider,
  Row,
  Statistic,
  Tag,
  Typography,
  message,
} from 'antd'
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  RobotOutlined,
  SyncOutlined,
} from '@ant-design/icons'
import { taggerApi } from '@/api/tagger'
import { adminAuditApi } from '@/api/admin-audit'
import { adminSystemApi } from '@/api/admin-system'
import type { AuditLog, TaggerConfig } from '@/types'
import dayjs from 'dayjs'

const { Title, Text } = Typography

const TaggerPage: React.FC = () => {
  const [pending, setPending] = useState<number | null>(null)
  const [lastRun, setLastRun] = useState<AuditLog | null>(null)
  const [taggerCfg, setTaggerCfg] = useState<TaggerConfig | null>(null)
  const [loading, setLoading] = useState(false)
  const [triggering, setTriggering] = useState(false)
  const refreshRef = useRef<ReturnType<typeof window.setInterval> | null>(null)

  const fetchPending = useCallback(async () => {
    try {
      const res = await taggerApi.pending()
      setPending(res.pending)
    } catch { /* interceptor shows toast */ }
  }, [])

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      await Promise.all([
        fetchPending(),
        adminAuditApi.list({ resource: 'tagger', action: 'run', pageSize: 1 })
          .then(res => { if (res.list.length > 0) setLastRun(res.list[0]) })
          .catch(() => {}),
        adminSystemApi.config()
          .then(res => { if (res?.tagger) setTaggerCfg(res.tagger) })
          .catch(() => {}),
      ])
    } finally {
      setLoading(false)
    }
  }, [fetchPending])

  useEffect(() => {
    void fetchAll()
    refreshRef.current = window.setInterval(() => void fetchPending(), 30_000)
    return () => { if (refreshRef.current) window.clearInterval(refreshRef.current) }
  }, [fetchAll, fetchPending])

  const handleTrigger = async () => {
    setTriggering(true)
    try {
      const res = await taggerApi.run()
      void message.success(res.message)
      window.setTimeout(() => void fetchAll(), 2000)
    } finally {
      setTriggering(false)
    }
  }

  const isEnabled = taggerCfg?.enabled ?? false

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', marginBottom: 24 }}>
        <RobotOutlined style={{ fontSize: 22, marginRight: 10, color: '#1677ff' }} />
        <Title level={4} style={{ margin: 0 }}>AI 打标任务</Title>
      </div>

      {/* Stats row */}
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
                : <Badge status="default" text={<Text type="secondary" style={{ fontSize: 12 }}>已停用</Text>} />
              }
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

      {/* Action + Last run */}
      <Row gutter={[16, 16]}>
        <Col xs={24} md={10}>
          <Card
            title={
              <span>
                <SyncOutlined style={{ marginRight: 6 }} />
                手动触发
              </span>
            }
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
                  API Key 未配置，请先到系统状态页完成设置
                </Tag>
              )}

              <Button
                type="primary"
                size="large"
                icon={<SyncOutlined spin={triggering} />}
                loading={triggering}
                disabled={!taggerCfg?.apiKeySet}
                onClick={() => void handleTrigger()}
                style={{ alignSelf: 'flex-start' }}
              >
                立即触发一轮打标
              </Button>

              <Text type="secondary" style={{ fontSize: 12 }}>
                待打标数量每 30 秒自动刷新
              </Text>
            </div>
          </Card>
        </Col>

        <Col xs={24} md={14}>
          <Card
            title={
              <span>
                <CheckCircleOutlined style={{ marginRight: 6 }} />
                最近一次手动触发记录
              </span>
            }
            style={{ height: '100%' }}
          >
            {lastRun ? (
              <>
                <Descriptions column={2} size="small" labelStyle={{ color: '#888', width: 80 }}>
                  <Descriptions.Item label="触发时间" span={2}>
                    <Text>{dayjs(lastRun.createdAt).format('YYYY-MM-DD HH:mm:ss')}</Text>
                    <Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>
                      （{dayjs(lastRun.createdAt).format('MM-DD HH:mm')}）
                    </Text>
                  </Descriptions.Item>
                  <Descriptions.Item label="操作人">
                    {lastRun.actorName}
                  </Descriptions.Item>
                  <Descriptions.Item label="用户 ID">
                    {lastRun.actorId}
                  </Descriptions.Item>
                  <Descriptions.Item label="来源 IP">
                    {lastRun.ip || '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="路径">
                    <Text code style={{ fontSize: 11 }}>{lastRun.method} {lastRun.path}</Text>
                  </Descriptions.Item>
                </Descriptions>
                <Divider style={{ margin: '12px 0' }} />
                <Text type="secondary" style={{ fontSize: 12 }}>
                  完整历史记录请查看
                  <a
                    href="/audit"
                    onClick={e => { e.preventDefault(); window.location.href = '/audit' }}
                    style={{ marginLeft: 4 }}
                  >
                    审计日志
                  </a>
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
    </div>
  )
}

export default TaggerPage

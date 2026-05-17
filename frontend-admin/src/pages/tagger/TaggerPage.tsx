import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Alert, Button, Card, Col, Row, Statistic, Typography, message } from 'antd'
import { RobotOutlined, SyncOutlined } from '@ant-design/icons'
import { taggerApi } from '@/api/tagger'
import { adminAuditApi } from '@/api/admin-audit'
import type { AuditLog } from '@/types'
import dayjs from 'dayjs'

const { Title, Text, Paragraph } = Typography

const TaggerPage: React.FC = () => {
  const [pending, setPending] = useState<number | null>(null)
  const [lastRun, setLastRun] = useState<AuditLog | null>(null)
  const [loading, setLoading] = useState(false)
  const [triggering, setTriggering] = useState(false)
  const refreshRef = useRef<ReturnType<typeof window.setInterval> | null>(null)

  const fetchPending = useCallback(async () => {
    try {
      const res = await taggerApi.pending()
      setPending(res.pending)
    } catch { /* handled */ }
  }, [])

  const fetchLastRun = useCallback(async () => {
    try {
      const res = await adminAuditApi.list({ resource: 'tagger', action: 'run', pageSize: 1 })
      if (res.list.length > 0) setLastRun(res.list[0])
    } catch { /* handled */ }
  }, [])

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try { await Promise.all([fetchPending(), fetchLastRun()]) } finally { setLoading(false) }
  }, [fetchPending, fetchLastRun])

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
      // 延迟 2s 刷新，给后端时间更新状态
      window.setTimeout(() => void fetchAll(), 2000)
    } finally {
      setTriggering(false)
    }
  }

  return (
    <div>
      <Title level={4} style={{ marginTop: 0 }}>
        <RobotOutlined style={{ marginRight: 8 }} />AI 打标任务
      </Title>
      <Paragraph type="secondary">
        每 120 秒自动轮询未打标文章，此处可手动触发一轮。
        待打标数量每 30 秒自动刷新。
      </Paragraph>

      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} md={8}>
          <Card>
            <Statistic
              title="当前待打标文章"
              value={pending ?? '-'}
              suffix="篇"
              loading={loading}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8}>
          <Card style={{ display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
            <Button
              type="primary"
              size="large"
              icon={<SyncOutlined spin={triggering} />}
              loading={triggering}
              onClick={() => void handleTrigger()}
            >
              立即触发一轮打标
            </Button>
            <Text type="secondary" style={{ marginTop: 8, fontSize: 12 }}>
              触发后在后台异步执行，不阻塞当前页面
            </Text>
          </Card>
        </Col>
      </Row>

      {lastRun ? (
        <Card title="最近一次手动触发记录">
          <div style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '4px 16px', fontSize: 13 }}>
            <Text type="secondary">时间</Text>
            <Text>{dayjs(lastRun.createdAt).format('YYYY-MM-DD HH:mm:ss')}</Text>
            <Text type="secondary">操作人</Text>
            <Text>{lastRun.actorName} (ID:{lastRun.actorId})</Text>
            <Text type="secondary">来源 IP</Text>
            <Text>{lastRun.ip}</Text>
          </div>
        </Card>
      ) : (
        <Alert type="info" message="暂无手动触发记录" />
      )}
    </div>
  )
}

export default TaggerPage

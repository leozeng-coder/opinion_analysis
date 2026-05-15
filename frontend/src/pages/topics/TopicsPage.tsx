import React, { useEffect, useState } from 'react'
import { Card, Row, Col, Tag, Typography, Spin, Progress, Space } from 'antd'
import { FireOutlined, RiseOutlined, FallOutlined, MinusOutlined } from '@ant-design/icons'
import { topicApi } from '@/api/topic'
import type { Topic } from '@/types'

const { Title, Text } = Typography

const TREND_CONFIG = {
  rising: { icon: <RiseOutlined />, color: '#ff4d4f', label: '上升' },
  stable: { icon: <MinusOutlined />, color: '#faad14', label: '稳定' },
  falling: { icon: <FallOutlined />, color: '#52c41a', label: '下降' },
}

const TopicsPage: React.FC = () => {
  const [topics, setTopics] = useState<Topic[]>([])
  const [loading, setLoading] = useState(true)
  const [total, setTotal] = useState(0)

  useEffect(() => {
    topicApi.list({ page: 1, pageSize: 30 })
      .then(res => { setTopics(res.list); setTotal(res.total) })
      .finally(() => setLoading(false))
  }, [])

  const maxHeat = Math.max(...topics.map(t => t.heatScore), 1)

  if (loading) return <Spin size="large" style={{ display: 'block', marginTop: 80 }} />

  return (
    <div>
      <Title level={4} style={{ marginTop: 0, marginBottom: 16 }}>
        热点话题 <Text type="secondary" style={{ fontSize: 14 }}>共 {total} 个</Text>
      </Title>
      <Row gutter={[16, 16]}>
        {topics.map((topic, idx) => {
          const trend = TREND_CONFIG[topic.trend] ?? TREND_CONFIG.stable
          return (
            <Col xs={24} sm={12} lg={8} key={topic.id}>
              <Card
                size="small"
                hoverable
                title={
                  <Space>
                    <span style={{ color: '#ff4d4f', fontWeight: 'bold' }}>#{idx + 1}</span>
                    <FireOutlined style={{ color: '#fa8c16' }} />
                    <Text ellipsis style={{ maxWidth: 180 }}>{topic.name}</Text>
                  </Space>
                }
                extra={
                  <Tag color={trend.color} icon={trend.icon}>{trend.label}</Tag>
                }
              >
                <div style={{ marginBottom: 8 }}>
                  <Text type="secondary" style={{ fontSize: 12 }}>热度</Text>
                  <Progress
                    percent={Math.round((topic.heatScore / maxHeat) * 100)}
                    size="small"
                    strokeColor={{ from: '#fa8c16', to: '#ff4d4f' }}
                  />
                </div>
                <div style={{ marginBottom: 8 }}>
                  <Text type="secondary" style={{ fontSize: 12 }}>相关文章：</Text>
                  <Text strong>{topic.articleCount}</Text>
                </div>
                <div>
                  {(topic.keywords ?? []).slice(0, 5).map((kw) => (
                    <Tag key={kw} style={{ marginBottom: 4 }}>{kw}</Tag>
                  ))}
                </div>
              </Card>
            </Col>
          )
        })}
      </Row>
    </div>
  )
}

export default TopicsPage

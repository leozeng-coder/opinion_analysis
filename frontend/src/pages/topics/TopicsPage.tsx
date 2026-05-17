import React, { useEffect, useState } from 'react'
import { Card, Row, Col, Tag, Typography, Spin, Progress, Input, Empty } from 'antd'
import { FireOutlined, SearchOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { articleApi } from '@/api/article'
import type { TagCount } from '@/types'

const { Title, Text } = Typography

const TopicsPage: React.FC = () => {
  const [tags, setTags] = useState<TagCount[]>([])
  const [loading, setLoading] = useState(true)
  const [keyword, setKeyword] = useState('')
  const navigate = useNavigate()

  useEffect(() => {
    articleApi.tags({ limit: 100 })
      .then(res => setTags(res))
      .finally(() => setLoading(false))
  }, [])

  const filtered = keyword
    ? tags.filter(t => t.tag.includes(keyword))
    : tags

  const maxCount = Math.max(...filtered.map(t => t.count), 1)

  if (loading) return <Spin size="large" style={{ display: 'block', marginTop: 80 }} />

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}>
          热点话题 <Text type="secondary" style={{ fontSize: 14 }}>共 {filtered.length} 个</Text>
        </Title>
        <Input
          placeholder="搜索标签"
          prefix={<SearchOutlined />}
          allowClear
          style={{ width: 200 }}
          onChange={e => setKeyword(e.target.value.trim())}
        />
      </div>

      {filtered.length === 0 ? (
        <Empty description="暂无热点话题，待 AI 打标完成后自动更新" />
      ) : (
        <Row gutter={[16, 16]}>
          {filtered.map((item, idx) => (
            <Col xs={24} sm={12} lg={8} xl={6} key={item.tag}>
              <Card
                size="small"
                hoverable
                onClick={() => navigate(`/opinion?tags=${encodeURIComponent(item.tag)}`)}
                title={
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
                    <span style={{ color: idx < 3 ? '#ff4d4f' : '#faad14', fontWeight: 'bold', flexShrink: 0 }}>
                      #{idx + 1}
                    </span>
                    <FireOutlined style={{ color: '#fa8c16', flexShrink: 0 }} />
                    <Text ellipsis style={{ flex: 1 }} title={item.tag}>{item.tag}</Text>
                  </div>
                }
                extra={
                  <Tag color={idx < 3 ? 'red' : idx < 10 ? 'orange' : 'default'}>
                    {item.count} 篇
                  </Tag>
                }
              >
                <div style={{ marginBottom: 4 }}>
                  <Text type="secondary" style={{ fontSize: 12 }}>相关文章热度</Text>
                </div>
                <Progress
                  percent={Math.round((item.count / maxCount) * 100)}
                  size="small"
                  strokeColor={idx < 3 ? { from: '#ff4d4f', to: '#fa8c16' } : { from: '#1677ff', to: '#36cfc9' }}
                  format={() => `${item.count}`}
                />
              </Card>
            </Col>
          ))}
        </Row>
      )}
    </div>
  )
}

export default TopicsPage

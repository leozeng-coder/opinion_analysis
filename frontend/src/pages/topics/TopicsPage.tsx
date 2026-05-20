import React, { useEffect, useState } from 'react'
import { Card, Row, Col, Tag, Typography, Spin, Progress, Input, Empty } from 'antd'
import { FireOutlined, SearchOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { articleApi } from '@/api/article'
import PageHeader from '@/components/common/PageHeader'
import page from '@/styles/page.module.css'
import type { TagCount } from '@/types'
import styles from './TopicsPage.module.css'

const { Text } = Typography

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

  const filtered = keyword ? tags.filter(t => t.tag.includes(keyword)) : tags
  const maxCount = Math.max(...filtered.map(t => t.count), 1)

  if (loading) {
    return (
      <div className={page.pageShell}>
        <Spin size="large" style={{ display: 'block', margin: '80px auto' }} />
      </div>
    )
  }

  const rankTone = (idx: number) => {
    if (idx < 3) return styles.rankTop
    if (idx < 10) return styles.rankMid
    return styles.rankRest
  }

  const progressColor = (idx: number) =>
    idx < 3
      ? { from: '#ec6b6b', to: '#e8a84a' }
      : { from: '#4d93e8', to: '#42c48c' }

  return (
    <div className={page.pageShell}>
      <PageHeader
        title="热点话题"
        subtitle={`共 ${filtered.length} 个 AI 标签，点击卡片查看相关舆情`}
        icon={<FireOutlined />}
        extra={
          <Input
            placeholder="搜索标签"
            prefix={<SearchOutlined />}
            allowClear
            style={{ width: 220 }}
            onChange={e => setKeyword(e.target.value.trim())}
          />
        }
      />

      {filtered.length === 0 ? (
        <Empty description="暂无热点话题，待 AI 打标完成后自动更新" className={page.emptyCompact} />
      ) : (
        <Row gutter={[20, 20]} align="stretch">
          {filtered.map((item, idx) => (
            <Col xs={24} sm={12} lg={8} xl={6} key={item.tag} className={page.colStretch}>
              <Card
                bordered={false}
                className={`${page.panelCardHover} ${styles.topicCard}`}
                onClick={() => navigate(`/opinion?tags=${encodeURIComponent(item.tag)}`)}
                title={
                  <div className={styles.topicTitle}>
                    <span className={`${styles.topicRank} ${rankTone(idx)}`}>#{idx + 1}</span>
                    <FireOutlined className={styles.topicIcon} />
                    <Text ellipsis className={styles.topicName} title={item.tag}>{item.tag}</Text>
                  </div>
                }
                extra={<Tag className={page.softTagAmber}>{item.count} 篇</Tag>}
              >
                <Text type="secondary" className={styles.topicHint}>相关文章热度</Text>
                <Progress
                  percent={Math.round((item.count / maxCount) * 100)}
                  size="small"
                  strokeColor={progressColor(idx)}
                  format={() => `${item.count}`}
                  className={styles.topicProgress}
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

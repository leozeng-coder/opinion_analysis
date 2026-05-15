import React, { useEffect, useState, useCallback } from 'react'
import {
  Table, Input, Select, DatePicker, Space, Tag, Button,
  Typography, Drawer, Descriptions, Card,
} from 'antd'
import { SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { articleApi, type ArticleQuery } from '@/api/article'
import type { Article } from '@/types'

const { Title, Paragraph } = Typography
const { RangePicker } = DatePicker

const SENTIMENT_TAG: Record<string, { color: string; label: string }> = {
  positive: { color: 'success', label: '正面' },
  neutral: { color: 'default', label: '中性' },
  negative: { color: 'error', label: '负面' },
}

const PLATFORM_OPTIONS = [
  { value: '', label: '全部平台' },
  { value: 'weibo', label: '微博' },
  { value: 'weixin', label: '微信' },
  { value: 'news', label: '新闻' },
  { value: 'forum', label: '论坛' },
]

const OpinionPage: React.FC = () => {
  const [data, setData] = useState<Article[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [query, setQuery] = useState<ArticleQuery>({ page: 1, pageSize: 20 })
  const [keyword, setKeyword] = useState('')
  const [detail, setDetail] = useState<Article | null>(null)

  const fetchData = useCallback(async (q: ArticleQuery) => {
    setLoading(true)
    try {
      const res = await articleApi.list(q)
      setData(res.list)
      setTotal(res.total)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchData(query) }, [query, fetchData])

  const columns: ColumnsType<Article> = [
    {
      title: '标题', dataIndex: 'title', ellipsis: true, width: '30%',
      render: (t, r) => <a onClick={() => setDetail(r)}>{t}</a>,
    },
    { title: '平台', dataIndex: 'platform', width: 80 },
    {
      title: '情感', dataIndex: 'sentiment', width: 80,
      render: (s) => {
        const cfg = SENTIMENT_TAG[s] ?? { color: 'default', label: s }
        return <Tag color={cfg.color}>{cfg.label}</Tag>
      },
    },
    {
      title: '情感分值', dataIndex: 'sentScore', width: 90,
      render: (v: number) => <span style={{ color: v > 0 ? '#52c41a' : v < 0 ? '#ff4d4f' : undefined }}>{v.toFixed(2)}</span>,
    },
    { title: '作者', dataIndex: 'author', width: 100, ellipsis: true },
    {
      title: '发布时间', dataIndex: 'publishedAt', width: 160,
      render: (t) => dayjs(t).format('YYYY-MM-DD HH:mm'),
    },
    {
      title: '操作', width: 80, fixed: 'right',
      render: (_, r) => <a onClick={() => setDetail(r)}>详情</a>,
    },
  ]

  return (
    <div>
      <Title level={4} style={{ marginTop: 0, marginBottom: 16 }}>舆情数据</Title>

      <Card size="small" style={{ marginBottom: 16 }}>
        <Space wrap>
          <Input
            placeholder="搜索关键词"
            prefix={<SearchOutlined />}
            style={{ width: 200 }}
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            onPressEnter={() => setQuery(q => ({ ...q, page: 1, keyword }))}
            allowClear
          />
          <Select
            style={{ width: 140 }}
            options={PLATFORM_OPTIONS}
            defaultValue=""
            onChange={(v) => setQuery(q => ({ ...q, page: 1, platform: v || undefined }))}
          />
          <Select
            style={{ width: 120 }}
            options={[
              { value: '', label: '全部情感' },
              { value: 'positive', label: '正面' },
              { value: 'neutral', label: '中性' },
              { value: 'negative', label: '负面' },
            ]}
            defaultValue=""
            onChange={(v) => setQuery(q => ({ ...q, page: 1, sentiment: v || undefined }))}
          />
          <RangePicker
            onChange={(dates) => setQuery(q => ({
              ...q, page: 1,
              startAt: dates?.[0]?.toISOString(),
              endAt: dates?.[1]?.toISOString(),
            }))}
          />
          <Button icon={<ReloadOutlined />} onClick={() => fetchData(query)}>刷新</Button>
        </Space>
      </Card>

      <Table
        rowKey="id"
        columns={columns}
        dataSource={data}
        loading={loading}
        scroll={{ x: 900 }}
        pagination={{
          current: query.page,
          pageSize: query.pageSize,
          total,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (page, pageSize) => setQuery(q => ({ ...q, page, pageSize })),
        }}
      />

      <Drawer
        open={!!detail}
        onClose={() => setDetail(null)}
        title="舆情详情"
        width={640}
      >
        {detail && (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Descriptions column={2} size="small" bordered>
              <Descriptions.Item label="平台">{detail.platform}</Descriptions.Item>
              <Descriptions.Item label="情感">
                <Tag color={SENTIMENT_TAG[detail.sentiment]?.color}>{SENTIMENT_TAG[detail.sentiment]?.label}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="作者">{detail.author}</Descriptions.Item>
              <Descriptions.Item label="情感分值">{detail.sentScore.toFixed(4)}</Descriptions.Item>
              <Descriptions.Item label="发布时间" span={2}>
                {dayjs(detail.publishedAt).format('YYYY-MM-DD HH:mm:ss')}
              </Descriptions.Item>
              <Descriptions.Item label="原文链接" span={2}>
                <a href={detail.originUrl} target="_blank" rel="noreferrer">{detail.originUrl}</a>
              </Descriptions.Item>
            </Descriptions>
            <Typography.Title level={5} style={{ marginBottom: 8 }}>{detail.title}</Typography.Title>
            <Paragraph style={{ whiteSpace: 'pre-wrap' }}>{detail.content}</Paragraph>
          </Space>
        )}
      </Drawer>
    </div>
  )
}

export default OpinionPage

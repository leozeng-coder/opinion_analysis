import React, { useEffect, useState } from 'react'
import { Row, Col, Card, Statistic, Typography, Spin } from 'antd'
import {
  ArrowDownOutlined,
  FileTextOutlined, FireOutlined, BellOutlined, AlertOutlined,
} from '@ant-design/icons'
import ReactECharts from 'echarts-for-react'
import { articleApi } from '@/api/article'
import { alertApi } from '@/api/alert'
import type { ArticleStats } from '@/types'

const { Title } = Typography

const SENTIMENT_COLOR: Record<string, string> = {
  positive: '#52c41a',
  neutral: '#1677ff',
  negative: '#ff4d4f',
}

const SENTIMENT_LABEL: Record<string, string> = {
  positive: '正面',
  neutral: '中性',
  negative: '负面',
}

const DashboardPage: React.FC = () => {
  const [stats, setStats] = useState<ArticleStats | null>(null)
  const [topicCount, setTopicCount] = useState<number>(0)
  const [todayAlertCount, setTodayAlertCount] = useState<number>(0)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const todayStart = new Date()
    todayStart.setHours(0, 0, 0, 0)

    Promise.all([
      articleApi.stats().catch(() => ({ sentiment: [], platform: [], trend: [], hotTopicCount: 0 })),
      alertApi.listRecords({ pageSize: 1, startAt: todayStart.toISOString() }).catch(() => ({ total: 0, list: [] })),
    ]).then(([s, a]) => {
      setStats(s)
      setTopicCount(s.hotTopicCount ?? 0)
      setTodayAlertCount(a.total)
    }).finally(() => setLoading(false))
  }, [])

  if (loading) return <Spin size="large" style={{ display: 'block', marginTop: 80 }} />

  const sentiment = stats?.sentiment ?? []
  const trend = stats?.trend ?? []
  const platform = stats?.platform ?? []

  const sentPieOption = {
    tooltip: { trigger: 'item' },
    legend: { bottom: 0 },
    series: [{
      type: 'pie',
      radius: ['40%', '70%'],
      data: sentiment.map(s => ({
        value: s.count,
        name: SENTIMENT_LABEL[s.sentiment] ?? s.sentiment,
        itemStyle: { color: SENTIMENT_COLOR[s.sentiment] },
      })),
    }],
  }

  const trendOption = {
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: trend.map(t => t.date) },
    yAxis: { type: 'value' },
    series: [{
      type: 'line', smooth: true,
      data: trend.map(t => t.count),
      lineStyle: { color: '#1677ff' },
      areaStyle: { color: 'rgba(22,119,255,.15)' },
    }],
  }

  const platformOption = {
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    xAxis: { type: 'value' },
    yAxis: { type: 'category', data: platform.map(p => p.platform) },
    series: [{
      type: 'bar',
      data: platform.map(p => p.count),
      itemStyle: { color: '#1677ff' },
    }],
  }

  const totalCount = sentiment.reduce((a, b) => a + b.count, 0)

  return (
    <div>
      <Title level={4} style={{ marginTop: 0, marginBottom: 20 }}>概览仪表盘</Title>

      <Row gutter={[16, 16]}>
        {[
          { title: '舆情总量', value: totalCount, icon: <FileTextOutlined />, color: '#1677ff' },
          { title: '热点话题', value: topicCount, icon: <FireOutlined />, color: '#fa8c16' },
          { title: '今日预警', value: todayAlertCount, icon: <BellOutlined />, color: '#ff4d4f' },
          { title: '负面舆情', value: sentiment.find(s => s.sentiment === 'negative')?.count ?? 0, icon: <AlertOutlined />, color: '#ff7875', suffix: <ArrowDownOutlined /> },
        ].map((item, idx) => (
          <Col xs={24} sm={12} lg={6} key={idx}>
            <Card>
              <Statistic
                title={item.title}
                value={item.value}
                prefix={item.icon}
                valueStyle={{ color: item.color }}
              />
            </Card>
          </Col>
        ))}
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={16}>
          <Card title="舆情趋势" size="small">
            <ReactECharts option={trendOption} style={{ height: 260 }} />
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card title="情感分布" size="small">
            <ReactECharts option={sentPieOption} style={{ height: 260 }} />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24}>
          <Card title="平台分布" size="small">
            <ReactECharts option={platformOption} style={{ height: 200 }} />
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default DashboardPage

import React, { useState, useEffect } from 'react'
import { Row, Col, Card, DatePicker, Typography, Spin } from 'antd'
import ReactECharts from 'echarts-for-react'
import { articleApi } from '@/api/article'
import type { ArticleStats } from '@/types'

const { Title } = Typography
const { RangePicker } = DatePicker

const SENTIMENT_COLOR: Record<string, string> = {
  positive: '#52c41a', neutral: '#1677ff', negative: '#ff4d4f',
}

const StatsPage: React.FC = () => {
  const [stats, setStats] = useState<ArticleStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [dateRange, setDateRange] = useState<[string, string] | undefined>()

  useEffect(() => {
    setLoading(true)
    articleApi.stats(dateRange ? { startAt: dateRange[0], endAt: dateRange[1] } : {})
      .then(setStats)
      .catch(() => {
        setStats({ sentiment: [], platform: [], trend: [] })
      })
      .finally(() => setLoading(false))
  }, [dateRange])

  const sentiment = stats?.sentiment ?? []
  const trend = stats?.trend ?? []
  const platform = stats?.platform ?? []

  const sentBarOption = {
    tooltip: {},
    xAxis: { type: 'category', data: sentiment.map(s => ({ positive: '正面', neutral: '中性', negative: '负面' }[s.sentiment] ?? s.sentiment)) },
    yAxis: { type: 'value' },
    series: [{
      type: 'bar',
      data: sentiment.map(s => ({
        value: s.count,
        itemStyle: { color: SENTIMENT_COLOR[s.sentiment] ?? '#999' },
      })),
    }],
  }

  const trendOption = {
    tooltip: { trigger: 'axis' },
    legend: { data: ['正面', '中性', '负面'] },
    xAxis: { type: 'category', data: trend.map(t => t.date) },
    yAxis: { type: 'value' },
    series: [{
      name: '总量', type: 'line', smooth: true,
      data: trend.map(t => t.count),
      lineStyle: { color: '#1677ff' },
    }],
  }

  const platformPieOption = {
    tooltip: { trigger: 'item' },
    legend: { orient: 'vertical', left: 'left' },
    series: [{
      type: 'pie', radius: '60%',
      data: platform.map(p => ({ name: p.platform, value: p.count })),
    }],
  }

  return (
    <div>
      <Title level={4} style={{ marginTop: 0, marginBottom: 16 }}>统计分析</Title>

      <div style={{ marginBottom: 16 }}>
        <RangePicker
          onChange={(dates) => {
            if (dates?.[0] && dates?.[1]) {
              setDateRange([dates[0].toISOString(), dates[1].toISOString()])
            } else {
              setDateRange(undefined)
            }
          }}
        />
      </div>

      {loading ? <Spin size="large" style={{ display: 'block', marginTop: 80 }} /> : (
        <Row gutter={[16, 16]}>
          <Col xs={24} lg={16}>
            <Card title="舆情量趋势" size="small">
              <ReactECharts option={trendOption} style={{ height: 280 }} />
            </Card>
          </Col>
          <Col xs={24} lg={8}>
            <Card title="情感分布" size="small">
              <ReactECharts option={sentBarOption} style={{ height: 280 }} />
            </Card>
          </Col>
          <Col xs={24} lg={12}>
            <Card title="平台分布" size="small">
              <ReactECharts option={platformPieOption} style={{ height: 280 }} />
            </Card>
          </Col>
        </Row>
      )}
    </div>
  )
}

export default StatsPage

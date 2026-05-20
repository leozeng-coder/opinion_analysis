import React, { useState, useEffect } from 'react'
import { Row, Col, Card, DatePicker, Spin, Empty } from 'antd'
import { BarChartOutlined } from '@ant-design/icons'
import ReactECharts from 'echarts-for-react'
import { articleApi } from '@/api/article'
import { CHART, chartTooltip, chartAxis } from '@/styles/chart'
import PageHeader from '@/components/common/PageHeader'
import page from '@/styles/page.module.css'
import { platformLabel, platformColor } from '@/utils/platform'
import type { ArticleStats } from '@/types'

const { RangePicker } = DatePicker

const SENTIMENT_COLOR: Record<string, string> = {
  positive: CHART.positive,
  neutral: CHART.neutral,
  negative: CHART.negative,
}

const StatsPage: React.FC = () => {
  const [stats, setStats] = useState<ArticleStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [dateRange, setDateRange] = useState<[string, string] | undefined>()

  useEffect(() => {
    setLoading(true)
    articleApi.stats(dateRange ? { startAt: dateRange[0], endAt: dateRange[1] } : {})
      .then(setStats)
      .catch(() => setStats({ sentiment: [], platform: [], trend: [] }))
      .finally(() => setLoading(false))
  }, [dateRange])

  const sentiment = stats?.sentiment ?? []
  const trend = stats?.trend ?? []
  const platform = stats?.platform ?? []

  const sentBarOption = {
    tooltip: chartTooltip,
    grid: { left: 40, right: 16, top: 16, bottom: 32 },
    xAxis: {
      type: 'category',
      data: sentiment.map(s => ({ positive: '正面', neutral: '中性', negative: '负面' }[s.sentiment] ?? s.sentiment)),
      axisLine: chartAxis.line,
      axisTick: { show: false },
      axisLabel: chartAxis.label,
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: chartAxis.splitLine,
      axisLabel: chartAxis.label,
    },
    series: [{
      type: 'bar',
      barWidth: 28,
      data: sentiment.map(s => ({
        value: s.count,
        itemStyle: { color: SENTIMENT_COLOR[s.sentiment] ?? '#999', borderRadius: [6, 6, 0, 0] },
      })),
    }],
  }

  const trendOption = {
    tooltip: { trigger: 'axis', ...chartTooltip },
    grid: { left: 44, right: 16, top: 20, bottom: 32 },
    xAxis: {
      type: 'category',
      data: trend.map(t => t.date),
      axisLine: chartAxis.line,
      axisTick: { show: false },
      axisLabel: chartAxis.label,
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: chartAxis.splitLine,
      axisLabel: chartAxis.label,
    },
    series: [{
      name: '总量', type: 'line', smooth: true,
      data: trend.map(t => t.count),
      lineStyle: { color: CHART.neutral, width: 2 },
      itemStyle: { color: CHART.neutral },
      areaStyle: { color: CHART.neutralArea },
    }],
  }

  const platformPieOption = {
    tooltip: { trigger: 'item', ...chartTooltip },
    legend: { orient: 'vertical', left: 'left', textStyle: { color: 'rgba(15,23,42,0.55)', fontSize: 12 } },
    series: [{
      type: 'pie', radius: ['42%', '68%'],
      itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
      label: { color: 'rgba(15,23,42,0.62)', fontSize: 12 },
      data: platform.map(p => ({
        name: platformLabel(p.platform),
        value: p.count,
        itemStyle: { color: platformColor(p.platform) },
      })),
    }],
  }

  return (
    <div className={page.pageShell}>
      <PageHeader
        title="统计分析"
        subtitle="按时间范围查看舆情量趋势、情感分布与平台占比"
        icon={<BarChartOutlined />}
        extra={
          <RangePicker
            onChange={(dates) => {
              if (dates?.[0] && dates?.[1]) {
                setDateRange([dates[0].toISOString(), dates[1].toISOString()])
              } else {
                setDateRange(undefined)
              }
            }}
          />
        }
      />

      {loading ? (
        <Spin size="large" style={{ display: 'block', margin: '80px auto' }} />
      ) : (
        <Row gutter={[20, 20]} align="stretch">
          <Col xs={24} lg={16} className={page.colStretch}>
            <Card bordered={false} className={page.panelCard} title="舆情量趋势">
              <div className={page.chartBody}>
                {trend.length === 0 ? (
                  <Empty description="暂无数据" className={page.emptyCompact} />
                ) : (
                  <ReactECharts option={trendOption} style={{ height: 280, width: '100%' }} />
                )}
              </div>
            </Card>
          </Col>
          <Col xs={24} lg={8} className={page.colStretch}>
            <Card bordered={false} className={page.panelCard} title="情感分布">
              <div className={page.chartBody}>
                {sentiment.length === 0 ? (
                  <Empty description="暂无数据" className={page.emptyCompact} />
                ) : (
                  <ReactECharts option={sentBarOption} style={{ height: 280, width: '100%' }} />
                )}
              </div>
            </Card>
          </Col>
          <Col xs={24} lg={12} className={page.colStretch}>
            <Card bordered={false} className={page.panelCard} title="平台分布">
              <div className={page.chartBody}>
                {platform.length === 0 ? (
                  <Empty description="暂无数据" className={page.emptyCompact} />
                ) : (
                  <ReactECharts option={platformPieOption} style={{ height: 280, width: '100%' }} />
                )}
              </div>
            </Card>
          </Col>
        </Row>
      )}
    </div>
  )
}

export default StatsPage

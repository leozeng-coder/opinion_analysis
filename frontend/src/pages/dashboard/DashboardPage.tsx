import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Row, Col, Card, Typography, Spin, Tag, Empty, Button, Space, Tooltip,
} from 'antd'
import {
  ArrowDownOutlined, ArrowUpOutlined, MinusOutlined,
  FileTextOutlined, FireOutlined, BellOutlined, AlertOutlined,
  ReloadOutlined, ReadOutlined, CloudSyncOutlined, ClockCircleOutlined,
  TagOutlined,
} from '@ant-design/icons'
import ReactECharts from 'echarts-for-react'
import * as echarts from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { TooltipComponent } from 'echarts/components'
import 'echarts-wordcloud'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/zh-cn'
import { dashboardApi } from '@/api/dashboard'
import { platformLabel, platformColor } from '@/utils/platform'
import { CHART, chartTooltip, chartAxis, wordCloudColor } from '@/styles/chart'
import PageHeader from '@/components/common/PageHeader'
import page from '@/styles/page.module.css'
import type { DashboardOverview } from '@/types'
import styles from './DashboardPage.module.css'

dayjs.extend(relativeTime)
dayjs.locale('zh-cn')

echarts.use([CanvasRenderer, TooltipComponent])

const { Paragraph, Text } = Typography

function ChangeBadge({ value, suffix = '%' }: { value?: number; suffix?: string }) {
  if (value == null) {
    return <span className={styles.kpiChange}>暂无对比数据</span>
  }
  if (value === 0) {
    return (
      <span className={`${styles.kpiChange} ${styles.kpiChangeFlat}`}>
        <MinusOutlined /> 较昨日持平
      </span>
    )
  }
  const up = value > 0
  return (
    <span className={`${styles.kpiChange} ${up ? styles.kpiChangeUp : styles.kpiChangeDown}`}>
      {up ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
      较昨日 {up ? '+' : ''}{value}{suffix}
    </span>
  )
}

interface KPICardProps {
  label: string
  value: number | string
  suffix?: string
  icon: React.ReactNode
  tone: 'blue' | 'amber' | 'rose' | 'sage'
  change?: number
  changeSuffix?: string
  onClick: () => void
}

const KPICard: React.FC<KPICardProps> = ({
  label, value, suffix, icon, tone, change, changeSuffix, onClick,
}) => {
  const toneClass = {
    blue: styles.kpiBlue,
    amber: styles.kpiAmber,
    rose: styles.kpiRose,
    sage: styles.kpiSage,
  }[tone]

  return (
    <Card className={`${page.panelCard} ${styles.kpiCard} ${toneClass}`} onClick={onClick}>
      <div className={styles.kpiInner}>
        <div className={styles.kpiIconWrap}>{icon}</div>
        <div className={styles.kpiBody}>
          <div className={styles.kpiLabel}>{label}</div>
          <div className={styles.kpiValue}>
            {value}
            {suffix && <span className={styles.kpiValueSuffix}>{suffix}</span>}
          </div>
          {change !== undefined ? (
            <ChangeBadge value={change} suffix={changeSuffix} />
          ) : (
            <span className={styles.kpiChange}>&nbsp;</span>
          )}
        </div>
      </div>
    </Card>
  )
}

const DashboardPage: React.FC = () => {
  const navigate = useNavigate()
  const [data, setData] = useState<DashboardOverview | null>(null)
  const [loading, setLoading] = useState(true)
  const [summaryExpanded, setSummaryExpanded] = useState(false)

  const fetchData = useCallback(async () => {
    setLoading(true)
    try {
      const res = await dashboardApi.overview()
      // Ensure arrays are never null
      if (res) {
        res.sentimentTrend = res.sentimentTrend || []
        res.hotTags = res.hotTags || []
        res.recentAlerts = res.recentAlerts || []
        res.recentNegative = res.recentNegative || []
        res.platform = res.platform || []
      }
      setData(res)
    } catch {
      setData(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchData() }, [fetchData])

  if (loading) {
    return (
      <div className={page.pageShell}>
        <Spin size="large" style={{ display: 'block', margin: '80px auto' }} />
      </div>
    )
  }

  if (!data) {
    return (
      <div className={page.pageShell}>
        <Empty description="加载失败" className={page.emptyCompact}>
          <Button type="primary" onClick={fetchData}>重试</Button>
        </Empty>
      </div>
    )
  }

  const { summary, kpi, sentimentTrend, hotTags, recentAlerts, recentNegative, platform, status } = data

  const trendOption = {
    color: [CHART.positive, CHART.neutral, CHART.negative],
    tooltip: {
      trigger: 'axis',
      ...chartTooltip,
      formatter: (params: any[]) => {
        const date = params[0].axisValue
        let html = `${date}<br/>`
        params.forEach(p => {
          html += `<span style="display:inline-block;width:10px;height:10px;border-radius:50%;background:${p.color};margin-right:6px;"></span>`
          html += `${p.seriesName}: <strong>${p.value}</strong> 条<br/>`
        })
        return html
      },
    },
    legend: {
      data: ['正面', '中性', '负面'],
      bottom: 0,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: 'rgba(15,23,42,0.55)', fontSize: 12 },
    },
    grid: { left: 44, right: 20, top: 20, bottom: 44 },
    xAxis: {
      type: 'category',
      data: sentimentTrend.map(t => t.date),
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
    series: [
      {
        name: '正面',
        type: 'line',
        smooth: true,
        data: sentimentTrend.map(t => t.positive),
        lineStyle: { width: 3 },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(16, 185, 129, 0.25)' },
            { offset: 1, color: 'rgba(16, 185, 129, 0.02)' },
          ]),
        },
        symbol: 'circle',
        symbolSize: 6,
        emphasis: {
          focus: 'series',
          symbolSize: 10,
        },
      },
      {
        name: '中性',
        type: 'line',
        smooth: true,
        data: sentimentTrend.map(t => t.neutral),
        lineStyle: { width: 3 },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(59, 130, 246, 0.25)' },
            { offset: 1, color: 'rgba(59, 130, 246, 0.02)' },
          ]),
        },
        symbol: 'circle',
        symbolSize: 6,
        emphasis: {
          focus: 'series',
          symbolSize: 10,
        },
      },
      {
        name: '负面',
        type: 'line',
        smooth: true,
        data: sentimentTrend.map(t => t.negative),
        lineStyle: { width: 3 },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(239, 68, 68, 0.25)' },
            { offset: 1, color: 'rgba(239, 68, 68, 0.02)' },
          ]),
        },
        symbol: 'circle',
        symbolSize: 6,
        emphasis: {
          focus: 'series',
          symbolSize: 10,
        },
      },
    ],
  }

  const platformOption = {
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      ...chartTooltip,
      formatter: (params: { name: string; value: number }[]) => {
        const p = params[0]
        return `${p.name}<br/>${p.value} 条`
      },
    },
    grid: { left: 4, right: 20, top: 8, bottom: 32, containLabel: true },
    xAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: 'rgba(15,23,42,0.05)', type: 'dashed' } },
      axisLabel: { color: 'rgba(15,23,42,0.45)', fontSize: 11, margin: 8 },
    },
    yAxis: {
      type: 'category',
      data: platform.map(p => platformLabel(p.platform)),
      inverse: true,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: 'rgba(15,23,42,0.62)', fontSize: 12 },
    },
    series: [{
      type: 'bar',
      barWidth: 14,
      data: platform.map(p => ({
        value: p.count,
        name: platformLabel(p.platform),
        platform: p.platform,
        itemStyle: {
          color: platformColor(p.platform),
          borderRadius: [0, 6, 6, 0],
        },
      })),
      cursor: 'pointer',
      emphasis: { itemStyle: { opacity: 1 } },
    }],
  }

  const onPlatformClick = (params: {
    componentType?: string
    seriesType?: string
    dataIndex?: number
    data?: { platform?: string } | number
  }) => {
    if (params.componentType !== 'series' || params.seriesType !== 'bar') return
    const fromData = typeof params.data === 'object' && params.data?.platform
    const code = fromData || platform[params.dataIndex ?? -1]?.platform
    if (code) navigate(`/opinion?platform=${encodeURIComponent(code)}`)
  }

  const summaryPreview = summary?.text
    ? (summary.text.length > 120 && !summaryExpanded ? `${summary.text.slice(0, 120)}…` : summary.text)
    : ''

  const lastRun = status.lastCrawlerRun
  const crawlerOk = lastRun?.status === 'success'
  const crawlerRunning = lastRun?.status === 'running'

  const rankClass = (idx: number) => {
    if (idx < 3) return styles.rankTop
    if (idx < 5) return styles.rankMid
    return styles.rankRest
  }

  return (
    <div className={page.pageShell}>
      <PageHeader
        title="概览仪表盘"
        subtitle="今日舆情全景 · 近 7 天趋势"
        extra={
          <Button icon={<ReloadOutlined />} onClick={fetchData} className={page.ghostBtn}>
            刷新
          </Button>
        }
      />

      <Card bordered={false} className={`${page.panelCard} ${styles.summaryCard}`}>
        <div className={styles.summaryHeader}>
          <div className={styles.summaryIcon}><ReadOutlined /></div>
          <span className={styles.summaryTitle}>今日 AI 摘要</span>
          {summary?.date && <span className={styles.summaryDate}>{summary.date}</span>}
        </div>
        {summary?.text ? (
          <>
            <Paragraph className={styles.summaryText}>{summaryPreview}</Paragraph>
            {summary.text.length > 120 && (
              <Button type="link" className={styles.summaryExpand}
                onClick={() => setSummaryExpanded(v => !v)}>
                {summaryExpanded ? '收起' : '展开全文'}
              </Button>
            )}
            {summary.keywords?.length > 0 && (
              <div className={styles.summaryKeywords}>
                <Space size={[6, 6]} wrap>
                  {summary.keywords.slice(0, 8).map(kw => (
                    <Tag key={kw} className={styles.keywordTag}
                      onClick={() => navigate(`/opinion?tags=${encodeURIComponent(kw)}`)}>
                      {kw}
                    </Tag>
                  ))}
                </Space>
              </div>
            )}
          </>
        ) : (
          <Text type="secondary">暂无 AI 摘要，请运行「新闻收集 + AI 关键词提取」</Text>
        )}
      </Card>

      {/* KPI — equal height row */}
      <Row gutter={[20, 20]} align="stretch">
        <Col xs={24} sm={12} lg={6} className={page.colStretch}>
          <KPICard label="今日新增" value={kpi.todayNew.count}
            icon={<FileTextOutlined />} tone="blue"
            change={kpi.todayNew.changePercent}
            onClick={() => navigate('/opinion')} />
        </Col>
        <Col xs={24} sm={12} lg={6} className={page.colStretch}>
          <KPICard label="热点话题" value={kpi.hotTopics.count}
            icon={<FireOutlined />} tone="amber"
            change={kpi.hotTopics.changePercent}
            onClick={() => navigate('/topics')} />
        </Col>
        <Col xs={24} sm={12} lg={6} className={page.colStretch}>
          <KPICard label="今日预警" value={kpi.todayAlerts.count}
            icon={<BellOutlined />} tone="rose"
            change={kpi.todayAlerts.changePercent}
            onClick={() => navigate('/alerts')} />
        </Col>
        <Col xs={24} sm={12} lg={6} className={page.colStretch}>
          <KPICard label="负面占比" value={kpi.negativeRatio.percent} suffix="%"
            icon={<AlertOutlined />} tone="sage"
            change={kpi.negativeRatio.changePoints} changeSuffix=" pp"
            onClick={() => navigate('/opinion?sentiment=negative')} />
        </Col>
      </Row>

      {/* 趋势 + 热点 — matched height */}
      <Row gutter={[20, 20]} align="stretch">
        <Col xs={24} lg={16} className={page.colStretch}>
          <Card bordered={false} className={page.panelCard}
            title="舆情趋势" extra={<Text type="secondary" style={{ fontSize: 12 }}>近 7 天 · 正/中/负</Text>}>
            <div className={page.chartBody}>
              {sentimentTrend.length === 0 ? (
                <Empty description="暂无趋势数据" className={page.emptyCompact} />
              ) : (
                <ReactECharts option={trendOption} style={{ height: 280, width: '100%' }} />
              )}
            </div>
          </Card>
        </Col>
        <Col xs={24} lg={8} className={page.colStretch}>
          <Card bordered={false} className={page.panelCard}
            title="热点 Top 5"
            extra={hotTags.length > 5 && (
              <Button type="link" size="small" onClick={() => navigate('/topics')}>全部</Button>
            )}>
            <div className={styles.hotTagsBody}>
              {hotTags.length === 0 ? (
                <Empty description="暂无热点标签" className={page.emptyCompact} />
              ) : (
                <>
                  <div className={styles.hotTagList}>
                    {hotTags.slice(0, 5).map((item, idx) => (
                      <div key={item.tag} className={styles.hotTagRow}
                        onClick={() => navigate(`/opinion?tags=${encodeURIComponent(item.tag)}`)}>
                        <span className={`${styles.hotTagRank} ${rankClass(idx)}`}>{idx + 1}</span>
                        <span className={styles.hotTagName}>{item.tag}</span>
                        <span className={styles.hotTagCount}>{item.count} 篇</span>
                      </div>
                    ))}
                  </div>
                  <div className={styles.wordCloudSection}>
                    <span className={styles.wordCloudLabel}>词云预览</span>
                    <WordCloudMini tags={hotTags.slice(0, 20)}
                      onClick={(tag) => navigate(`/opinion?tags=${encodeURIComponent(tag)}`)} />
                  </div>
                </>
              )}
            </div>
          </Card>
        </Col>
      </Row>

      {/* 预警 + 负面 — equal height */}
      <Row gutter={[20, 20]} align="stretch">
        <Col xs={24} lg={12} className={page.colStretch}>
          <Card bordered={false} className={page.panelCard}
            title="最新预警"
            extra={<Button type="link" size="small" onClick={() => navigate('/alerts')}>预警中心</Button>}>
            <div className={styles.listPanelBody}>
              {recentAlerts.length === 0 ? (
                <Empty description="暂无预警记录" className={page.emptyCompact} />
              ) : (
                recentAlerts.map(alert => (
                  <div key={alert.id} className={styles.listItem} onClick={() => navigate('/alerts')}>
                    <span className={`${styles.listItemDot} ${styles.dotAlert}`} />
                    <div className={styles.listItemMain}>
                      <div className={styles.listItemTitle}>{alert.title || alert.content}</div>
                    </div>
                    <div className={styles.listItemMeta}>
                      {alert.rule?.name && (
                        <Tag className={page.softTagRose}>{alert.rule.name}</Tag>
                      )}
                      <span className={styles.listItemTime}>
                        {dayjs(alert.createdAt).format('MM-DD HH:mm')}
                      </span>
                    </div>
                  </div>
                ))
              )}
            </div>
          </Card>
        </Col>
        <Col xs={24} lg={12} className={page.colStretch}>
          <Card bordered={false} className={page.panelCard}
            title="最新负面舆情"
            extra={
              <Button type="link" size="small"
                onClick={() => navigate('/opinion?sentiment=negative')}>全部</Button>
            }>
            <div className={styles.listPanelBody}>
              {recentNegative.length === 0 ? (
                <Empty description="暂无负面舆情" className={page.emptyCompact} />
              ) : (
                recentNegative.map(article => (
                  <div key={article.id} className={styles.listItem}
                    onClick={() => navigate(`/opinion?id=${article.id}&sentiment=negative`)}>
                    <span className={`${styles.listItemDot} ${styles.dotNegative}`} />
                    <div className={styles.listItemMain}>
                      <div className={styles.listItemTitle}>{article.title}</div>
                    </div>
                    <div className={styles.listItemMeta}>
                      <Tag className={page.softTagBlue}>{platformLabel(article.platform)}</Tag>
                      <span className={styles.listItemTime}>
                        {dayjs(article.publishedAt).format('MM-DD HH:mm')}
                      </span>
                    </div>
                  </div>
                ))
              )}
            </div>
          </Card>
        </Col>
      </Row>

      {/* 平台 + 状态 — aligned bottom row */}
      <Row gutter={[20, 20]} align="stretch">
        <Col xs={24} lg={17} className={page.colStretch}>
          <Card bordered={false} className={`${page.panelCard} ${styles.platformChartCard}`}
            title="平台分布"
            extra={<Text type="secondary" style={{ fontSize: 12 }}>近 7 天 · 点击跳转</Text>}>
            <div className={styles.bottomPlatformBody}>
              {platform.length === 0 ? (
                <Empty description="暂无平台数据" className={page.emptyCompact} />
              ) : (
                <ReactECharts
                  option={platformOption}
                  style={{ height: Math.max(220, platform.length * 36 + 36), width: '100%' }}
                  onEvents={{ click: onPlatformClick }}
                />
              )}
            </div>
          </Card>
        </Col>
        <Col xs={24} lg={7} className={page.colStretch}>
          <Card bordered={false} className={page.panelCard} title="数据新鲜度">
            <div className={styles.statusCardBody}>
              <div className={styles.statusItem}>
                <div className={`${styles.statusIconWrap} ${
                  crawlerRunning ? styles.statusIconWarn
                    : crawlerOk ? styles.statusIconOk : styles.statusIconErr
                }`}>
                  <CloudSyncOutlined />
                </div>
                <div className={styles.statusContent}>
                  <div className={styles.statusLabel}>爬虫状态</div>
                  <div className={styles.statusValue}>
                    {lastRun ? (
                      <Tooltip title={`任务: ${lastRun.spiders} · ${lastRun.status}`}>
                        <span>
                          {crawlerRunning ? '运行中' : crawlerOk ? '正常' : '异常'}
                          {' · '}{dayjs(lastRun.startedAt).fromNow()}
                        </span>
                      </Tooltip>
                    ) : '暂无爬虫记录'}
                  </div>
                </div>
              </div>

              {status.latestArticleAt && (
                <div className={styles.statusItem}>
                  <div className={styles.statusIconWrap}>
                    <ClockCircleOutlined />
                  </div>
                  <div className={styles.statusContent}>
                    <div className={styles.statusLabel}>最新数据</div>
                    <div className={styles.statusValue}>
                      {dayjs(status.latestArticleAt).fromNow()}更新
                    </div>
                  </div>
                </div>
              )}

              <div className={styles.statusItem}>
                <div className={`${styles.statusIconWrap} ${
                  status.pendingTagging > 0 ? styles.statusIconWarn : styles.statusIconOk
                }`}>
                  <TagOutlined />
                </div>
                <div className={styles.statusContent}>
                  <div className={styles.statusLabel}>AI 打标</div>
                  <div className={styles.statusValue}>
                    {status.pendingTagging > 0
                      ? `${status.pendingTagging} 篇待处理`
                      : '已全部完成'}
                  </div>
                </div>
              </div>

              <div className={styles.statusLink}>
                <Button type="link" className={styles.statusLinkBtn}
                  onClick={() => navigate('/crawler')}>
                  前往爬虫调度 →
                </Button>
              </div>
            </div>
          </Card>
        </Col>
      </Row>
    </div>
  )
}

interface WordCloudMiniProps {
  tags: { tag: string; count: number }[]
  onClick: (tag: string) => void
}

const WordCloudMini: React.FC<WordCloudMiniProps> = ({ tags, onClick }) => {
  const chartRef = useRef<HTMLDivElement | null>(null)
  const instRef = useRef<echarts.ECharts | null>(null)

  useEffect(() => {
    if (!chartRef.current) return
    if (!instRef.current) instRef.current = echarts.init(chartRef.current)
    const inst = instRef.current

    inst.setOption({
      tooltip: {
        show: true,
        backgroundColor: 'rgba(255,255,255,0.96)',
        borderColor: 'rgba(15,23,42,0.08)',
        formatter: (p: { name: string; value: number }) => `${p.name}：${p.value} 条`,
      },
      series: [{
        type: 'wordCloud',
        shape: 'circle',
        gridSize: 12,
        sizeRange: [11, 26],
        rotationRange: [0, 0],
        drawOutOfBound: false,
        textStyle: {
          fontFamily: 'inherit',
        },
        emphasis: {
          textStyle: { fontWeight: 600, textShadowBlur: 4, textShadowColor: 'rgba(0,0,0,0.08)' },
        },
        data: tags.map(d => ({
          name: d.tag,
          value: d.count,
          textStyle: { color: wordCloudColor(d.tag) },
        })),
      }],
    } as echarts.EChartsCoreOption)

    const handler = (params: { name: string }) => onClick(params.name)
    inst.off('click')
    inst.on('click', handler)

    const onResize = () => inst.resize()
    window.addEventListener('resize', onResize)
    return () => { window.removeEventListener('resize', onResize) }
  }, [tags, onClick])

  useEffect(() => () => { instRef.current?.dispose(); instRef.current = null }, [])

  return <div ref={chartRef} className={styles.wordCloudWrap} />
}

export default DashboardPage

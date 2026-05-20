import React, { useEffect, useMemo, useRef, useState, useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  Table, Input, Select, DatePicker, Space, Tag, Button,
  Typography, Drawer, Descriptions, Card, Empty, Tooltip,
} from 'antd'
import { ReloadOutlined, CloseOutlined, FileTextOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as echarts from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { TooltipComponent } from 'echarts/components'
import 'echarts-wordcloud'
import dayjs from 'dayjs'
import { articleApi, type ArticleQuery } from '@/api/article'
import type { Article, TagCount } from '@/types'
import { platformLabel } from '@/utils/platform'
import { wordCloudColor } from '@/styles/chart'
import PageHeader from '@/components/common/PageHeader'
import page from '@/styles/page.module.css'

echarts.use([CanvasRenderer, TooltipComponent])

const { Paragraph } = Typography
const { RangePicker } = DatePicker

const SENTIMENT_TAG: Record<string, { className: string; label: string }> = {
  positive: { className: page.softTagSage, label: '正面' },
  neutral: { className: page.softTagBlue, label: '中性' },
  negative: { className: page.softTagRose, label: '负面' },
}

// 解析 aiTags（后端返回 JSON 字符串）→ string[]
const parseTags = (raw?: string | null): string[] => {
  if (!raw) return []
  try {
    const v = JSON.parse(raw)
    return Array.isArray(v) ? v.filter((x) => typeof x === 'string') : []
  } catch {
    return []
  }
}

const OpinionPage: React.FC = () => {
  const [searchParams] = useSearchParams()
  const [data, setData] = useState<Article[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [query, setQuery] = useState<ArticleQuery>(() => {
    const init: ArticleQuery = { page: 1, pageSize: 20 }
    const plat = searchParams.get('platform')
    if (plat) init.platform = plat
    return init
  })
  const [keyword, setKeyword] = useState('')
  const [detail, setDetail] = useState<Article | null>(null)
  const [platformOptions, setPlatformOptions] = useState<{ value: string; label: string }[]>([
    { value: '', label: '全部平台' },
  ])
  const [tagCounts, setTagCounts] = useState<TagCount[]>([])
  const [selectedTags, setSelectedTags] = useState<string[]>(() => {
    const t = searchParams.get('tags')
    return t ? t.split(',').filter(Boolean) : []
  })

  // 平台列表
  useEffect(() => {
    articleApi.platforms().then((list) => {
      setPlatformOptions([
        { value: '', label: '全部平台' },
        ...list.map((p) => ({ value: p, label: platformLabel(p) })),
      ])
    })
  }, [])

  // 词云数据 —— 跟随平台/时间范围筛选，但不跟随 tags 自身（避免词云被自己筛空）
  const refreshTagCounts = useCallback(() => {
    articleApi.tags({
      platform: query.platform,
      startAt: query.startAt,
      endAt: query.endAt,
      limit: 80,
    }).then(setTagCounts).catch(() => setTagCounts([]))
  }, [query.platform, query.startAt, query.endAt])

  useEffect(() => { refreshTagCounts() }, [refreshTagCounts])

  // 文章列表
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

  // URL ?platform= 跳转（如仪表盘柱状图点击）
  useEffect(() => {
    const plat = searchParams.get('platform')
    setQuery((q) => ({ ...q, page: 1, platform: plat || undefined }))
  }, [searchParams])

  // 选中标签 → 同步到 query
  useEffect(() => {
    setQuery((q) => ({
      ...q,
      page: 1,
      tags: selectedTags.length ? selectedTags.join(',') : undefined,
    }))
  }, [selectedTags])

  const toggleTag = (tag: string) => {
    setSelectedTags((prev) =>
      prev.includes(tag) ? prev.filter((t) => t !== tag) : [...prev, tag]
    )
  }

  const columns: ColumnsType<Article> = [
    {
      title: '标题', dataIndex: 'title', ellipsis: true, width: '28%',
      render: (t, r) => <a onClick={() => setDetail(r)}>{t}</a>,
    },
    {
      title: 'AI 标签', dataIndex: 'aiTags', width: 220,
      render: (raw: string | null | undefined) => {
        const tags = parseTags(raw)
        if (!tags.length) return <span style={{ color: '#bfbfbf' }}>—</span>
        return (
          <Space size={[4, 4]} wrap>
            {tags.map((t) => (
              <Tag
                key={t}
                color={selectedTags.includes(t) ? 'blue' : undefined}
                style={{ cursor: 'pointer', marginInlineEnd: 0 }}
                onClick={(e) => { e.stopPropagation(); toggleTag(t) }}
              >
                {t}
              </Tag>
            ))}
          </Space>
        )
      },
    },
    { title: '平台', dataIndex: 'platform', width: 80, render: (p: string) => platformLabel(p) },
    {
      title: '情感', dataIndex: 'sentiment', width: 80,
      render: (s) => {
        const cfg = SENTIMENT_TAG[s] ?? { className: page.softTagNeutral, label: s }
        return <Tag className={cfg.className}>{cfg.label}</Tag>
      },
    },
    {
      title: '情感分值', dataIndex: 'sentScore', width: 90,
      render: (v: number) => <span style={{ color: v > 0 ? '#42C48C' : v < 0 ? '#EC6B6B' : undefined }}>{v.toFixed(2)}</span>,
    },
    {
      title: '发布时间', dataIndex: 'publishedAt', width: 160,
      render: (t) => dayjs(t).format('YYYY-MM-DD HH:mm'),
    },
    {
      title: '操作', width: 70, fixed: 'right',
      render: (_, r) => <a onClick={() => setDetail(r)}>详情</a>,
    },
  ]

  // 标签下拉选项（按词频排序）
  const tagSelectOptions = useMemo(
    () => tagCounts.map((t) => ({ value: t.tag, label: `${t.tag} (${t.count})` })),
    [tagCounts]
  )

  return (
    <div className={page.pageShell}>
      <PageHeader
        title="舆情数据"
        subtitle="浏览、筛选各平台舆情文章，支持 AI 标签词云与多维检索"
        icon={<FileTextOutlined />}
        extra={
          <Button icon={<ReloadOutlined />} className={page.ghostBtn}
            onClick={() => { fetchData(query); refreshTagCounts() }}>
            刷新
          </Button>
        }
      />

      <TagCloudCard
        data={tagCounts}
        selected={selectedTags}
        onToggle={toggleTag}
        onClear={() => setSelectedTags([])}
      />

      <Card bordered={false} className={`${page.panelCard} ${page.toolbar}`}>
        <Space wrap>
          <Input.Search
            placeholder="搜索关键词"
            style={{ width: 200 }}
            value={keyword}
            onChange={(e) => {
              setKeyword(e.target.value)
              if (!e.target.value) {
                setQuery(q => ({ ...q, page: 1, keyword: undefined }))
              }
            }}
            onSearch={(v) => setQuery(q => ({ ...q, page: 1, keyword: v || undefined }))}
            allowClear
          />
          <Select
            style={{ width: 140 }}
            options={platformOptions}
            value={query.platform ?? ''}
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
          <Select
            mode="multiple"
            allowClear
            placeholder="按 AI 标签筛选"
            style={{ minWidth: 240 }}
            value={selectedTags}
            options={tagSelectOptions}
            onChange={setSelectedTags}
            maxTagCount="responsive"
            optionFilterProp="label"
          />
          <RangePicker
            onChange={(dates) => setQuery(q => ({
              ...q, page: 1,
              startAt: dates?.[0]?.toISOString(),
              endAt: dates?.[1]?.toISOString(),
            }))}
          />
        </Space>
      </Card>

      <Card bordered={false} className={`${page.panelCard} ${page.tableWrap}`}>
      <Table
        rowKey="id"
        columns={columns}
        dataSource={data}
        loading={loading}
        scroll={{ x: 1100 }}
        pagination={{
          current: query.page,
          pageSize: query.pageSize,
          total,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (page, pageSize) => setQuery(q => ({ ...q, page, pageSize })),
        }}
      />
      </Card>

      <Drawer
        open={!!detail}
        onClose={() => setDetail(null)}
        title="舆情详情"
        width={640}
      >
        {detail && (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Descriptions column={2} size="small" bordered>
              <Descriptions.Item label="平台">{platformLabel(detail.platform)}</Descriptions.Item>
              <Descriptions.Item label="情感">
                <Tag className={SENTIMENT_TAG[detail.sentiment]?.className ?? page.softTagNeutral}>
                  {SENTIMENT_TAG[detail.sentiment]?.label ?? detail.sentiment}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="作者">{detail.author}</Descriptions.Item>
              <Descriptions.Item label="情感分值">{detail.sentScore.toFixed(4)}</Descriptions.Item>
              <Descriptions.Item label="AI 标签" span={2}>
                {parseTags(detail.aiTags).length === 0 ? (
                  <span style={{ color: '#bfbfbf' }}>未打标</span>
                ) : (
                  <Space size={[4, 4]} wrap>
                    {parseTags(detail.aiTags).map((t) => (
                      <Tag key={t} className={page.softTagBlue}>{t}</Tag>
                    ))}
                  </Space>
                )}
              </Descriptions.Item>
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

// ────────────────────────────────────────────────────────────────────────────
// 词云卡片

interface TagCloudCardProps {
  data: TagCount[]
  selected: string[]
  onToggle: (tag: string) => void
  onClear: () => void
}

const TagCloudCard: React.FC<TagCloudCardProps> = ({ data, selected, onToggle, onClear }) => {
  const chartRef = useRef<HTMLDivElement | null>(null)
  const instRef = useRef<echarts.ECharts | null>(null)

  useEffect(() => {
    if (!chartRef.current) return
    if (!instRef.current) instRef.current = echarts.init(chartRef.current)
    const inst = instRef.current!

    if (!data.length) {
      inst.clear()
      return
    }

    inst.setOption({
      tooltip: { show: true, formatter: (p: { name: string; value: number }) => `${p.name}：${p.value} 条` },
      series: [{
        type: 'wordCloud',
        shape: 'circle',
        gridSize: 8,
        sizeRange: [12, 40],
        rotationRange: [0, 0],
        drawOutOfBound: false,
        textStyle: {},
        emphasis: { textStyle: { fontWeight: 'bold', textShadowBlur: 6, textShadowColor: 'rgba(0,0,0,0.2)' } },
        data: data.map((d) => ({
          name: d.tag,
          value: d.count,
          textStyle: { color: wordCloudColor(d.tag) },
        })),
      }],
    } as echarts.EChartsCoreOption)

    const handler = (params: { name: string }) => onToggle(params.name)
    inst.off('click')
    inst.on('click', handler)

    const onResize = () => inst.resize()
    window.addEventListener('resize', onResize)
    return () => { window.removeEventListener('resize', onResize) }
  }, [data, onToggle])

  useEffect(() => () => { instRef.current?.dispose(); instRef.current = null }, [])

  return (
    <Card
      bordered={false}
      className={page.panelCard}
      title="标签词云"
      extra={
        selected.length > 0 ? (
          <Space>
            <span style={{ color: '#666' }}>已选 {selected.length} 个</span>
            <Button size="small" icon={<CloseOutlined />} onClick={onClear}>清空</Button>
          </Space>
        ) : (
          <span style={{ color: 'var(--app-text-muted)', fontSize: 13 }}>点击标签即可筛选；可与下方筛选叠加</span>
        )
      }
    >
      {data.length === 0 ? (
        <Empty description="暂无标签数据（等待 AI 打标完成）" className={page.emptyCompact} style={{ padding: '24px 0' }} />
      ) : (
        <>
          <div ref={chartRef} style={{ width: '100%', height: 260 }} />
          {selected.length > 0 && (
            <div style={{ padding: '8px 8px 0', borderTop: '1px dashed #f0f0f0' }}>
              <span style={{ color: '#666', marginRight: 8 }}>当前筛选：</span>
              <Space size={[4, 4]} wrap>
                {selected.map((t) => (
                  <Tooltip key={t} title="点击移除">
                    <Tag
                      color="blue"
                      closable
                      className={page.softTagBlue}
                      onClose={(e) => { e.preventDefault(); onToggle(t) }}
                      style={{ cursor: 'pointer' }}
                    >
                      {t}
                    </Tag>
                  </Tooltip>
                ))}
              </Space>
            </div>
          )}
        </>
      )}
    </Card>
  )
}

export default OpinionPage

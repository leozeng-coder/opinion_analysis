import React, { useEffect, useState } from 'react'
import {
  Table, Button, Modal, Form, Input, Select, InputNumber,
  Space, Tag, Popconfirm, Tabs, message, Card, Switch, Drawer, Typography,
} from 'antd'
import { PlusOutlined, DeleteOutlined, EditOutlined, BellOutlined, EyeOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import ReactMarkdown from 'react-markdown'
import { alertApi } from '@/api/alert'
import PageHeader from '@/components/common/PageHeader'
import page from '@/styles/page.module.css'
import type { AlertRule, AlertRecord, AlertRulePayload } from '@/types'

function formatRuleKeywords(andKw?: string, orKw?: string): string {
  const parseList = (raw?: string): string[] => {
    if (!raw || raw === '[]') return []
    const trimmed = raw.trim()
    if (trimmed.startsWith('[')) {
      try {
        const parsed = JSON.parse(trimmed) as unknown
        if (Array.isArray(parsed)) {
          return parsed.map((k) => String(k).trim()).filter(Boolean)
        }
      } catch { /* fall through */ }
    }
    return []
  }

  const andList = parseList(andKw)
  const orList = parseList(orKw)

  const parts: string[] = []
  if (andList.length > 0) {
    parts.push(`必含: ${andList.join('、')}`)
  }
  if (orList.length > 0) {
    parts.push(`任一: ${orList.join('、')}`)
  }

  return parts.length > 0 ? parts.join(' | ') : '全部'
}

interface ParsedAlertContent {
  rule?: string
  timeWindow?: string
  keywords?: string
  sentiment?: string
  matchCount?: string
  aiAnalysis?: string
  articles?: string[]
}

function parseAlertContent(content: string): ParsedAlertContent {
  const result: ParsedAlertContent = {}
  const lines = content.split('\n')

  let currentSection = ''
  const articleLines: string[] = []
  const aiLines: string[] = []

  for (const line of lines) {
    const trimmed = line.trim()

    if (trimmed.startsWith('规则：')) {
      result.rule = trimmed.substring(3)
    } else if (trimmed.startsWith('时间窗口：')) {
      result.timeWindow = trimmed.substring(5)
    } else if (trimmed.startsWith('关键词：')) {
      result.keywords = trimmed.substring(4)
    } else if (trimmed.startsWith('情感：')) {
      result.sentiment = trimmed.substring(3)
    } else if (trimmed.startsWith('匹配条数：')) {
      result.matchCount = trimmed.substring(5)
    } else if (trimmed === '【AI 分析】') {
      currentSection = 'ai'
    } else if (trimmed === '【匹配文章】') {
      currentSection = 'articles'
    } else if (trimmed && currentSection === 'ai') {
      aiLines.push(trimmed)
    } else if (trimmed && currentSection === 'articles') {
      articleLines.push(trimmed)
    }
  }

  if (aiLines.length > 0) {
    result.aiAnalysis = aiLines.join('\n')
  }
  if (articleLines.length > 0) {
    result.articles = articleLines
  }

  return result
}

const NOTIFY_TYPE_LABEL: Record<string, string> = {
  email: '邮件',
}

const SENTIMENT_LABEL: Record<string, string> = {
  negative: '负面',
  neutral: '中性',
  positive: '正面',
}

const DEFAULT_FORM = {
  threshold: 10,
  interval: 60,
  status: true,
  notifyType: 'email',
  sentiment: '',
  keywordListAnd: [] as string[],
  keywordListOr: [] as string[],
  timeRangeDays: 3,
  remark: '',
}

function buildPayload(values: Record<string, unknown>): AlertRulePayload {
  return {
    name: values.name as string,
    remark: (values.remark as string | undefined) ?? '',
    keywordListAnd: (values.keywordListAnd as string[] | undefined) ?? [],
    keywordListOr: (values.keywordListOr as string[] | undefined) ?? [],
    sentiment: (values.sentiment as string | undefined) ?? '',
    timeRangeDays: (values.timeRangeDays as number | undefined) ?? 3,
    threshold: values.threshold as number,
    interval: values.interval as number,
    notifyType: 'email',
    notifyEmail: values.notifyEmail as string | undefined,
    notifyWebhook: undefined,
    notifyPhone: undefined,
    status: values.status ? 1 : 0,
  }
}

function ruleToFormValues(rule: AlertRule): Record<string, unknown> {
  const parseKeywordList = (raw?: string): string[] => {
    if (!raw) return []
    const trimmed = raw.trim()
    if (trimmed.startsWith('[')) {
      try {
        const parsed = JSON.parse(trimmed) as unknown
        if (Array.isArray(parsed)) {
          return parsed.map((k) => String(k).trim()).filter(Boolean)
        }
      } catch {
        return trimmed.split(',').map((k) => k.trim()).filter(Boolean)
      }
    }
    return trimmed.split(',').map((k) => k.trim()).filter(Boolean)
  }

  const values: Record<string, unknown> = {
    name: rule.name,
    remark: rule.remark || '',
    keywordListAnd: parseKeywordList(rule.keywordsAnd),
    keywordListOr: parseKeywordList(rule.keywordsOr),
    sentiment: rule.sentiment || '',
    timeRangeDays: rule.timeRangeDays ?? 3,
    threshold: rule.threshold,
    interval: rule.interval,
    notifyType: 'email',
    status: rule.status === 1,
  }
  values.notifyEmail = rule.notifyConf !== '-' ? rule.notifyConf : ''
  return values
}

const AlertsPage: React.FC = () => {
  const [rules, setRules] = useState<AlertRule[]>([])
  const [records, setRecords] = useState<AlertRecord[]>([])
  const [recordTotal, setRecordTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [evaluating, setEvaluating] = useState(false)
  const [detailDrawerOpen, setDetailDrawerOpen] = useState(false)
  const [currentRecord, setCurrentRecord] = useState<AlertRecord | null>(null)
  const [form] = Form.useForm()

  const fetchRules = async () => {
    try {
      const data = await alertApi.listRules()
      setRules(data)
    } catch {
      /* error toast from interceptor */
    }
  }

  const fetchRecords = async (pageNum = 1) => {
    setLoading(true)
    try {
      const res = await alertApi.listRecords({ page: pageNum, pageSize: 20 })
      setRecords(res.list)
      setRecordTotal(res.total)
    } catch {
      /* error toast from interceptor */
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchRules(); fetchRecords() }, [])

  const openCreate = () => {
    setEditingId(null)
    form.resetFields()
    form.setFieldsValue(DEFAULT_FORM)
    setModalOpen(true)
  }

  const openEdit = (rule: AlertRule) => {
    setEditingId(rule.id)
    form.resetFields()
    form.setFieldsValue(ruleToFormValues(rule))
    setModalOpen(true)
  }

  const closeModal = () => {
    setModalOpen(false)
    setEditingId(null)
    form.resetFields()
  }

  const handleSubmit = async () => {
    const values = await form.validateFields()
    const payload = buildPayload(values)
    setSubmitting(true)
    try {
      if (editingId) {
        await alertApi.updateRule(editingId, payload)
        message.success('规则已更新')
      } else {
        await alertApi.createRule(payload)
        message.success('预警规则创建成功')
      }
      closeModal()
      fetchRules()
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: number) => {
    await alertApi.deleteRule(id)
    message.success('删除成功')
    fetchRules()
  }

  const handleEvaluate = async () => {
    setEvaluating(true)
    try {
      const res = await alertApi.evaluate(true)
      if ('triggered' in res) {
        message.success(`评估完成：触发 ${res.triggered} 条，跳过 ${res.skipped} 条`)
        if (res.details?.length) {
          const skipped = res.details.filter((d) => !d.triggered && d.skipReason)
          if (skipped.length > 0) {
            Modal.info({
              title: '未触发原因',
              width: 560,
              content: (
                <ul style={{ margin: 0, paddingLeft: 20 }}>
                  {skipped.map((d) => (
                    <li key={d.ruleId} style={{ marginBottom: 8 }}>
                      <strong>{d.ruleName}</strong>：{d.skipReason}
                      {d.matchCount != null && d.threshold != null && (
                        <span style={{ color: '#888' }}>
                          {' '}
                          （匹配 {d.matchCount}/{d.threshold}，窗口自 {d.windowStart ?? '今日 0 点'}）
                        </span>
                      )}
                    </li>
                  ))}
                </ul>
              ),
            })
          }
        }
        fetchRecords()
      } else {
        message.success(res.message)
      }
    } finally {
      setEvaluating(false)
    }
  }

  const handleViewDetail = async (record: AlertRecord) => {
    try {
      const detail = await alertApi.getRecordDetail(record.id)
      setCurrentRecord(detail)
      setDetailDrawerOpen(true)
      if (detail.status === 'pending') {
        await alertApi.markAsRead(record.id)
        fetchRecords()
      }
    } catch {
      /* error toast from interceptor */
    }
  }

  const closeDetailDrawer = () => {
    setDetailDrawerOpen(false)
    setCurrentRecord(null)
  }

  const ruleColumns: ColumnsType<AlertRule> = [
    { title: '规则名称', dataIndex: 'name' },
    {
      title: '关键词',
      dataIndex: 'keywords',
      render: (_, r) => formatRuleKeywords(r.keywordsAnd, r.keywordsOr)
    },
    {
      title: '触发情感', dataIndex: 'sentiment', width: 90,
      render: (s) => s
        ? <Tag className={page.softTagNeutral}>{SENTIMENT_LABEL[s] || s}</Tag>
        : '全部',
    },
    { title: '阈值', dataIndex: 'threshold', width: 80 },
    { title: '间隔(分)', dataIndex: 'interval', width: 90 },
    {
      title: '通知方式', dataIndex: 'notifyType', width: 100,
      render: (t) => NOTIFY_TYPE_LABEL[t] || t,
    },
    { title: '通知目标', dataIndex: 'notifyConf', ellipsis: true },
    {
      title: '状态', dataIndex: 'status', width: 80,
      render: (s) => (
        <Tag className={s ? page.softTagSage : page.softTagNeutral}>
          {s ? '启用' : '停用'}
        </Tag>
      ),
    },
    {
      title: '操作', width: 120, fixed: 'right',
      render: (_, r) => (
        <Space size={0}>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(r)}>
            编辑
          </Button>
          <Popconfirm title="确认删除?" onConfirm={() => handleDelete(r.id)}>
            <Button type="link" danger size="small" icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const recordColumns: ColumnsType<AlertRecord> = [
    { title: '标题', dataIndex: 'title', ellipsis: true },
    { title: '规则', dataIndex: ['rule', 'name'], width: 140 },
    {
      title: '状态', dataIndex: 'status', width: 90,
      render: (s) => (
        <Tag className={s === 'read' ? page.softTagNeutral : page.softTagRose}>
          {s === 'read' ? '已读' : '未读'}
        </Tag>
      ),
    },
    {
      title: '时间', dataIndex: 'createdAt', width: 160,
      render: (t) => dayjs(t).format('YYYY-MM-DD HH:mm'),
    },
    {
      title: '操作', width: 80, fixed: 'right',
      render: (_, r) => (
        <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => void handleViewDetail(r)}>
          查看
        </Button>
      ),
    },
  ]

  return (
    <div className={page.pageShell}>
      <PageHeader
        title="预警中心"
        subtitle="查看预警记录，管理自动触发规则"
        icon={<BellOutlined />}
        extra={
          <Button icon={<BellOutlined />} loading={evaluating} onClick={() => void handleEvaluate()}>
            立即评估
          </Button>
        }
      />

      <Card bordered={false} className={`${page.panelCard} ${page.tabsPanel}`}>
        <Tabs
          defaultActiveKey="records"
          items={[
            {
              key: 'records',
              label: '预警记录',
              children: (
                <Table
                  rowKey="id"
                  columns={recordColumns}
                  dataSource={records}
                  loading={loading}
                  pagination={{
                    total: recordTotal,
                    showTotal: (t) => `共 ${t} 条`,
                    onChange: (p) => fetchRecords(p),
                  }}
                />
              ),
            },
            {
              key: 'rules',
              label: '预警规则',
              children: (
                <>
                  <div style={{ marginBottom: 12 }}>
                    <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                      新建规则
                    </Button>
                  </div>
                  <Table rowKey="id" columns={ruleColumns} dataSource={rules} scroll={{ x: 1020 }} />
                </>
              ),
            },
          ]}
        />
      </Card>

      <Modal
        title={editingId ? '编辑预警规则' : '新建预警规则'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={closeModal}
        confirmLoading={submitting}
        width={560}
        destroyOnClose
      >
        <Form form={form} layout="vertical" initialValues={DEFAULT_FORM}>
          <Form.Item name="name" label="规则名称" rules={[{ required: true, message: '请输入规则名称' }]}>
            <Input placeholder="如：负面舆情预警" />
          </Form.Item>

          <Form.Item name="remark" label="分析方向备注" extra="可选，填写后 AI 将按此方向生成分析建议">
            <Input.TextArea
              placeholder="如：关注食品安全风险、分析对品牌形象的影响、评估政策合规性等"
              rows={2}
              maxLength={200}
              showCount
            />
          </Form.Item>

          <Form.Item label="监测关键词" style={{ marginBottom: 8 }}>
            <Form.Item
              name="keywordListAnd"
              label="必须包含（AND）"
              extra="所有关键词都必须匹配"
              style={{ marginBottom: 12 }}
            >
              <Select
                mode="tags"
                placeholder="输入关键词后回车，可留空"
                tokenSeparators={[',', '，']}
                open={false}
                suffixIcon={null}
              />
            </Form.Item>
            <Form.Item
              name="keywordListOr"
              label="任一包含（OR）"
              extra="至少一个关键词匹配即可"
              style={{ marginBottom: 0 }}
            >
              <Select
                mode="tags"
                placeholder="输入关键词后回车，可留空"
                tokenSeparators={[',', '，']}
                open={false}
                suffixIcon={null}
              />
            </Form.Item>
          </Form.Item>

          <Space style={{ width: '100%' }} size={16}>
            <Form.Item name="sentiment" label="触发情感" style={{ width: 120, marginBottom: 0 }}>
              <Select options={[
                { value: '', label: '全部' },
                { value: 'negative', label: '负面' },
                { value: 'neutral', label: '中性' },
                { value: 'positive', label: '正面' },
              ]} />
            </Form.Item>
            <Form.Item name="timeRangeDays" label="时间范围" style={{ width: 140, marginBottom: 0 }}>
              <Select options={[
                { value: 1, label: '最近1天' },
                { value: 3, label: '最近3天' },
                { value: 7, label: '最近7天' },
              ]} />
            </Form.Item>
          </Space>

          <Space style={{ width: '100%', marginTop: 16 }} size={16}>
            <Form.Item name="threshold" label="触发阈值" extra="今日匹配文章数达到此值才告警，测试建议设为 1">
              <InputNumber min={1} style={{ width: 140 }} addonAfter="条" />
            </Form.Item>
            <Form.Item name="interval" label="检测间隔" extra="两次告警之间的最小冷却时间（分钟），不影响统计窗口（统计今日 0 点至今的文章）">
              <InputNumber min={1} style={{ width: 140 }} addonAfter="分钟" />
            </Form.Item>
          </Space>

          <Form.Item name="status" label="规则状态" valuePropName="checked">
            <Switch checkedChildren="启用" unCheckedChildren="停用" />
          </Form.Item>

          <Form.Item
            name="notifyEmail"
            label="通知邮箱"
            rules={[
              { required: true, message: '请输入邮箱' },
              { type: 'email', message: '邮箱格式不正确' },
            ]}
          >
            <Input placeholder="admin@example.com" />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title="预警详情"
        open={detailDrawerOpen}
        onClose={closeDetailDrawer}
        width={720}
      >
        {currentRecord && (() => {
          const parsed = parseAlertContent(currentRecord.content)
          return (
            <div>
              <Typography.Title level={5} style={{ marginTop: 0 }}>
                {currentRecord.title}
              </Typography.Title>

              <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Card size="small" title="基本信息" style={{ background: '#fafafa' }}>
                  <Space direction="vertical" size={8} style={{ width: '100%' }}>
                    {parsed.rule && (
                      <div>
                        <span style={{ color: '#666' }}>触发规则：</span>
                        <Tag className={page.softTagSage}>{parsed.rule}</Tag>
                      </div>
                    )}
                    {parsed.timeWindow && (
                      <div>
                        <span style={{ color: '#666' }}>时间窗口：</span>
                        <span>{parsed.timeWindow}</span>
                      </div>
                    )}
                    {parsed.keywords && (
                      <div>
                        <span style={{ color: '#666' }}>关键词：</span>
                        <span>{parsed.keywords}</span>
                      </div>
                    )}
                    {parsed.sentiment && (
                      <div>
                        <span style={{ color: '#666' }}>情感：</span>
                        <span>{parsed.sentiment}</span>
                      </div>
                    )}
                    {parsed.matchCount && (
                      <div>
                        <span style={{ color: '#666' }}>匹配条数：</span>
                        <span>{parsed.matchCount}</span>
                      </div>
                    )}
                    <div>
                      <span style={{ color: '#666' }}>触发时间：</span>
                      <span>{dayjs(currentRecord.createdAt).format('YYYY-MM-DD HH:mm:ss')}</span>
                    </div>
                  </Space>
                </Card>

                {parsed.aiAnalysis && (
                  <Card size="small" title="AI 分析" style={{ background: '#f0f9ff' }}>
                    <div style={{
                      lineHeight: 1.8,
                      paddingLeft: '4px',
                    } as React.CSSProperties}>
                      <ReactMarkdown
                        components={{
                          ol: ({ children }) => (
                            <ol style={{ paddingLeft: '1.5em', margin: '0.5em 0' }}>{children}</ol>
                          ),
                          ul: ({ children }) => (
                            <ul style={{ paddingLeft: '1.5em', margin: '0.5em 0' }}>{children}</ul>
                          ),
                          li: ({ children }) => (
                            <li style={{ marginBottom: '0.25em' }}>{children}</li>
                          ),
                          p: ({ children }) => (
                            <p style={{ margin: '0.5em 0' }}>{children}</p>
                          ),
                          strong: ({ children }) => (
                            <strong style={{ fontWeight: 600 }}>{children}</strong>
                          ),
                        }}
                      >
                        {parsed.aiAnalysis}
                      </ReactMarkdown>
                    </div>
                  </Card>
                )}

                {parsed.articles && parsed.articles.length > 0 && (
                  <Card size="small" title={`匹配文章（${parsed.articles.length} 条）`}>
                    <ul style={{ margin: 0, paddingLeft: 20 }}>
                      {parsed.articles.map((article, idx) => (
                        <li key={idx} style={{ marginBottom: 8 }}>
                          {article}
                        </li>
                      ))}
                    </ul>
                  </Card>
                )}
              </Space>
            </div>
          )
        })()}
      </Drawer>
    </div>
  )
}

export default AlertsPage

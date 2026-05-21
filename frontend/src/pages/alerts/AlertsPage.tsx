import React, { useEffect, useState } from 'react'
import {
  Table, Button, Modal, Form, Input, Select, InputNumber,
  Space, Tag, Popconfirm, Tabs, message, Card, Switch,
} from 'antd'
import { PlusOutlined, DeleteOutlined, EditOutlined, BellOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { alertApi } from '@/api/alert'
import PageHeader from '@/components/common/PageHeader'
import page from '@/styles/page.module.css'
import type { AlertRule, AlertRecord, AlertRulePayload } from '@/types'

const NOTIFY_TYPE_LABEL: Record<string, string> = {
  email: '邮件',
  webhook: 'Webhook',
  sms: '短信',
}

const SENTIMENT_LABEL: Record<string, string> = {
  negative: '负面',
  positive: '正面',
}

const DEFAULT_FORM = {
  threshold: 10,
  interval: 60,
  status: 1,
  notifyType: 'email',
  sentiment: '',
  keywordList: [] as string[],
}

function buildPayload(values: Record<string, unknown>): AlertRulePayload {
  return {
    name: values.name as string,
    keywordList: (values.keywordList as string[] | undefined) ?? [],
    sentiment: (values.sentiment as string | undefined) ?? '',
    threshold: values.threshold as number,
    interval: values.interval as number,
    notifyType: values.notifyType as string,
    notifyEmail: values.notifyEmail as string | undefined,
    notifyWebhook: values.notifyWebhook as string | undefined,
    notifyPhone: values.notifyPhone as string | undefined,
    status: values.status ? 1 : 0,
  }
}

function ruleToFormValues(rule: AlertRule): Record<string, unknown> {
  const keywordList = rule.keywords
    ? rule.keywords.split(',').map((k) => k.trim()).filter(Boolean)
    : []
  const values: Record<string, unknown> = {
    name: rule.name,
    keywordList,
    sentiment: rule.sentiment || '',
    threshold: rule.threshold,
    interval: rule.interval,
    notifyType: rule.notifyType,
    status: rule.status === 1,
  }
  if (rule.notifyType === 'email') values.notifyEmail = rule.notifyConf !== '-' ? rule.notifyConf : ''
  if (rule.notifyType === 'webhook') values.notifyWebhook = rule.notifyConf !== '-' ? rule.notifyConf : ''
  if (rule.notifyType === 'sms') values.notifyPhone = rule.notifyConf !== '-' ? rule.notifyConf : ''
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
  const [form] = Form.useForm()
  const notifyType = Form.useWatch('notifyType', form)

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
        fetchRecords()
      } else {
        message.success(res.message)
      }
    } finally {
      setEvaluating(false)
    }
  }

  const ruleColumns: ColumnsType<AlertRule> = [
    { title: '规则名称', dataIndex: 'name' },
    { title: '关键词', dataIndex: 'keywords', render: (v) => v || '全部' },
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

          <Space style={{ width: '100%', alignItems: 'flex-start' }} size={16}>
            <Form.Item
              name="keywordList"
              label="监测关键词"
              style={{ flex: 1, marginBottom: 0 }}
              extra="选「全部」情感时可留空；输入后按回车添加"
            >
              <Select
                mode="tags"
                placeholder="输入关键词后回车"
                tokenSeparators={[',', '，']}
                open={false}
                suffixIcon={null}
              />
            </Form.Item>
            <Form.Item name="sentiment" label="触发情感" style={{ width: 120, marginBottom: 0 }}>
              <Select options={[
                { value: '', label: '全部' },
                { value: 'negative', label: '负面' },
                { value: 'positive', label: '正面' },
              ]} />
            </Form.Item>
          </Space>

          <Space style={{ width: '100%', marginTop: 16 }} size={16}>
            <Form.Item name="threshold" label="触发阈值" extra="达到该条数时触发">
              <InputNumber min={1} style={{ width: 140 }} addonAfter="条" />
            </Form.Item>
            <Form.Item name="interval" label="检测间隔" extra="两次检测的最小间隔">
              <InputNumber min={1} style={{ width: 140 }} addonAfter="分钟" />
            </Form.Item>
          </Space>

          <Form.Item name="status" label="规则状态" valuePropName="checked">
            <Switch checkedChildren="启用" unCheckedChildren="停用" />
          </Form.Item>

          <Form.Item
            name="notifyType"
            label="通知方式"
            rules={[{ required: true, message: '请选择通知方式' }]}
          >
            <Select options={[
              { value: 'email', label: '邮件' },
              { value: 'webhook', label: 'Webhook' },
              { value: 'sms', label: '短信' },
            ]} />
          </Form.Item>

          {notifyType === 'email' && (
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
          )}

          {notifyType === 'webhook' && (
            <Form.Item
              name="notifyWebhook"
              label="Webhook 地址"
              rules={[
                { required: true, message: '请输入 Webhook 地址' },
                { type: 'url', message: '请输入有效的 URL' },
              ]}
            >
              <Input placeholder="https://example.com/hook" />
            </Form.Item>
          )}

          {notifyType === 'sms' && (
            <Form.Item
              name="notifyPhone"
              label="手机号"
              rules={[
                { required: true, message: '请输入手机号' },
                { pattern: /^1\d{10}$/, message: '请输入 11 位手机号' },
              ]}
            >
              <Input placeholder="13800138000" maxLength={11} />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </div>
  )
}

export default AlertsPage

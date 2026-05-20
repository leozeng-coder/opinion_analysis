import React, { useEffect, useState } from 'react'
import {
  Table, Button, Modal, Form, Input, Select, InputNumber,
  Space, Tag, Popconfirm, Tabs, message, Card,
} from 'antd'
import { PlusOutlined, DeleteOutlined, BellOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { alertApi } from '@/api/alert'
import PageHeader from '@/components/common/PageHeader'
import page from '@/styles/page.module.css'
import type { AlertRule, AlertRecord } from '@/types'

const AlertsPage: React.FC = () => {
  const [rules, setRules] = useState<AlertRule[]>([])
  const [records, setRecords] = useState<AlertRecord[]>([])
  const [recordTotal, setRecordTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [form] = Form.useForm()

  const fetchRules = async () => {
    const data = await alertApi.listRules()
    setRules(data)
  }

  const fetchRecords = async (pageNum = 1) => {
    setLoading(true)
    try {
      const res = await alertApi.listRecords({ page: pageNum, pageSize: 20 })
      setRecords(res.list)
      setRecordTotal(res.total)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchRules(); fetchRecords() }, [])

  const handleCreate = async () => {
    const values = await form.validateFields()
    await alertApi.createRule(values)
    message.success('预警规则创建成功')
    setModalOpen(false)
    form.resetFields()
    fetchRules()
  }

  const handleDelete = async (id: number) => {
    await alertApi.deleteRule(id)
    message.success('删除成功')
    fetchRules()
  }

  const ruleColumns: ColumnsType<AlertRule> = [
    { title: '规则名称', dataIndex: 'name' },
    { title: '关键词', dataIndex: 'keywords', render: (v) => v || '-' },
    {
      title: '触发情感', dataIndex: 'sentiment',
      render: (s) => s ? <Tag className={page.softTagNeutral}>{s}</Tag> : '全部',
    },
    { title: '阈值', dataIndex: 'threshold', width: 80 },
    { title: '间隔(分)', dataIndex: 'interval', width: 90 },
    { title: '通知方式', dataIndex: 'notifyType', width: 100 },
    {
      title: '状态', dataIndex: 'status', width: 80,
      render: (s) => (
        <Tag className={s ? page.softTagSage : page.softTagNeutral}>
          {s ? '启用' : '停用'}
        </Tag>
      ),
    },
    {
      title: '操作', width: 80, fixed: 'right',
      render: (_, r) => (
        <Popconfirm title="确认删除?" onConfirm={() => handleDelete(r.id)}>
          <Button type="link" danger size="small" icon={<DeleteOutlined />}>删除</Button>
        </Popconfirm>
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
                    <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>
                      新建规则
                    </Button>
                  </div>
                  <Table rowKey="id" columns={ruleColumns} dataSource={rules} scroll={{ x: 800 }} />
                </>
              ),
            },
          ]}
        />
      </Card>

      <Modal
        title="新建预警规则"
        open={modalOpen}
        onOk={handleCreate}
        onCancel={() => { setModalOpen(false); form.resetFields() }}
        width={560}
      >
        <Form form={form} layout="vertical" initialValues={{ threshold: 10, interval: 60, status: 1 }}>
          <Form.Item name="name" label="规则名称" rules={[{ required: true }]}>
            <Input placeholder="如：负面舆情预警" />
          </Form.Item>
          <Space style={{ width: '100%' }} size={16}>
            <Form.Item name="keywords" label="关键词(逗号分隔)" style={{ flex: 1 }}>
              <Input placeholder="关键词1,关键词2" />
            </Form.Item>
            <Form.Item name="sentiment" label="触发情感" style={{ width: 120 }}>
              <Select options={[
                { value: '', label: '全部' },
                { value: 'negative', label: '负面' },
                { value: 'positive', label: '正面' },
              ]} />
            </Form.Item>
          </Space>
          <Space style={{ width: '100%' }} size={16}>
            <Form.Item name="threshold" label="触发阈值">
              <InputNumber min={1} style={{ width: 120 }} addonAfter="条" />
            </Form.Item>
            <Form.Item name="interval" label="检测间隔">
              <InputNumber min={1} style={{ width: 120 }} addonAfter="分钟" />
            </Form.Item>
          </Space>
          <Form.Item name="notifyType" label="通知方式" rules={[{ required: true }]}>
            <Select options={[
              { value: 'email', label: '邮件' },
              { value: 'webhook', label: 'Webhook' },
              { value: 'sms', label: '短信' },
            ]} />
          </Form.Item>
          <Form.Item name="notifyConf" label="通知配置(JSON)">
            <Input.TextArea rows={3} placeholder='{"email": "admin@example.com"}' />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default AlertsPage

import React, { useCallback, useEffect, useState } from 'react'
import {
  Badge, Button, Form, Input, message, Modal, Popconfirm, Select, Space, Table, Tag, Typography,
} from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { adminDataSourceApi } from '@/api/admin-datasource'
import type { DataSource } from '@/types'

const { Title, Text } = Typography

const TYPE_OPTIONS = [
  { label: 'weibo', value: 'weibo' }, { label: 'weixin', value: 'weixin' },
  { label: 'news', value: 'news' }, { label: 'forum', value: 'forum' },
  { label: 'xhs', value: 'xhs' }, { label: 'douyin', value: 'dy' },
  { label: 'kuaishou', value: 'ks' }, { label: 'bilibili', value: 'bili' },
  { label: 'tieba', value: 'tieba' }, { label: 'zhihu', value: 'zhihu' },
]

type DSForm = { name: string; type: string; url?: string; config?: string; status?: number }

function validateJSON(s?: string): Promise<void> {
  if (!s || !s.trim()) return Promise.resolve()
  try { JSON.parse(s); return Promise.resolve() } catch (e) { return Promise.reject(new Error('JSON 格式错误')) }
}

const DataSourcePage: React.FC = () => {
  const [list, setList] = useState<DataSource[]>([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<DataSource | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm<DSForm>()

  const fetch = useCallback(async () => {
    setLoading(true)
    try { setList(await adminDataSourceApi.list()) } finally { setLoading(false) }
  }, [])

  useEffect(() => { void fetch() }, [fetch])

  const openCreate = () => { setEditing(null); form.resetFields(); setModalOpen(true) }
  const openEdit = (ds: DataSource) => {
    setEditing(ds)
    form.setFieldsValue({ name: ds.name, type: ds.type, url: ds.url, config: ds.config, status: ds.status })
    setModalOpen(true)
  }

  const handleSubmit = async () => {
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      if (editing) {
        await adminDataSourceApi.update(editing.id, values)
        void message.success('已更新')
      } else {
        await adminDataSourceApi.create(values)
        void message.success('已创建')
      }
      setModalOpen(false); void fetch()
    } finally { setSubmitting(false) }
  }

  const handleDelete = async (id: number) => {
    try { await adminDataSourceApi.delete(id); void message.success('已删除'); void fetch() } catch { /* handled */ }
  }

  const columns: ColumnsType<DataSource> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '名称', dataIndex: 'name', width: 160 },
    {
      title: '类型', dataIndex: 'type', width: 100,
      render: (t: string) => <Tag>{t}</Tag>,
    },
    { title: 'URL', dataIndex: 'url', ellipsis: true },
    {
      title: 'Config（脱敏）', dataIndex: 'config', width: 200,
      render: (c: string) => <Text code style={{ fontSize: 11 }}>{c ? (c.length > 80 ? c.slice(0, 80) + '…' : c) : '—'}</Text>,
    },
    {
      title: '状态', dataIndex: 'status', width: 80,
      render: (s: number) => <Badge status={s === 1 ? 'success' : 'error'} text={s === 1 ? '启用' : '停用'} />,
    },
    {
      title: '更新', dataIndex: 'updatedAt', width: 130,
      render: (t: string) => dayjs(t).format('MM-DD HH:mm'),
    },
    {
      title: '操作', key: 'ops', width: 110,
      render: (_, record) => (
        <Space>
          <Button type="link" size="small" onClick={() => openEdit(record)}>编辑</Button>
          <Popconfirm title="确定删除？" okText="删除" okType="danger" cancelText="取消" onConfirm={() => void handleDelete(record.id)}>
            <Button type="link" size="small" danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Title level={4} style={{ marginTop: 0 }}>数据源管理</Title>
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新建数据源</Button>
        <Button icon={<ReloadOutlined />} onClick={() => void fetch()}>刷新</Button>
      </Space>
      <Table<DataSource> rowKey="id" columns={columns} dataSource={list} loading={loading}
        pagination={{ pageSize: 30, showSizeChanger: false }} size="middle" scroll={{ x: 980 }} />

      <Modal title={editing ? '编辑数据源' : '新建数据源'} open={modalOpen}
        onCancel={() => setModalOpen(false)} onOk={() => void handleSubmit()}
        confirmLoading={submitting} okText={editing ? '保存' : '创建'} cancelText="取消" width={560} destroyOnClose>
        <Form form={form} layout="vertical">
          <Form.Item label="名称" name="name" rules={[{ required: true }]}>
            <Input placeholder="例如：微博舆情" />
          </Form.Item>
          <Form.Item label="类型" name="type" rules={[{ required: true }]}>
            <Select options={TYPE_OPTIONS} placeholder="选择数据源类型" />
          </Form.Item>
          <Form.Item label="URL" name="url">
            <Input placeholder="https://..." />
          </Form.Item>
          <Form.Item
            label="配置（JSON，敏感字段会在列表展示时脱敏）"
            name="config"
            rules={[{ validator: (_, v) => validateJSON(v as string) }]}
          >
            <Input.TextArea rows={5} placeholder='{"cookie":"...","apiKey":"..."}' style={{ fontFamily: 'monospace' }} />
          </Form.Item>
          <Form.Item label="状态" name="status" initialValue={1}>
            <Select options={[{ label: '启用', value: 1 }, { label: '停用', value: 0 }]} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default DataSourcePage

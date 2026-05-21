import React, { useCallback, useEffect, useState } from 'react'
import {
  Button, Card, DatePicker, Input, Select, Space, Table, Tag, Tooltip, Typography,
} from 'antd'
import { AuditOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { adminAuditApi } from '@/api/admin-audit'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'
import type { AuditLog } from '@/types'

const { Text } = Typography
const { RangePicker } = DatePicker

const ACTION_OPTIONS = [
  { label: '创建', value: 'create' },
  { label: '更新', value: 'update' },
  { label: '删除', value: 'delete' },
  { label: '执行任务', value: 'run' },
  { label: '重置密码', value: 'reset_password' },
  { label: '更新爬虫', value: 'update_spiders' },
]

const RESOURCE_OPTIONS = [
  { label: '用户', value: 'user' },
  { label: '爬虫', value: 'crawler' },
  { label: '打标器', value: 'tagger' },
  { label: '告警规则', value: 'alert_rule' },
  { label: '系统设置', value: 'system_setting' },
  { label: '数据源', value: 'data_source' },
]

const actionColor: Record<string, string> = {
  create: 'green', update: 'blue', delete: 'red',
  run: 'purple', reset_password: 'orange', update_spiders: 'cyan',
}

const AuditPage: React.FC = () => {
  const [list, setList] = useState<AuditLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)

  const [actorName, setActorName] = useState('')
  const [action, setAction] = useState('')
  const [resource, setResource] = useState('')
  const [range, setRange] = useState<[string, string] | null>(null)

  const fetch = useCallback(async (p = 1) => {
    setLoading(true)
    try {
      const res = await adminAuditApi.list({
        page: p,
        pageSize: 30,
        actorName: actorName || undefined,
        action: action || undefined,
        resource: resource || undefined,
        startAt: range?.[0] || undefined,
        endAt: range?.[1] || undefined,
      })
      setList(res.list)
      setTotal(res.total)
    } finally {
      setLoading(false)
    }
  }, [actorName, action, resource, range])

  useEffect(() => {
    void fetch(1)
  }, [fetch])

  const handleSearch = () => {
    setPage(1)
    void fetch(1)
  }

  const handleActionChange = (v: string | undefined) => {
    const newAction = v ?? ''
    setAction(newAction)
    setPage(1)
    // 立即使用新值查询，而不是等待状态更新
    void adminAuditApi
      .list({
        page: 1,
        pageSize: 30,
        actorName: actorName || undefined,
        action: newAction || undefined,
        resource: resource || undefined,
        startAt: range?.[0] || undefined,
        endAt: range?.[1] || undefined,
      })
      .then((res) => {
        setList(res.list)
        setTotal(res.total)
      })
  }

  const handleResourceChange = (v: string | undefined) => {
    const newResource = v ?? ''
    setResource(newResource)
    setPage(1)
    // 立即使用新值查询，而不是等待状态更新
    void adminAuditApi
      .list({
        page: 1,
        pageSize: 30,
        actorName: actorName || undefined,
        action: action || undefined,
        resource: newResource || undefined,
        startAt: range?.[0] || undefined,
        endAt: range?.[1] || undefined,
      })
      .then((res) => {
        setList(res.list)
        setTotal(res.total)
      })
  }

  const columns: ColumnsType<AuditLog> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '操作人', dataIndex: 'actorName', width: 100 },
    {
      title: '动作', dataIndex: 'action', width: 120,
      render: (a: string) => <Tag color={actionColor[a] ?? 'default'}>{a}</Tag>,
    },
    {
      title: '资源', dataIndex: 'resource', width: 120,
      render: (r: string) => <Tag>{r}</Tag>,
    },
    { title: '资源ID', dataIndex: 'resourceId', width: 70 },
    { title: '路径', dataIndex: 'path', width: 220, ellipsis: true },
    {
      title: '状态', dataIndex: 'status', width: 70,
      render: (s: number) => <Tag color={s < 400 ? 'green' : 'red'}>{s}</Tag>,
    },
    {
      title: '请求摘要', dataIndex: 'payload', ellipsis: true,
      render: (p: string) => (
        <Tooltip title={p}>
          <Text code style={{ fontSize: 11 }}>{p ? (p.length > 80 ? p.slice(0, 80) + '…' : p) : '—'}</Text>
        </Tooltip>
      ),
    },
    { title: 'IP', dataIndex: 'ip', width: 120 },
    {
      title: '时间', dataIndex: 'createdAt', width: 155,
      render: (t: string) => dayjs(t).format('YYYY-MM-DD HH:mm:ss'),
    },
  ]

  return (
    <div className={ui.pageShell}>
      <PageHeader
        title="审计日志"
        subtitle="追踪管理后台的操作记录与请求摘要"
        icon={<AuditOutlined />}
        extra={
          <Space wrap>
            <Input
              allowClear
              placeholder="操作人"
              style={{ width: 140 }}
              value={actorName}
              onChange={(e) => setActorName(e.target.value)}
              onPressEnter={handleSearch}
            />
            <Select
              allowClear
              placeholder="动作"
              style={{ width: 140 }}
              value={action || undefined}
              onChange={handleActionChange}
              options={ACTION_OPTIONS}
            />
            <Select
              allowClear
              placeholder="资源"
              style={{ width: 140 }}
              value={resource || undefined}
              onChange={handleResourceChange}
              options={RESOURCE_OPTIONS}
            />
            <RangePicker
              showTime
              style={{ width: 360 }}
              onChange={(_, strs) => {
                setRange(strs[0] && strs[1] ? [strs[0], strs[1]] : null)
                setPage(1)
              }}
            />
            <Button icon={<SearchOutlined />} type="primary" onClick={handleSearch}>
              查询
            </Button>
            <Button icon={<ReloadOutlined />} className={ui.ghostBtn} onClick={handleSearch}>
              刷新
            </Button>
          </Space>
        }
      />
      <Card bordered={false} className={`${ui.panelCard} ${ui.tableWrap}`}>
      <Table<AuditLog> rowKey="id" columns={columns} dataSource={list} loading={loading}
        pagination={{ current: page, pageSize: 30, total, showSizeChanger: false, onChange: (p) => { setPage(p); void fetch(p) } }}
        size="middle" scroll={{ x: 1400 }} />
      </Card>
    </div>
  )
}

export default AuditPage

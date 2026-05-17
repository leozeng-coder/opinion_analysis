import React, { useCallback, useEffect, useState } from 'react'
import {
  Badge,
  Button,
  Form,
  Input,
  message,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd'
import { CopyOutlined, KeyOutlined, ReloadOutlined, UserAddOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { adminUserApi } from '@/api/admin-user'
import type { User } from '@/types'

const { Title, Text } = Typography

const roleOptions = [
  { label: 'admin', value: 'admin' },
  { label: 'analyst', value: 'analyst' },
  { label: 'viewer', value: 'viewer' },
]

const roleColor: Record<string, string> = { admin: 'red', analyst: 'blue', viewer: 'default' }

const UsersPage: React.FC = () => {
  const [users, setUsers] = useState<User[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [newPwd, setNewPwd] = useState<string | null>(null)

  const fetch = useCallback(async (p = 1, kw = keyword) => {
    setLoading(true)
    try {
      const res = await adminUserApi.list({ page: p, pageSize: 20, keyword: kw })
      setUsers(res.list)
      setTotal(res.total)
    } finally {
      setLoading(false)
    }
  }, [keyword])

  useEffect(() => { void fetch(1) }, [fetch])

  const handleRoleChange = async (id: number, role: string) => {
    try {
      await adminUserApi.update(id, { role })
      void message.success('角色已更新')
      void fetch(page)
    } catch {
      /* handled by interceptor */
    }
  }

  const handleToggle = async (user: User) => {
    const newStatus = user.status === 1 ? 0 : 1
    try {
      await adminUserApi.update(user.id, { status: newStatus })
      void message.success(newStatus === 1 ? '已启用' : '已禁用')
      void fetch(page)
    } catch {
      /* handled */
    }
  }

  const handleResetPwd = async (id: number) => {
    try {
      const res = await adminUserApi.resetPassword(id)
      setNewPwd(res.password)
    } catch {
      /* handled */
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await adminUserApi.delete(id)
      void message.success('用户已删除')
      void fetch(page)
    } catch {
      /* handled */
    }
  }

  const columns: ColumnsType<User> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '用户名', dataIndex: 'username', width: 120 },
    { title: '昵称', dataIndex: 'nickname', width: 120, ellipsis: true },
    { title: '邮箱', dataIndex: 'email', width: 200, ellipsis: true },
    {
      title: '角色',
      dataIndex: 'role',
      width: 130,
      render: (role: string, record) => (
        <Select
          value={role}
          size="small"
          style={{ width: 110 }}
          options={roleOptions}
          onChange={(v) => void handleRoleChange(record.id, v)}
        />
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (s: number, record) => (
        <Badge
          status={s === 1 ? 'success' : 'error'}
          text={
            <Button type="link" size="small" onClick={() => void handleToggle(record)}>
              {s === 1 ? '启用' : '禁用'}
            </Button>
          }
        />
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'createdAt',
      width: 170,
      render: (t: string) => dayjs(t).format('YYYY-MM-DD HH:mm'),
    },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      render: (_, record) => (
        <Space size={0}>
          <Tooltip title="重置密码">
            <Button
              type="link"
              size="small"
              icon={<KeyOutlined />}
              onClick={() => void handleResetPwd(record.id)}
            />
          </Tooltip>
          <Popconfirm
            title="确定删除该用户？"
            okText="删除"
            okType="danger"
            cancelText="取消"
            onConfirm={() => void handleDelete(record.id)}
          >
            <Button type="link" size="small" danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Title level={4} style={{ marginTop: 0 }}>用户管理</Title>
      <Space style={{ marginBottom: 16 }} wrap>
        <Input.Search
          placeholder="搜索用户名 / 邮箱 / 昵称"
          allowClear
          style={{ width: 260 }}
          onSearch={(v) => { setKeyword(v); setPage(1); void fetch(1, v) }}
        />
        <Button icon={<ReloadOutlined />} onClick={() => void fetch(page)}>刷新</Button>
      </Space>
      <Table<User>
        rowKey="id"
        columns={columns}
        dataSource={users}
        loading={loading}
        pagination={{
          current: page,
          pageSize: 20,
          total,
          showSizeChanger: false,
          onChange: (p) => { setPage(p); void fetch(p) },
        }}
        size="middle"
        scroll={{ x: 980 }}
      />

      <Modal
        title={<Space><KeyOutlined />重置密码成功</Space>}
        open={newPwd !== null}
        onCancel={() => setNewPwd(null)}
        footer={<Button type="primary" onClick={() => setNewPwd(null)}>关闭</Button>}
      >
        <p style={{ marginBottom: 8 }}>新密码（关闭后不再显示）：</p>
        <Space>
          <Text code style={{ fontSize: 18 }}>{newPwd}</Text>
          <Button
            icon={<CopyOutlined />}
            size="small"
            onClick={() => {
              if (newPwd) { void navigator.clipboard.writeText(newPwd); void message.success('已复制') }
            }}
          >
            复制
          </Button>
        </Space>
        <p style={{ marginTop: 12, color: '#888', fontSize: 12 }}>请立即告知用户并让其修改密码。</p>
      </Modal>
    </div>
  )
}

export default UsersPage

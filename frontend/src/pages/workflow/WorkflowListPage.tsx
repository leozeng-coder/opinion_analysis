import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Table, Button, Space, Tag, message, Popconfirm } from 'antd'
import { PlusOutlined, DeleteOutlined, PlayCircleOutlined, ClockCircleOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import PageHeader from '@/components/common/PageHeader'
import { workflowApi } from '@/api/workflow'
import { Workflow } from '@/types'

const WorkflowListPage: React.FC = () => {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)

  useEffect(() => {
    fetchWorkflows()
  }, [page, pageSize])

  const fetchWorkflows = async () => {
    setLoading(true)
    try {
      const res = await workflowApi.list({ page, pageSize })
      setWorkflows(res.list || [])
      setTotal(res.total || 0)
    } catch (error) {
      message.error('加载工作流列表失败')
    } finally {
      setLoading(false)
    }
  }

  // 进入编辑器并自动触发执行，在内置控制台查看实时日志
  const handleExecute = (id: number) => {
    navigate(`/workflows/${id}/edit`, { state: { autoRun: true } })
  }

  const handleDelete = async (id: number) => {
    try {
      await workflowApi.delete(id)
      message.success('删除成功')
      fetchWorkflows()
    } catch (error) {
      message.error('删除失败')
    }
  }

  const columns: ColumnsType<Workflow> = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
    },
    {
      title: '工作流名称',
      dataIndex: 'name',
      width: 200,
    },
    {
      title: '描述',
      dataIndex: 'description',
      ellipsis: true,
    },
    {
      title: '触发类型',
      dataIndex: 'triggerType',
      width: 120,
      render: (type: string) => {
        const typeMap: Record<string, { label: string; color: string; icon: React.ReactNode }> = {
          schedule: { label: '定时', color: 'blue', icon: <ClockCircleOutlined /> },
          manual: { label: '手动', color: 'green', icon: <PlayCircleOutlined /> },
          webhook: { label: 'Webhook', color: 'purple', icon: <PlayCircleOutlined /> },
        }
        const config = typeMap[type] || { label: type, color: 'default', icon: null }
        return (
          <Tag color={config.color} icon={config.icon}>
            {config.label}
          </Tag>
        )
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (status: number) => (
        <Tag color={status === 1 ? 'success' : 'default'}>
          {status === 1 ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'createdAt',
      width: 180,
      render: (time: string) => time?.replace('T', ' ').slice(0, 19),
    },
    {
      title: '操作',
      key: 'action',
      width: 220,
      fixed: 'right',
      render: (_, record) => (
        <Space size="small">
          <Button
            type="link"
            size="small"
            icon={<PlayCircleOutlined />}
            onClick={() => handleExecute(record.id!)}
          >
            执行
          </Button>
          <Popconfirm
            title="确定删除此工作流吗？"
            onConfirm={() => handleDelete(record.id!)}
            okText="确定"
            cancelText="取消"
          >
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <PageHeader
        title="工作流编排"
        extra={
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => navigate('/workflows/new')}
          >
            新建工作流
          </Button>
        }
      />

      <div style={{ padding: '24px', background: '#fff' }}>
        <Table
          columns={columns}
          dataSource={workflows}
          rowKey="id"
          loading={loading}
          onRow={(record) => ({
            onDoubleClick: () => navigate(`/workflows/${record.id}/edit`),
            style: { cursor: 'pointer' },
          })}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条`,
            onChange: (page, pageSize) => {
              setPage(page)
              setPageSize(pageSize)
            },
          }}
        />
      </div>
    </div>
  )
}

export default WorkflowListPage

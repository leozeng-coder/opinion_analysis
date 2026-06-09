import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Table, Button, Tag, Space, message, Card, Modal, Timeline, Spin, Popconfirm, Alert } from 'antd'
import { ArrowLeftOutlined, EyeOutlined, CheckCircleOutlined, CloseCircleOutlined, SyncOutlined, ExclamationCircleOutlined, StopOutlined, DownloadOutlined, FileTextOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import PageHeader from '@/components/common/PageHeader'
import { workflowApi, reportApi } from '@/api/workflow'
import { WorkflowExecution, WorkflowNodeExecution } from '@/types'
import { useAuthStore } from '@/store/auth'

const WorkflowExecutionPage: React.FC = () => {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const [loading, setLoading] = useState(false)
  const [executions, setExecutions] = useState<WorkflowExecution[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [workflowName, setWorkflowName] = useState('')

  // 查看详情
  const [detailModalVisible, setDetailModalVisible] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [selectedExecution, setSelectedExecution] = useState<WorkflowExecution | null>(null)
  const [nodeLogs, setNodeLogs] = useState<WorkflowNodeExecution[]>([])
  const [regenerating, setRegenerating] = useState(false)

  useEffect(() => {
    if (id) {
      fetchWorkflowInfo()
      fetchExecutions()
    }
  }, [id, page, pageSize])

  // 自动刷新：如果有运行中的执行，每3秒刷新一次
  useEffect(() => {
    const hasRunning = executions.some(e => e.status === 'running')
    if (!hasRunning) return

    const timer = setInterval(() => {
      fetchExecutions()
    }, 3000)

    return () => clearInterval(timer)
  }, [executions, id, page, pageSize])

  const fetchWorkflowInfo = async () => {
    try {
      const workflow = await workflowApi.detail(Number(id))
      setWorkflowName(workflow.name)
    } catch (error) {
      message.error('加载工作流信息失败')
    }
  }

  const fetchExecutions = async () => {
    setLoading(true)
    try {
      const res = await workflowApi.executions(Number(id), { page, pageSize })
      setExecutions(res.list || [])
      setTotal(res.total || 0)
    } catch (error) {
      message.error('加载执行历史失败')
    } finally {
      setLoading(false)
    }
  }

  const downloadReport = async (reportId: string, reportFormat?: string) => {
    try {
      const token = useAuthStore.getState().token
      const resp = await fetch(`/api/reports/${reportId}/download`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      })
      if (!resp.ok) throw new Error('报告不存在或已过期')
      const blob = await resp.blob()
      const ext = reportFormat === 'html' ? 'html' : 'md'
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `report-${String(reportId).slice(0, 8)}.${ext}`
      a.click()
      URL.revokeObjectURL(url)
    } catch (e: any) {
      message.error(e?.message || '下载失败')
    }
  }

  const handleRegenerateReport = async (execution: WorkflowExecution) => {
    if (!execution.id) return
    setRegenerating(true)
    try {
      const res = await reportApi.regenerate({ executionId: execution.id })
      message.success(`报告重新生成成功（${res.articleCount} 篇文章）`)
      downloadReport(res.reportId, res.format)
    } catch (e: any) {
      message.error(e?.response?.data?.message || e?.message || '重新生成失败')
    } finally {
      setRegenerating(false)
    }
  }

  const handleViewDetail = async (execution: WorkflowExecution) => {
    setSelectedExecution(execution)
    setDetailModalVisible(true)
    setDetailLoading(true)

    try {
      const logs = await workflowApi.executionLogs(execution.id!)
      setNodeLogs(logs || [])
    } catch (error) {
      message.error('加载执行日志失败')
    } finally {
      setDetailLoading(false)
    }
  }

  const getStatusConfig = (status: string) => {
    const configs: Record<string, { color: string; icon: React.ReactNode; label: string }> = {
      running: { color: 'processing', icon: <SyncOutlined spin />, label: '运行中' },
      success: { color: 'success', icon: <CheckCircleOutlined />, label: '成功' },
      partial_success: { color: 'warning', icon: <ExclamationCircleOutlined />, label: '部分成功' },
      failed: { color: 'error', icon: <CloseCircleOutlined />, label: '失败' },
      cancelled: { color: 'default', icon: <StopOutlined />, label: '已取消' },
    }
    return configs[status] || { color: 'default', icon: null, label: status }
  }

  const handleCancel = async (execId: number) => {
    try {
      await workflowApi.cancelExecution(execId)
      message.success('取消信号已发送')
      fetchExecutions()
    } catch (error) {
      // 错误提示已由 axios 拦截器处理
    }
  }

  const columns: ColumnsType<WorkflowExecution> = [
    {
      title: '执行ID',
      dataIndex: 'id',
      width: 100,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 120,
      render: (status: string) => {
        const config = getStatusConfig(status)
        return (
          <Tag color={config.color} icon={config.icon}>
            {config.label}
          </Tag>
        )
      },
    },
    {
      title: '开始时间',
      dataIndex: 'startedAt',
      width: 180,
      render: (time: string) => time?.replace('T', ' ').slice(0, 19),
    },
    {
      title: '结束时间',
      dataIndex: 'finishedAt',
      width: 180,
      render: (time: string) => time ? time.replace('T', ' ').slice(0, 19) : '-',
    },
    {
      title: '耗时',
      key: 'duration',
      width: 120,
      render: (_, record) => {
        if (!record.finishedAt) return '-'
        const start = new Date(record.startedAt).getTime()
        const end = new Date(record.finishedAt).getTime()
        const duration = Math.round((end - start) / 1000)
        return `${duration}秒`
      },
    },
    {
      title: '错误信息',
      dataIndex: 'errorMsg',
      ellipsis: true,
      render: (msg: string) => msg || '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 180,
      fixed: 'right',
      render: (_, record) => (
        <Space size={4}>
          <Button
            type="link"
            size="small"
            icon={<EyeOutlined />}
            onClick={() => handleViewDetail(record)}
          >
            详情
          </Button>
          {record.status === 'running' && (
            <Popconfirm
              title="确定取消该执行？"
              description="正在执行的节点会在下一个检查点退出"
              onConfirm={() => handleCancel(record.id!)}
              okText="取消执行"
              cancelText="返回"
            >
              <Button type="link" size="small" danger icon={<StopOutlined />}>
                取消
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <PageHeader
        title={`执行历史: ${workflowName}`}
        extra={
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/workflows')}>
            返回
          </Button>
        }
      />

      <div style={{ padding: '24px' }}>
        <Card>
          <Table
            columns={columns}
            dataSource={executions}
            rowKey="id"
            loading={loading}
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
        </Card>
      </div>

      {/* 执行详情弹窗 */}
      <Modal
        title={`执行详情 #${selectedExecution?.id}`}
        open={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        footer={null}
        width={800}
      >
        {detailLoading ? (
          <div style={{ textAlign: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : (
          <div>
            <Card size="small" style={{ marginBottom: 16 }}>
              <Space direction="vertical" style={{ width: '100%' }}>
                <div>
                  <strong>状态：</strong>
                  {selectedExecution && (
                    <Tag color={getStatusConfig(selectedExecution.status).color} icon={getStatusConfig(selectedExecution.status).icon}>
                      {getStatusConfig(selectedExecution.status).label}
                    </Tag>
                  )}
                </div>
                <div>
                  <strong>开始时间：</strong>
                  {selectedExecution?.startedAt?.replace('T', ' ').slice(0, 19)}
                </div>
                <div>
                  <strong>结束时间：</strong>
                  {selectedExecution?.finishedAt ? selectedExecution.finishedAt.replace('T', ' ').slice(0, 19) : '运行中'}
                </div>
                {selectedExecution?.errorMsg && (
                  <div>
                    <strong>错误信息：</strong>
                    <div style={{ color: '#ff4d4f', marginTop: 8, padding: 8, background: '#fff2f0', borderRadius: 4 }}>
                      {selectedExecution.errorMsg}
                    </div>
                  </div>
                )}
              </Space>
            </Card>

            {/* 分析报告下载区：若本次执行生成了报告，在此醒目展示 */}
            {nodeLogs.filter(l => l.output?.reportId).map(l => (
              <Alert
                key={l.output!.reportId}
                style={{ marginBottom: 12 }}
                icon={<FileTextOutlined />}
                showIcon
                type="success"
                message={`AI 分析报告已生成（${l.output!.reportFormat || 'markdown'} 格式）`}
                description={`节点：${l.nodeId}`}
                action={
                  <Space direction="vertical" size={4}>
                    <Button
                      type="primary"
                      size="small"
                      icon={<DownloadOutlined />}
                      onClick={() => downloadReport(l.output!.reportId, l.output!.reportFormat)}
                    >
                      下载报告
                    </Button>
                    <Button
                      size="small"
                      icon={<ReloadOutlined />}
                      loading={regenerating}
                      onClick={() => handleRegenerateReport(selectedExecution!)}
                    >
                      重新生成
                    </Button>
                  </Space>
                }
              />
            ))}

            <Card size="small" title="节点执行日志">
              {nodeLogs.length === 0 ? (
                <div style={{ textAlign: 'center', padding: '20px', color: '#999' }}>
                  暂无节点执行日志
                </div>
              ) : (
                <Timeline
                  items={nodeLogs.map((log) => {
                    const statusConfig = getStatusConfig(log.status)
                    return {
                      color: statusConfig.color === 'processing' ? 'blue' : statusConfig.color === 'success' ? 'green' : 'red',
                      dot: statusConfig.icon,
                      children: (
                        <div>
                          <div style={{ marginBottom: 8 }}>
                            <strong>{log.nodeId}</strong>
                            <Tag color={statusConfig.color} style={{ marginLeft: 8 }}>
                              {statusConfig.label}
                            </Tag>
                          </div>
                          <div style={{ fontSize: 12, color: '#666' }}>
                            开始: {log.startedAt?.replace('T', ' ').slice(0, 19)}
                          </div>
                          {log.finishedAt && (
                            <div style={{ fontSize: 12, color: '#666' }}>
                              结束: {log.finishedAt.replace('T', ' ').slice(0, 19)}
                            </div>
                          )}
                          {log.input && Object.keys(log.input).length > 0 && (
                            <div style={{ marginTop: 8 }}>
                              <div style={{ fontSize: 12, color: '#666' }}>输入:</div>
                              <pre style={{ fontSize: 11, background: '#f5f5f5', padding: 8, borderRadius: 4, marginTop: 4 }}>
                                {JSON.stringify(log.input, null, 2)}
                              </pre>
                            </div>
                          )}
                          {log.output && Object.keys(log.output).length > 0 && (
                            <div style={{ marginTop: 8 }}>
                              <div style={{ fontSize: 12, color: '#666' }}>输出:</div>
                              <pre style={{ fontSize: 11, background: '#f5f5f5', padding: 8, borderRadius: 4, marginTop: 4 }}>
                                {JSON.stringify(log.output, null, 2)}
                              </pre>
                              {log.output.reportId && (
                                <Button
                                  type="primary"
                                  size="small"
                                  icon={<DownloadOutlined />}
                                  style={{ marginTop: 8 }}
                                  onClick={() => downloadReport(log.output!.reportId, log.output!.reportFormat)}
                                >
                                  下载分析报告（{log.output.reportFormat || 'markdown'}）
                                </Button>
                              )}
                            </div>
                          )}
                          {log.errorMsg && (
                            <div style={{ marginTop: 8, color: '#ff4d4f', fontSize: 12 }}>
                              错误: {log.errorMsg}
                            </div>
                          )}
                        </div>
                      ),
                    }
                  })}
                />
              )}
            </Card>
          </div>
        )}
      </Modal>
    </div>
  )
}

export default WorkflowExecutionPage

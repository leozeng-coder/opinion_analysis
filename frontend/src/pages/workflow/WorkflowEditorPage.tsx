import React, { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import ReactFlow, {
  Node,
  Controls,
  Background,
  useNodesState,
  useEdgesState,
  addEdge,
  Connection,
  MarkerType,
  Panel,
  Handle,
  Position,
} from 'reactflow'
import 'reactflow/dist/style.css'
import { Form, Input, Button, Card, Space, message, Switch, Modal, InputNumber, Select, Drawer, Tag } from 'antd'
import { SaveOutlined, ArrowLeftOutlined, PlusOutlined } from '@ant-design/icons'
import PageHeader from '@/components/common/PageHeader'
import { workflowApi } from '@/api/workflow'
import { Workflow } from '@/types'

const { TextArea } = Input

// 节点类型注册表
export const NODE_REGISTRY = {
  ai_tagger: {
    label: 'AI 打标',
    description: '自动为文章添加 AI 标签',
    color: '#1890ff',
    icon: '🤖',
    configSchema: [
      { name: 'batchSize', label: '批次大小', type: 'number', required: true, default: 20, min: 1, max: 100 },
      { name: 'onlyProvidedIds', label: '仅处理上游 articleIds', type: 'boolean', required: false, default: true },
    ],
  },
  alert_evaluate: {
    label: '告警评估',
    description: '评估所有告警规则',
    color: '#ff4d4f',
    icon: '⚠️',
    configSchema: [],
  },
  rag_vectorize: {
    label: 'RAG 向量化',
    description: '触发 RAG 服务对新文章进行向量化',
    color: '#52c41a',
    icon: '📊',
    configSchema: [
      { name: 'onlyProvidedIds', label: '仅处理上游 articleIds', type: 'boolean', required: false, default: true },
      { name: 'waitForCompletion', label: '等待向量化完成', type: 'boolean', required: false, default: true },
      { name: 'timeoutMinutes', label: '超时时间(分钟)', type: 'number', required: false, default: 5, min: 1, max: 60 },
    ],
  },
  condition: {
    label: '条件判断',
    description: '根据条件决定执行路径',
    color: '#faad14',
    icon: '🔀',
    configSchema: [
      { name: 'expression', label: '条件表达式', type: 'text', required: true, placeholder: 'input.taggedCount > 10' },
    ],
  },
  delay: {
    label: '延迟',
    description: '延迟指定秒数',
    color: '#722ed1',
    icon: '⏱️',
    configSchema: [
      { name: 'seconds', label: '延迟秒数', type: 'number', required: true, default: 60, min: 1, max: 3600 },
    ],
  },
  platform_sync: {
    label: '平台数据同步',
    description: '将 MediaCrawler 平台表同步到 articles 中心表',
    color: '#597ef7',
    icon: '🔄',
    configSchema: [
      {
        name: 'platforms',
        label: '平台',
        type: 'select-multiple',
        required: false,
        options: [
          { label: '小红书', value: 'xiaohongshu' },
          { label: '微博', value: 'weibo' },
          { label: '抖音', value: 'douyin' },
          { label: '快手', value: 'kuaishou' },
          { label: 'B站', value: 'bilibili' },
          { label: '百度贴吧', value: 'tieba' },
          { label: '知乎', value: 'zhihu' },
        ],
        placeholder: '留空则继承上游爬虫节点的平台'
      },
      {
        name: 'syncMode',
        label: '同步模式',
        type: 'select',
        required: false,
        default: 'incremental',
        options: [
          { label: '增量同步', value: 'incremental' },
          { label: '全量同步', value: 'full' },
        ],
      },
      { name: 'syncSinceMinutes', label: '回溯分钟数', type: 'number', required: false, default: 0, min: 0, max: 10080, placeholder: '无上游爬虫时按最近 N 分钟同步，0 表示用系统记录' },
      { name: 'enableSentiment', label: '同步时情感分析', type: 'boolean', required: false, default: false },
    ],
  },
  crawler_run: {
    label: '执行爬虫',
    description: '触发爬虫任务抓取数据',
    color: '#13c2c2',
    icon: '🕷️',
    configSchema: [
      {
        name: 'platforms',
        label: '平台',
        type: 'select-multiple',
        required: false,
        options: [
          { label: '小红书', value: 'xiaohongshu' },
          { label: '微博', value: 'weibo' },
          { label: '抖音', value: 'douyin' },
          { label: '快手', value: 'kuaishou' },
          { label: 'B站', value: 'bilibili' },
          { label: '百度贴吧', value: 'tieba' },
          { label: '知乎', value: 'zhihu' },
        ],
        placeholder: '选择要爬取的平台'
      },
      { name: 'keywords', label: '关键词', type: 'tags', required: false, placeholder: '输入关键词后按回车添加' },
      { name: 'topics', label: '话题', type: 'tags', required: false, placeholder: '输入话题后按回车添加' },
      { name: 'waitForCompletion', label: '等待完成', type: 'boolean', required: false, default: true },
      { name: 'timeoutMinutes', label: '超时时间(分钟)', type: 'number', required: false, default: 10, min: 1, max: 60 },
    ],
  },
  crawler_schedule: {
    label: '配置爬虫调度',
    description: '更新爬虫定时任务配置',
    color: '#2f54eb',
    icon: '⏰',
    configSchema: [
      { name: 'spiderKey', label: '爬虫标识', type: 'text', required: true, placeholder: 'broad-topic 或 deep-sentiment' },
      { name: 'intervalMinutes', label: '执行间隔(分钟)', type: 'number', required: true, default: 60, min: 1, max: 10080 },
      { name: 'enabled', label: '启用', type: 'boolean', required: true },
    ],
  },
  crawler_status: {
    label: '检查爬虫状态',
    description: '查询爬虫运行状态',
    color: '#eb2f96',
    icon: '📈',
    configSchema: [
      { name: 'runID', label: '运行ID', type: 'number', required: false, placeholder: '留空则检查最近运行' },
      { name: 'checkRecent', label: '检查最近运行', type: 'boolean', required: false },
    ],
  },
}

// 自定义节点组件
const CustomNode = ({ data }: any) => {
  const nodeType = NODE_REGISTRY[data.type as keyof typeof NODE_REGISTRY]
  return (
    <div
      style={{
        padding: '12px 16px',
        borderRadius: 8,
        background: '#fff',
        border: `2px solid ${nodeType?.color || '#d9d9d9'}`,
        minWidth: 180,
        cursor: 'pointer',
      }}
    >
      {/* 输入连接点 */}
      <Handle type="target" position={Position.Left} style={{ background: nodeType?.color }} />

      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
        <span style={{ fontSize: 20 }}>{nodeType?.icon}</span>
        <strong style={{ fontSize: 14 }}>{nodeType?.label || data.type}</strong>
      </div>
      <div style={{ fontSize: 12, color: '#666' }}>{data.label}</div>
      {Object.keys(data.config || {}).length > 0 && (
        <div style={{ fontSize: 11, color: '#999', marginTop: 4 }}>
          {JSON.stringify(data.config)}
        </div>
      )}

      {/* 输出连接点 */}
      <Handle type="source" position={Position.Right} style={{ background: nodeType?.color }} />
    </div>
  )
}

const nodeTypes = {
  custom: CustomNode,
}

const WorkflowEditorPage: React.FC = () => {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const isEdit = !!id
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  // React Flow 状态
  const [nodes, setNodes, onNodesChange] = useNodesState([])
  const [edges, setEdges, onEdgesChange] = useEdgesState([])

  // 节点配置抽屉
  const [drawerVisible, setDrawerVisible] = useState(false)
  const [selectedNode, setSelectedNode] = useState<Node | null>(null)
  const [nodeConfigForm] = Form.useForm()

  // 节点类型选择器
  const [nodeTypeModalVisible, setNodeTypeModalVisible] = useState(false)

  useEffect(() => {
    if (isEdit) {
      loadWorkflow()
    }
  }, [id])

  const loadWorkflow = async () => {
    setLoading(true)
    try {
      const workflow = await workflowApi.detail(Number(id))
      form.setFieldsValue({
        name: workflow.name,
        description: workflow.description,
        status: workflow.status === 1,
        triggerType: workflow.triggerType,
        triggerConfig: JSON.stringify(workflow.triggerConfig, null, 2),
      })

      // 转换为 React Flow 格式
      const flowNodes = (workflow.nodes || []).map((node: any, index: number) => ({
        id: node.id,
        type: 'custom',
        position: { x: 100 + index * 250, y: 100 },
        data: {
          label: node.id,
          type: node.type,
          config: node.config,
        },
      }))

      const flowEdges = (workflow.edges || []).map((edge: any, index: number) => ({
        id: `edge-${index}`,
        source: edge.source,
        target: edge.target,
        type: 'smoothstep',
        animated: true,
        markerEnd: { type: MarkerType.ArrowClosed },
      }))

      setNodes(flowNodes)
      setEdges(flowEdges)
    } catch (error) {
      message.error('加载工作流失败')
    } finally {
      setLoading(false)
    }
  }

  const onConnect = useCallback(
    (params: Connection) => {
      setEdges((eds) =>
        addEdge(
          {
            ...params,
            type: 'smoothstep',
            animated: true,
            markerEnd: { type: MarkerType.ArrowClosed },
          },
          eds
        )
      )
    },
    [setEdges]
  )

  const handleAddNode = (nodeType: string) => {
    const nodeId = `node_${Date.now()}`
    const nodeConfig = NODE_REGISTRY[nodeType as keyof typeof NODE_REGISTRY]

    const newNode: Node = {
      id: nodeId,
      type: 'custom',
      position: { x: Math.random() * 400 + 100, y: Math.random() * 300 + 100 },
      data: {
        label: nodeId,
        type: nodeType,
        config: {},
      },
    }

    setNodes((nds) => [...nds, newNode])
    setNodeTypeModalVisible(false)
    message.success(`已添加 ${nodeConfig.label} 节点`)
  }

  const handleNodeClick = useCallback((_event: React.MouseEvent, node: Node) => {
    setSelectedNode(node)
    nodeConfigForm.setFieldsValue({
      label: node.data.label,
      ...node.data.config,
    })
    setDrawerVisible(true)
  }, [])

  const handleNodeConfigSave = async () => {
    try {
      const values = await nodeConfigForm.validateFields()
      const { label, ...config } = values

      setNodes((nds) =>
        nds.map((node) => {
          if (node.id === selectedNode?.id) {
            return {
              ...node,
              data: {
                ...node.data,
                label,
                config,
              },
            }
          }
          return node
        })
      )

      setDrawerVisible(false)
      message.success('节点配置已更新')
    } catch (error) {
      // 验证失败
    }
  }

  const handleDeleteNode = () => {
    if (!selectedNode) return
    setNodes((nds) => nds.filter((node) => node.id !== selectedNode.id))
    setEdges((eds) => eds.filter((edge) => edge.source !== selectedNode.id && edge.target !== selectedNode.id))
    setDrawerVisible(false)
    message.success('节点已删除')
  }

  const handleSubmit = async (values: any) => {
    if (nodes.length === 0) {
      message.error('请至少添加一个节点')
      return
    }

    setSaving(true)
    try {
      let triggerConfig = {}
      try {
        triggerConfig = JSON.parse(values.triggerConfig || '{}')
      } catch {
        message.error('触发配置 JSON 格式错误')
        setSaving(false)
        return
      }

      // 转换为后端格式
      const workflowNodes = nodes.map((node) => ({
        id: node.id,
        type: node.data.type,
        label: node.data.label,
        position: node.position,
        config: node.data.config || {},
      }))

      const workflowEdges = edges.map((edge) => ({
        id: edge.id,
        source: edge.source,
        target: edge.target,
      }))

      const payload: Partial<Workflow> = {
        name: values.name,
        description: values.description,
        status: values.status ? 1 : 0,
        triggerType: values.triggerType,
        triggerConfig,
        nodes: workflowNodes,
        edges: workflowEdges,
      }

      if (isEdit) {
        await workflowApi.update(Number(id), payload)
        message.success('更新成功')
      } else {
        await workflowApi.create(payload)
        message.success('创建成功')
      }
      navigate('/workflows')
    } catch (error: any) {
      message.error(error.message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const currentNodeType = selectedNode ? NODE_REGISTRY[selectedNode.data.type as keyof typeof NODE_REGISTRY] : null

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column' }}>
      <PageHeader
        title={isEdit ? '编辑工作流' : '新建工作流'}
        extra={
          <Space>
            <Button icon={<PlusOutlined />} onClick={() => setNodeTypeModalVisible(true)}>
              添加节点
            </Button>
            <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={() => form.submit()}>
              保存
            </Button>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/workflows')}>
              返回
            </Button>
          </Space>
        }
      />

      <div style={{ flex: 1, display: 'flex', padding: '16px', gap: 16 }}>
        {/* 左侧配置面板 */}
        <Card style={{ width: 320, overflow: 'auto' }} loading={loading}>
          <Form
            form={form}
            layout="vertical"
            onFinish={handleSubmit}
            initialValues={{
              status: true,
              triggerType: 'manual',
              triggerConfig: '{}',
            }}
          >
            <Form.Item
              label="工作流名称"
              name="name"
              rules={[{ required: true, message: '请输入工作流名称' }]}
            >
              <Input placeholder="例如：每日舆情分析" />
            </Form.Item>

            <Form.Item label="描述" name="description">
              <TextArea rows={2} placeholder="工作流用途说明" />
            </Form.Item>

            <Form.Item
              label="触发类型"
              name="triggerType"
              rules={[{ required: true, message: '请选择触发类型' }]}
            >
              <Select>
                <Select.Option value="manual">手动触发</Select.Option>
                <Select.Option value="schedule">定时触发</Select.Option>
                <Select.Option value="webhook">Webhook</Select.Option>
              </Select>
            </Form.Item>

            <Form.Item
              label="触发配置"
              name="triggerConfig"
              rules={[
                { required: true },
                {
                  validator: (_, value) => {
                    try {
                      JSON.parse(value)
                      return Promise.resolve()
                    } catch {
                      return Promise.reject(new Error('JSON 格式错误'))
                    }
                  },
                },
              ]}
            >
              <TextArea rows={3} placeholder='{"cron": "0 2 * * *"}' />
            </Form.Item>

            <Form.Item label="状态" name="status" valuePropName="checked">
              <Switch checkedChildren="启用" unCheckedChildren="禁用" />
            </Form.Item>
          </Form>
        </Card>

        {/* 右侧画布 */}
        <Card style={{ flex: 1 }} bodyStyle={{ padding: 0, height: '100%' }}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onNodeClick={handleNodeClick}
            nodeTypes={nodeTypes}
            fitView
          >
            <Background />
            <Controls />
            <Panel position="top-left">
              <div style={{ background: '#fff', padding: 8, borderRadius: 4, fontSize: 12 }}>
                💡 拖拽节点调整位置，连接节点创建流程
              </div>
            </Panel>
          </ReactFlow>
        </Card>
      </div>

      {/* 节点类型选择弹窗 */}
      <Modal
        title="选择节点类型"
        open={nodeTypeModalVisible}
        onCancel={() => setNodeTypeModalVisible(false)}
        footer={null}
        width={600}
      >
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 12 }}>
          {Object.entries(NODE_REGISTRY).map(([key, config]) => (
            <Card
              key={key}
              hoverable
              onClick={() => handleAddNode(key)}
              style={{ cursor: 'pointer' }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                <span style={{ fontSize: 24 }}>{config.icon}</span>
                <strong>{config.label}</strong>
              </div>
              <div style={{ fontSize: 12, color: '#666' }}>{config.description}</div>
            </Card>
          ))}
        </div>
      </Modal>

      {/* 节点配置抽屉 */}
      <Drawer
        title={`配置节点: ${currentNodeType?.label}`}
        placement="right"
        onClose={() => setDrawerVisible(false)}
        open={drawerVisible}
        width={400}
        extra={
          <Space>
            <Button danger onClick={handleDeleteNode}>
              删除节点
            </Button>
            <Button type="primary" onClick={handleNodeConfigSave}>
              保存
            </Button>
          </Space>
        }
      >
        {selectedNode && (
          <Form form={nodeConfigForm} layout="vertical">
            <Form.Item label="节点 ID">
              <Input value={selectedNode.id} disabled />
            </Form.Item>

            <Form.Item
              label="节点标签"
              name="label"
              rules={[{ required: true, message: '请输入节点标签' }]}
            >
              <Input placeholder="节点显示名称" />
            </Form.Item>

            {currentNodeType?.configSchema.map((field) => {
              const fieldConfig = field as any
              const isNumberField = field.type === 'number'
              const isBooleanField = field.type === 'boolean'
              const isTagsField = field.type === 'tags'
              const isSelectMultiple = field.type === 'select-multiple'
              const isSelect = field.type === 'select'

              return (
                <Form.Item
                  key={field.name}
                  label={field.label}
                  name={field.name}
                  rules={[{ required: field.required, message: `请输入${field.label}` }]}
                  valuePropName={isBooleanField ? 'checked' : 'value'}
                >
                  {isNumberField ? (
                    <InputNumber
                      min={fieldConfig.min}
                      max={fieldConfig.max}
                      placeholder={fieldConfig.placeholder}
                      style={{ width: '100%' }}
                    />
                  ) : isBooleanField ? (
                    <Switch />
                  ) : isTagsField ? (
                    <Select
                      mode="tags"
                      style={{ width: '100%' }}
                      placeholder={fieldConfig.placeholder}
                      tokenSeparators={[',']}
                    />
                  ) : isSelectMultiple ? (
                    <Select
                      mode="multiple"
                      style={{ width: '100%' }}
                      placeholder={fieldConfig.placeholder}
                      options={fieldConfig.options}
                    />
                  ) : isSelect ? (
                    <Select
                      style={{ width: '100%' }}
                      placeholder={fieldConfig.placeholder}
                      options={fieldConfig.options}
                    />
                  ) : (
                    <Input placeholder={fieldConfig.placeholder} />
                  )}
                </Form.Item>
              )
            })}
          </Form>
        )}
      </Drawer>
    </div>
  )
}

export default WorkflowEditorPage

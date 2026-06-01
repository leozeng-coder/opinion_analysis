import React, { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import {
  DatabaseOutlined,
  ReloadOutlined,
  RobotOutlined,
  SaveOutlined,
} from '@ant-design/icons'
import { adminRagApi } from '@/api/admin-rag'
import { adminSystemApi } from '@/api/admin-system'
import type {
  RagStatus,
  SystemConfigResponse,
  ConfigSnapshot,
  RagSnapshotConfig,
  TaggerSnapshotConfig,
  TaggerConfig,
  UpdateTaggerPayload,
  UpdateRagConfigPayload,
} from '@/types'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'
import dayjs from 'dayjs'

const { Text } = Typography

const PRESETS = [
  { label: 'DeepSeek', baseUrl: 'https://api.deepseek.com', model: 'deepseek-chat' },
  { label: 'OpenAI', baseUrl: 'https://api.openai.com', model: 'gpt-4o' },
  { label: '百炼/Qwen', baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1', model: 'qwen-plus' },
  { label: 'Kimi', baseUrl: 'https://api.moonshot.cn/v1', model: 'moonshot-v1-8k' },
  { label: '智谱/GLM', baseUrl: 'https://open.bigmodel.cn/api/paas/v4', model: 'glm-4-flash' },
]

const EMBED_API_PRESETS = [
  { label: 'OpenAI', baseUrl: 'https://api.openai.com/v1', model: 'text-embedding-3-small' },
  { label: 'Jina', baseUrl: 'https://api.jina.ai/v1', model: 'jina-embeddings-v3' },
  { label: '百炼/Qwen', baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1', model: 'text-embedding-v3' },
  { label: 'DeepSeek', baseUrl: 'https://api.deepseek.com/v1', model: 'deepseek-embedding' },
]

const EMBED_PRESETS = [
  { label: '多语言 MiniLM（默认）', model: 'paraphrase-multilingual-MiniLM-L12-v2' },
  { label: 'BGE 中文 small', model: 'BAAI/bge-small-zh-v1.5' },
  { label: 'BGE 中文 base', model: 'BAAI/bge-base-zh-v1.5' },
  { label: 'M3E base', model: 'moka-ai/m3e-base' },
]

interface TaggerFormValues {
  enabled: boolean
  llmModel: string
  llmBaseUrl: string
  llmApiKey: string
  intervalSeconds: number
  batchSize: number
  maxPerTick: number
}

interface RagFormValues {
  embed_provider: 'local' | 'api'
  embed_model: string
  embed_api_base: string
  embed_api_key: string
  chunk_max_chars: number
  chunk_overlap: number
  sync_enabled: boolean
  sync_interval_sec: number
  sync_batch: number
}

const AIConfigPage: React.FC = () => {
  const [cfg, setCfg] = useState<SystemConfigResponse | null>(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<TaggerFormValues>()

  const [taggerHistory, setTaggerHistory] = useState<ConfigSnapshot[]>([])
  const [taggerHistTotal, setTaggerHistTotal] = useState(0)
  const [taggerHistPage, setTaggerHistPage] = useState(1)

  const [ragForm] = Form.useForm<RagFormValues>()
  const ragProvider = Form.useWatch('embed_provider', ragForm) ?? 'local'
  const ragSyncEnabledWatch = Form.useWatch('sync_enabled', ragForm) ?? true
  const [ragSaving, setRagSaving] = useState(false)
  const [ragEnvLocks, setRagEnvLocks] = useState<string[]>([])
  const [ragApiKeySet, setRagApiKeySet] = useState(false)
  const [ragStatus, setRagStatus] = useState<RagStatus | null>(null)
  const [rebuildingMilvus, setRebuildingMilvus] = useState(false)
  const [restartingRag, setRestartingRag] = useState(false)

  const [ragHistory, setRagHistory] = useState<ConfigSnapshot[]>([])
  const [ragHistTotal, setRagHistTotal] = useState(0)
  const [ragHistPage, setRagHistPage] = useState(1)

  const [loading, setLoading] = useState(false)

  const applyTaggerToForm = useCallback(
    (t: TaggerConfig) => {
      form.setFieldsValue({
        enabled: t.enabled,
        llmModel: t.llmModel,
        llmBaseUrl: t.llmBaseUrl,
        llmApiKey: '',
        intervalSeconds: t.intervalSeconds,
        batchSize: t.batchSize,
        maxPerTick: t.maxPerTick,
      })
    },
    [form]
  )

  const loadRagHistory = useCallback(async (page: number) => {
    const r = await adminSystemApi.settingHistory({ domain: 'rag', page, pageSize: 8 })
    setRagHistory(r.list)
    setRagHistTotal(r.total)
    setRagHistPage(page)
  }, [])

  const loadTaggerHistory = useCallback(async (page: number) => {
    const r = await adminSystemApi.settingHistory({ domain: 'tagger', page, pageSize: 8 })
    setTaggerHistory(r.list)
    setTaggerHistTotal(r.total)
    setTaggerHistPage(page)
  }, [])

  const refreshRagConfigForm = useCallback(async () => {
    const cfg2 = await adminRagApi.getConfig().catch(() => null)
    if (cfg2 != null) {
      setRagEnvLocks(cfg2.env_overrides ?? [])
      setRagApiKeySet(cfg2.api_key_set ?? false)
      ragForm.setFieldsValue({
        embed_provider: (cfg2.embed_provider === 'api' ? 'api' : 'local') as 'local' | 'api',
        embed_model: cfg2.embed_model,
        embed_api_base: cfg2.embed_api_base ?? '',
        embed_api_key: '',
        chunk_max_chars: cfg2.chunk_max_chars,
        chunk_overlap: cfg2.chunk_overlap,
        sync_enabled: cfg2.sync_enabled ?? true,
        sync_interval_sec: cfg2.sync_interval_sec,
        sync_batch: cfg2.sync_batch,
      })
    }
  }, [ragForm])

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      const [c, rs] = await Promise.all([
        adminSystemApi.config(),
        adminRagApi.status().catch(() => null),
      ])
      setCfg(c)
      if (c?.tagger) applyTaggerToForm(c.tagger)
      setRagStatus(rs as RagStatus | null)
      await refreshRagConfigForm()
      await Promise.all([loadRagHistory(1), loadTaggerHistory(1)])
    } finally {
      setLoading(false)
    }
  }, [applyTaggerToForm, loadRagHistory, loadTaggerHistory, refreshRagConfigForm])

  useEffect(() => {
    void fetchAll()
  }, [fetchAll])

  const handleDeleteHistory = async (id: number, reload: () => Promise<void>) => {
    try {
      await adminSystemApi.deleteSettingHistory(id)
      message.success('已删除历史记录')
      await reload()
    } catch (e) {
      console.error(e)
    }
  }

  const handleReapplyHistory = async (id: number, kind: 'rag' | 'tagger') => {
    try {
      const resp = await adminSystemApi.reapplySettingHistory(id)
      if (resp.warning) {
        message.warning(resp.message)
      } else {
        message.success(resp.message)
      }
      if (kind === 'rag') {
        await Promise.all([loadRagHistory(ragHistPage), refreshRagConfigForm()])
        void adminRagApi.status().then(setRagStatus).catch(() => undefined)
      } else {
        await loadTaggerHistory(taggerHistPage)
        const c = await adminSystemApi.config()
        setCfg(c)
        if (c?.tagger) applyTaggerToForm(c.tagger)
      }
    } catch (e) {
      console.error(e)
    }
  }

  const historyActionColumn = (kind: 'rag' | 'tagger', reload: () => Promise<void>) => ({
    title: '操作',
    key: 'actions',
    width: 100,
    fixed: 'right' as const,
    render: (_: unknown, row: ConfigSnapshot) => (
      <Space size={4} wrap>
        <Button type="link" size="small" onClick={() => void handleReapplyHistory(row.id, kind)}>
          应用
        </Button>
        <Popconfirm
          title="确定删除这条历史记录？"
          onConfirm={() => void handleDeleteHistory(row.id, reload)}
        >
          <Button type="link" size="small" danger>
            删除
          </Button>
        </Popconfirm>
      </Space>
    ),
  })

  const handleSaveTagger = async (values: TaggerFormValues) => {
    const payload: UpdateTaggerPayload = {
      enabled: values.enabled,
      llmModel: values.llmModel,
      llmBaseUrl: values.llmBaseUrl,
      intervalSeconds: values.intervalSeconds,
      batchSize: values.batchSize,
      maxPerTick: values.maxPerTick,
    }
    const trimmed = (values.llmApiKey ?? '').trim()
    if (trimmed) payload.llmApiKey = trimmed
    setSaving(true)
    try {
      const resp = await adminSystemApi.updateTagger(payload)
      setCfg(resp)
      if (resp?.tagger) applyTaggerToForm(resp.tagger)
      message.success('已保存，后台任务下一轮 tick 生效')
      void loadTaggerHistory(1)
    } catch (e) {
      console.error(e)
    } finally {
      setSaving(false)
    }
  }

  const ragFieldLocked = (dbKey: string) => ragEnvLocks.includes(dbKey)

  const handleSaveRagEmbed = async (values: RagFormValues) => {
    setRagSaving(true)
    try {
      const payload: UpdateRagConfigPayload = {
        embed_provider: values.embed_provider,
        embed_model: values.embed_model.trim(),
        chunk_max_chars: values.chunk_max_chars,
        chunk_overlap: values.chunk_overlap,
        sync_enabled: values.sync_enabled,
        sync_interval_sec: values.sync_interval_sec,
        sync_batch: values.sync_batch,
      }
      if (values.embed_provider === 'api') {
        payload.embed_api_base = (values.embed_api_base ?? '').trim()
      }
      const keyTrimmed = (values.embed_api_key ?? '').trim()
      if (keyTrimmed) payload.embed_api_key = keyTrimmed

      const resp = await adminRagApi.updateConfig(payload)
      setRagEnvLocks(resp.env_overrides ?? [])
      setRagApiKeySet(resp.api_key_set ?? ragApiKeySet)
      ragForm.setFieldValue('embed_api_key', '')
      if (resp.warnings?.length) {
        message.warning(resp.warnings.join('；'))
      } else if (resp.warning) {
        message.warning(resp.warning)
      } else if (resp.service_applied === false) {
        message.success('配置已保存到数据库，请重启 RAG 服务后生效')
      } else {
        message.success('Embedding 配置已保存（换模型后建议立即同步）')
      }
      void adminRagApi.status().then(setRagStatus).catch(() => undefined)
      void loadRagHistory(1)
    } catch (e) {
      console.error(e)
      message.error('保存失败')
    } finally {
      setRagSaving(false)
    }
  }

  const handleRagRestart = () => {
    Modal.confirm({
      title: '重启 RAG 服务',
      content: '将停止占用端口的旧 RAG 进程并重新拉起（加载最新代码与配置）。本地模型首次加载可能需 1～2 分钟，是否继续？',
      okText: '重启',
      cancelText: '取消',
      onOk: async () => {
        setRestartingRag(true)
        try {
          const result = await adminRagApi.restartService()
          if (result.healthReady) {
            message.success(result.message || `RAG 已就绪（PID ${result.pid ?? '-'})`)
          } else {
            message.loading(result.message || 'RAG 启动中…', 0)
          }
          const deadline = Date.now() + 180_000
          while (Date.now() < deadline) {
            await new Promise((r) => setTimeout(r, 2000))
            const rs = await adminRagApi.status().catch(() => null)
            if (rs) setRagStatus(rs)
            if (rs?.serviceReachable) {
              message.destroy()
              message.success(
                rs.embedderReady === false
                  ? `RAG HTTP 已就绪（PID ${result.pid ?? '-'}），嵌入模型仍在加载`
                  : `RAG 服务已就绪（PID ${result.pid ?? rs.processPid ?? '-'})`,
              )
              return
            }
          }
          message.destroy()
          message.warning('RAG 进程已提交启动，但等待就绪超时；请查看 rag/logs/rag_service_managed.log')
        } catch (e: unknown) {
          message.destroy()
          const err = e as { response?: { data?: { message?: string } }; message?: string }
          message.error(err.response?.data?.message || err.message || '重启失败')
        } finally {
          setRestartingRag(false)
        }
      },
    })
  }

  const handleRagRebuildAndSync = () => {
    Modal.confirm({
      title: '重建 Milvus 向量库',
      content:
        '将删除当前 Milvus 集合中的全部向量，并按当前嵌入模型维度重建；同时清空文章的同步标记以便全量重算。此操作不可撤销，是否继续？',
      okText: '重建并同步',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        setRebuildingMilvus(true)
        try {
          const result = await adminRagApi.rebuildMilvus()
          message.success(
            `向量库已重建（${result.collection_dimension} 维），已重置 ${result.articles_reset_for_resync} 篇文章的同步标记`,
          )
          void adminRagApi.status().then(setRagStatus).catch(() => undefined)
          await adminRagApi.triggerSync()
          message.success('已提交全量向量同步，可前往「任务管理」查看进度')
        } catch (e: unknown) {
          console.error(e)
          const err = e as { response?: { data?: { message?: string } }; message?: string }
          message.error(err.response?.data?.message || err.message || '重建失败')
        } finally {
          setRebuildingMilvus(false)
        }
      },
    })
  }

  const ragSnapshotColumns = [
    {
      title: '嵌入来源',
      width: 88,
      render: (_: unknown, row: ConfigSnapshot) =>
        (row.config as RagSnapshotConfig).embed_provider === 'api' ? (
          <Tag color="purple">API</Tag>
        ) : (
          <Tag color="blue">本地</Tag>
        ),
    },
    {
      title: '模型',
      width: 160,
      ellipsis: true,
      render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).embed_model || '-',
    },
    {
      title: 'API URL',
      width: 200,
      ellipsis: true,
      render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).embed_api_base || '-',
    },
    {
      title: 'API Key',
      width: 120,
      ellipsis: true,
      render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).embed_api_key || '-',
    },
    { title: '切块', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).chunk_max_chars },
    { title: '重叠', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).chunk_overlap },
    {
      title: '同步间隔',
      width: 88,
      render: (_: unknown, row: ConfigSnapshot) => `${(row.config as RagSnapshotConfig).sync_interval_sec}s`,
    },
    { title: '批量', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).sync_batch },
    {
      title: '定时同步',
      width: 88,
      render: (_: unknown, row: ConfigSnapshot) =>
        (row.config as RagSnapshotConfig).sync_enabled ? <Tag color="success">开</Tag> : <Tag>关</Tag>,
    },
    { title: '操作者', width: 88, dataIndex: 'updatedByName', render: (v: string) => v || '-' },
    { title: '时间', width: 128, dataIndex: 'createdAt', render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss') },
    historyActionColumn('rag', () => loadRagHistory(ragHistPage)),
  ]

  const taggerSnapshotColumns = [
    {
      title: '启用',
      width: 64,
      render: (_: unknown, row: ConfigSnapshot) =>
        (row.config as TaggerSnapshotConfig).enabled ? <Tag color="success">是</Tag> : <Tag>否</Tag>,
    },
    {
      title: '模型',
      width: 140,
      ellipsis: true,
      render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).llm_model || '-',
    },
    {
      title: 'API URL',
      width: 200,
      ellipsis: true,
      render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).llm_base_url || '-',
    },
    {
      title: 'API Key',
      width: 120,
      ellipsis: true,
      render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).llm_api_key || '-',
    },
    {
      title: '轮询间隔',
      width: 88,
      render: (_: unknown, row: ConfigSnapshot) => `${(row.config as TaggerSnapshotConfig).interval_seconds}s`,
    },
    { title: '批次', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).batch_size },
    { title: '上限', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as TaggerSnapshotConfig).max_per_tick },
    { title: '操作者', width: 88, dataIndex: 'updatedByName', render: (v: string) => v || '-' },
    { title: '时间', width: 128, dataIndex: 'createdAt', render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss') },
    historyActionColumn('tagger', () => loadTaggerHistory(taggerHistPage)),
  ]

  const tagger = cfg?.tagger
  const apiKeyHint = tagger?.apiKeySet
    ? `已配置（${tagger.llmApiKey || '***'}），留空则保留`
    : '尚未配置，必须填写后台任务才能运行'
  const ragApiKeyHint = ragApiKeySet ? '已配置，留空则保留当前值' : '使用第三方 API 时必须填写'

  return (
    <div className={ui.pageShell}>
      <PageHeader
        title="AI 配置"
        subtitle="RAG 向量库、AI 打标、Embedding 模型配置"
        icon={<RobotOutlined />}
      />

      <Card
        bordered={false}
        className={ui.panelCard}
        title={
          <span>
            <DatabaseOutlined style={{ marginRight: 8 }} />
            Embedding 配置
          </span>
        }
        extra={
          <Space>
            <Space size={4}>
              <Text style={{ fontSize: 13 }}>定时同步</Text>
              <Switch
                size="small"
                checked={ragSyncEnabledWatch}
                checkedChildren="开"
                unCheckedChildren="关"
                onChange={(v) => ragForm.setFieldValue('sync_enabled', v)}
              />
            </Space>
            {ragStatus?.processManaged && (
              <Button
                icon={<ReloadOutlined />}
                loading={restartingRag}
                onClick={() => handleRagRestart()}
              >
                重启 RAG 服务
              </Button>
            )}
            <Button
              danger={!!ragStatus?.dimensionMismatch}
              loading={rebuildingMilvus}
              disabled={!ragStatus?.serviceReachable}
              onClick={() => handleRagRebuildAndSync()}
            >
              重建向量库并同步
            </Button>
            <Button
              type="primary"
              size="small"
              icon={<SaveOutlined />}
              loading={ragSaving}
              onClick={() => ragForm.submit()}
            >
              保存
            </Button>
          </Space>
        }
      >
        {ragEnvLocks.length > 0 && (
          <Text type="warning" style={{ display: 'block', fontSize: 12, marginBottom: 12 }}>
            以下项被 RAG 进程环境变量锁定，后台修改无效：{ragEnvLocks.join('、')}
          </Text>
        )}
        {!ragStatus?.serviceReachable && (
          <Text type="warning" style={{ display: 'block', fontSize: 12, marginBottom: 12 }}>
            RAG 服务当前不可达，仍可保存配置到数据库；保存后请重启 RAG 服务使配置生效。
          </Text>
        )}
        {ragStatus?.dimensionMismatch && (
          <Alert
            type="error"
            showIcon
            style={{ marginBottom: 16 }}
            message="向量维度不一致"
            description={
              <>
                当前嵌入模型为 {ragStatus.embedDim ?? '-'} 维，Milvus 集合为{' '}
                {ragStatus.collectionDim ?? '-'} 维，无法写入或检索向量。请点击右上角
                <Text strong>「重建向量库并同步」</Text>修复。
              </>
            }
          />
        )}
        <Form
          form={ragForm}
          layout="vertical"
          onFinish={(v) => void handleSaveRagEmbed(v)}
          initialValues={{
            embed_provider: 'local',
            embed_model: 'paraphrase-multilingual-MiniLM-L12-v2',
            embed_api_base: '',
            embed_api_key: '',
            chunk_max_chars: 420,
            chunk_overlap: 72,
            sync_enabled: true,
            sync_interval_sec: 120,
            sync_batch: 100,
          }}
        >
          <Row gutter={16}>
            <Col xs={24} md={8}>
              <Form.Item
                label="嵌入来源"
                name="embed_provider"
                rules={[{ required: true }]}
                extra="local=本地 Sentence-Transformers；api=OpenAI 兼容 Embedding API"
              >
                <Select
                  disabled={ragFieldLocked('rag.embed_provider')}
                  options={[
                    { value: 'local', label: '本地模型（Sentence-Transformers）' },
                    { value: 'api', label: '第三方 API（OpenAI 兼容）' },
                  ]}
                />
              </Form.Item>
            </Col>
            <Col xs={24} md={16}>
              <Form.Item
                label={ragProvider === 'api' ? 'Embedding 模型名（API model）' : '句向量模型（HuggingFace id）'}
                name="embed_model"
                rules={[{ required: true, message: '请输入模型名' }]}
                extra="更换模型后若维度变化，需「重建向量库并同步」"
              >
                <Input
                  disabled={ragFieldLocked('rag.embed_model')}
                  placeholder={
                    ragProvider === 'api' ? 'text-embedding-3-small' : 'paraphrase-multilingual-MiniLM-L12-v2'
                  }
                />
              </Form.Item>
            </Col>
            {ragProvider === 'local' && (
              <Col xs={24}>
                <Space wrap style={{ marginBottom: 16 }}>
                  {EMBED_PRESETS.map((p) => (
                    <Button
                      key={p.model}
                      size="small"
                      disabled={ragFieldLocked('rag.embed_model')}
                      onClick={() => ragForm.setFieldValue('embed_model', p.model)}
                    >
                      {p.label}
                    </Button>
                  ))}
                </Space>
              </Col>
            )}
            {ragProvider === 'api' && (
              <>
                <Col xs={24} md={14}>
                  <Form.Item
                    label="Embedding API Base URL"
                    name="embed_api_base"
                    rules={[{ required: true, message: '请输入 API Base URL' }]}
                  >
                    <Input disabled={ragFieldLocked('rag.embed_api_base')} placeholder="https://api.openai.com/v1" />
                  </Form.Item>
                  <Space wrap style={{ marginBottom: 16 }}>
                    {EMBED_API_PRESETS.map((p) => (
                      <Button
                        key={p.label}
                        size="small"
                        disabled={ragFieldLocked('rag.embed_api_base')}
                        onClick={() => ragForm.setFieldsValue({ embed_api_base: p.baseUrl, embed_model: p.model })}
                      >
                        {p.label}
                      </Button>
                    ))}
                  </Space>
                </Col>
                <Col xs={24} md={10}>
                  <Form.Item label="Embedding API Key" name="embed_api_key" extra={ragApiKeyHint}>
                    <Input.Password
                      disabled={ragFieldLocked('rag.embed_api_key')}
                      autoComplete="new-password"
                      placeholder={ragApiKeySet ? '留空则保留当前值' : 'sk-...'}
                    />
                  </Form.Item>
                </Col>
              </>
            )}
            <Col xs={24} md={6}>
              <Form.Item label="切块最大字符" name="chunk_max_chars">
                <InputNumber
                  min={128}
                  max={2000}
                  style={{ width: '100%' }}
                  disabled={ragFieldLocked('rag.chunk_max_chars')}
                />
              </Form.Item>
            </Col>
            <Col xs={24} md={6}>
              <Form.Item label="切块重叠字符" name="chunk_overlap">
                <InputNumber min={0} max={500} style={{ width: '100%' }} disabled={ragFieldLocked('rag.chunk_overlap')} />
              </Form.Item>
            </Col>
            <Col xs={24} md={6}>
              <Form.Item label="定时同步间隔（秒）" name="sync_interval_sec">
                <InputNumber
                  min={30}
                  max={86400}
                  style={{ width: '100%' }}
                  disabled={ragFieldLocked('rag.sync_interval_sec')}
                />
              </Form.Item>
            </Col>
            <Col xs={24} md={6}>
              <Form.Item
                label="单次同步文章数上限"
                name="sync_batch"
                extra="重建后可调大（最大 2000）以便一次同步全部"
              >
                <InputNumber min={1} max={2000} style={{ width: '100%' }} disabled={ragFieldLocked('rag.sync_batch')} />
              </Form.Item>
            </Col>
          </Row>
        </Form>

        <Table<ConfigSnapshot>
          size="small"
          title={() => (
            <Text type="secondary" style={{ fontSize: 12 }}>
              Embedding 配置变更历史
            </Text>
          )}
          rowKey="id"
          style={{ marginTop: 8 }}
          scroll={{ x: 1400 }}
          dataSource={ragHistory}
          pagination={{
            current: ragHistPage,
            total: ragHistTotal,
            pageSize: 8,
            showSizeChanger: false,
            onChange: (p) => void loadRagHistory(p),
          }}
          columns={ragSnapshotColumns}
        />
      </Card>

      <Card
        bordered={false}
        className={ui.panelCard}
        title={
          <span>
            <RobotOutlined style={{ marginRight: 8 }} />
            大模型配置
          </span>
        }
        extra={
          <Text type="secondary" style={{ fontSize: 12 }}>
            修改后立即生效并持久化到数据库
          </Text>
        }
      >
        <div style={{ marginBottom: 16 }}>
          <Text type="secondary" style={{ marginRight: 8, fontSize: 12 }}>
            快速填入：
          </Text>
          <Space wrap size="small">
            {PRESETS.map((p) => (
              <Button
                key={p.label}
                size="small"
                onClick={() => form.setFieldsValue({ llmBaseUrl: p.baseUrl, llmModel: p.model })}
              >
                {p.label}
              </Button>
            ))}
          </Space>
        </div>

        <Form
          form={form}
          layout="vertical"
          onFinish={handleSaveTagger}
          initialValues={{
            enabled: false,
            llmModel: 'deepseek-chat',
            llmBaseUrl: 'https://api.deepseek.com',
            llmApiKey: '',
            intervalSeconds: 120,
            batchSize: 20,
            maxPerTick: 200,
          }}
        >
          <Row gutter={24}>
            <Col xs={24} md={8}>
              <Form.Item label="启用后台任务" name="enabled" valuePropName="checked">
                <Switch checkedChildren="启用" unCheckedChildren="停用" />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item label="API Base URL" name="llmBaseUrl" rules={[{ required: true }]}>
                <Input placeholder="https://api.deepseek.com" />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item label="模型名" name="llmModel" rules={[{ required: true }]}>
                <Input placeholder="deepseek-chat" />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item label="API Key" name="llmApiKey" extra={apiKeyHint}>
            <Input.Password
              autoComplete="new-password"
              placeholder={tagger?.apiKeySet ? '留空则保留当前值' : 'sk-...'}
            />
          </Form.Item>
          <Row gutter={24}>
            <Col xs={24} md={8}>
              <Form.Item label="轮询间隔（秒）" name="intervalSeconds" rules={[{ required: true }]}>
                <InputNumber min={10} max={86400} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item label="单次 LLM 请求条数" name="batchSize" rules={[{ required: true }]}>
                <InputNumber min={1} max={100} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item label="单次轮询最多处理" name="maxPerTick" rules={[{ required: true }]}>
                <InputNumber min={1} max={10000} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item style={{ marginBottom: 0 }}>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={saving}>
                保存
              </Button>
              <Button
                onClick={() => {
                  if (cfg?.tagger) applyTaggerToForm(cfg.tagger)
                }}
              >
                重置
              </Button>
              {cfg?.note && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {cfg.note}
                </Text>
              )}
            </Space>
          </Form.Item>
        </Form>

        <Table<ConfigSnapshot>
          size="small"
          title={() => (
            <Text type="secondary" style={{ fontSize: 12 }}>
              大模型配置变更历史
            </Text>
          )}
          rowKey="id"
          style={{ marginTop: 16 }}
          scroll={{ x: 1200 }}
          dataSource={taggerHistory}
          pagination={{
            current: taggerHistPage,
            total: taggerHistTotal,
            pageSize: 8,
            showSizeChanger: false,
            onChange: (p) => void loadTaggerHistory(p),
          }}
          columns={taggerSnapshotColumns}
        />
      </Card>
    </div>
  )
}

export default AIConfigPage

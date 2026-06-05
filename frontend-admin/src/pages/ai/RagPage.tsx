import React, { useCallback, useEffect, useRef, useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  Descriptions,
  Drawer,
  Form,
  Input,
  InputNumber,
  List,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Switch,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import {
  CheckOutlined,
  ClockCircleOutlined,
  CloseOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
  LinkOutlined,
  ReloadOutlined,
  SaveOutlined,
  SearchOutlined,
  SyncOutlined,
} from '@ant-design/icons'
import { adminRagApi } from '@/api/admin-rag'
import { adminSystemApi } from '@/api/admin-system'
import { workflowApi } from '@/api/workflow'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'
import type {
  ConfigSnapshot,
  RagKBArticle,
  RagKBArticleDetail,
  RagKBChunk,
  RagSnapshotConfig,
  RagStatus,
  RagSyncLog,
  UpdateRagConfigPayload,
} from '@/types'
import dayjs from 'dayjs'

const { Text } = Typography
const { TextArea } = Input

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

const SENTIMENT_COLOR: Record<string, string> = { positive: 'green', negative: 'red', neutral: 'default' }

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

// ─── Chunk 行 ──────────────────────────────────────────────────────────────────
interface ChunkRowProps {
  chunk: RagKBChunk
  onUpdated: (pk: string, snippet: string) => void
  onDeleted: (pk: string) => void
}

const ChunkRow: React.FC<ChunkRowProps> = ({ chunk: ck, onUpdated, onDeleted }) => {
  const [editMode, setEditMode] = useState(false)
  const [editText, setEditText] = useState(ck.snippet)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const handleSave = async () => {
    if (!editText.trim()) return
    setSaving(true)
    try {
      await adminRagApi.updateChunk(ck.chunkPk, editText.trim())
      onUpdated(ck.chunkPk, editText.trim())
      setEditMode(false)
      message.success('chunk 已重新向量化')
    } catch { message.error('保存失败') }
    finally { setSaving(false) }
  }

  const handleDelete = async () => {
    setDeleting(true)
    try {
      await adminRagApi.deleteChunk(ck.chunkPk)
      onDeleted(ck.chunkPk)
      message.success('chunk 已删除（下次同步会重建）')
    } catch { message.error('删除失败') }
    finally { setDeleting(false) }
  }

  return (
    <List.Item style={{ alignItems: 'flex-start' }}>
      <div style={{ display: 'flex', gap: 8, width: '100%', alignItems: 'flex-start' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 4, flexShrink: 0, paddingTop: 2 }}>
          <Text type="secondary" style={{ fontSize: 11, lineHeight: 1 }}>#{ck.chunkIdx}</Text>
          {ck.chunkType === 'comment' ? <Tag color="blue" style={{ margin: 0 }}>评论</Tag> : <Tag color="purple" style={{ margin: 0 }}>正文</Tag>}
        </div>
        <div style={{ flex: 1 }}>
          {editMode ? (
            <Space direction="vertical" style={{ width: '100%' }} size={4}>
              <TextArea value={editText} onChange={e => setEditText(e.target.value)} autoSize={{ minRows: 3, maxRows: 10 }} style={{ fontSize: 12 }} />
              <Space>
                <Button size="small" type="primary" icon={<CheckOutlined />} loading={saving} onClick={() => void handleSave()}>保存并重算向量</Button>
                <Button size="small" icon={<CloseOutlined />} onClick={() => { setEditMode(false); setEditText(ck.snippet) }}>取消</Button>
              </Space>
            </Space>
          ) : (
            <Text style={{ fontSize: 12 }}>{ck.snippet.length > 200 ? ck.snippet.slice(0, 200) + '…' : ck.snippet}</Text>
          )}
        </div>
        {!editMode && (
          <Space size={4} style={{ flexShrink: 0 }}>
            <Button type="text" size="small" icon={<EditOutlined />} onClick={() => { setEditText(ck.snippet); setEditMode(true) }} />
            <Popconfirm title="从 Milvus 删除此 chunk？下次同步时会重建。" onConfirm={handleDelete} okText="删除" cancelText="取消">
              <Button type="text" size="small" danger icon={<DeleteOutlined />} loading={deleting} />
            </Popconfirm>
          </Space>
        )}
      </div>
    </List.Item>
  )
}

// ─── 配置 Tab ──────────────────────────────────────────────────────────────────
const RagConfigTab: React.FC = () => {
  const [ragForm] = Form.useForm<RagFormValues>()
  const ragProvider = Form.useWatch('embed_provider', ragForm) ?? 'local'
  const ragSyncEnabledWatch = Form.useWatch('sync_enabled', ragForm) ?? true
  const [ragSaving, setRagSaving] = useState(false)
  const [ragEnvLocks, setRagEnvLocks] = useState<string[]>([])
  const [ragApiKeySet, setRagApiKeySet] = useState(false)
  const [ragStatus, setRagStatus] = useState<RagStatus | null>(null)
  const [rebuildingMilvus, setRebuildingMilvus] = useState(false)
  const [restartingRag, setRestartingRag] = useState(false)
  const [loading, setLoading] = useState(false)
  const [history, setHistory] = useState<ConfigSnapshot[]>([])
  const [histTotal, setHistTotal] = useState(0)
  const [histPage, setHistPage] = useState(1)

  const refreshRagConfigForm = useCallback(async () => {
    const cfg2 = await adminRagApi.getConfig().catch(() => null)
    if (cfg2 != null) {
      setRagEnvLocks(cfg2.env_overrides ?? [])
      setRagApiKeySet(cfg2.api_key_set ?? false)
      ragForm.setFieldsValue({
        embed_provider: (cfg2.embed_provider === 'api' ? 'api' : 'local') as 'local' | 'api',
        embed_model: cfg2.embed_model, embed_api_base: cfg2.embed_api_base ?? '',
        embed_api_key: '', chunk_max_chars: cfg2.chunk_max_chars,
        chunk_overlap: cfg2.chunk_overlap, sync_enabled: cfg2.sync_enabled ?? true,
        sync_interval_sec: cfg2.sync_interval_sec, sync_batch: cfg2.sync_batch,
      })
    }
  }, [ragForm])

  const loadHistory = useCallback(async (page: number) => {
    const r = await adminSystemApi.settingHistory({ domain: 'rag', page, pageSize: 8 })
    setHistory(r.list); setHistTotal(r.total); setHistPage(page)
  }, [])

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      const rs = await adminRagApi.status().catch(() => null)
      setRagStatus(rs as RagStatus | null)
      await refreshRagConfigForm()
      await loadHistory(1)
    } finally { setLoading(false) }
  }, [refreshRagConfigForm, loadHistory])

  useEffect(() => { void fetchAll() }, [fetchAll])

  const ragFieldLocked = (dbKey: string) => ragEnvLocks.includes(dbKey)

  const handleSaveRagEmbed = async (values: RagFormValues) => {
    setRagSaving(true)
    try {
      const payload: UpdateRagConfigPayload = {
        embed_provider: values.embed_provider, embed_model: values.embed_model.trim(),
        chunk_max_chars: values.chunk_max_chars, chunk_overlap: values.chunk_overlap,
        sync_enabled: values.sync_enabled, sync_interval_sec: values.sync_interval_sec,
        sync_batch: values.sync_batch,
      }
      if (values.embed_provider === 'api') payload.embed_api_base = (values.embed_api_base ?? '').trim()
      const keyTrimmed = (values.embed_api_key ?? '').trim()
      if (keyTrimmed) payload.embed_api_key = keyTrimmed
      const resp = await adminRagApi.updateConfig(payload)
      setRagEnvLocks(resp.env_overrides ?? [])
      setRagApiKeySet(resp.api_key_set ?? ragApiKeySet)
      ragForm.setFieldValue('embed_api_key', '')
      if (resp.warnings?.length) message.warning(resp.warnings.join('；'))
      else if (resp.warning) message.warning(resp.warning)
      else if (resp.service_applied === false) message.success('配置已保存到数据库，请重启 RAG 服务后生效')
      else message.success('Embedding 配置已保存（换模型后建议立即同步）')
      void adminRagApi.status().then(setRagStatus).catch(() => undefined)
      void loadHistory(1)
    } catch { message.error('保存失败') }
    finally { setRagSaving(false) }
  }

  const handleRagRestart = () => {
    Modal.confirm({
      title: '重启 RAG 服务',
      content: '将停止占用端口的旧 RAG 进程并重新拉起。本地模型首次加载可能需 1～2 分钟，是否继续？',
      okText: '重启', cancelText: '取消',
      onOk: async () => {
        setRestartingRag(true)
        try {
          const result = await adminRagApi.restartService()
          if (result.healthReady) message.success(result.message || `RAG 已就绪（PID ${result.pid ?? '-'})`)
          else message.loading(result.message || 'RAG 启动中…', 0)
          const deadline = Date.now() + 180_000
          while (Date.now() < deadline) {
            await new Promise((r) => setTimeout(r, 2000))
            const rs = await adminRagApi.status().catch(() => null)
            if (rs) setRagStatus(rs)
            if (rs?.serviceReachable) {
              message.destroy()
              message.success(rs.embedderReady === false
                ? `RAG HTTP 已就绪（PID ${result.pid ?? '-'}），嵌入模型仍在加载`
                : `RAG 服务已就绪（PID ${result.pid ?? rs.processPid ?? '-'})`)
              return
            }
          }
          message.destroy()
          message.warning('RAG 进程已提交启动，但等待就绪超时；请查看 rag/logs/rag_service_managed.log')
        } catch (e: unknown) {
          message.destroy()
          const err = e as { response?: { data?: { message?: string } }; message?: string }
          message.error(err.response?.data?.message || err.message || '重启失败')
        } finally { setRestartingRag(false) }
      },
    })
  }

  const handleRagRebuildAndSync = () => {
    Modal.confirm({
      title: '重建 Milvus 向量库',
      content: '将删除当前 Milvus 集合中的全部向量，并按当前嵌入模型维度重建；同时清空文章同步标记以便全量重算。此操作不可撤销，是否继续？',
      okText: '重建并同步', cancelText: '取消', okButtonProps: { danger: true },
      onOk: async () => {
        setRebuildingMilvus(true)
        try {
          const result = await adminRagApi.rebuildMilvus()
          message.success(`向量库已重建（${result.collection_dimension} 维），已重置 ${result.articles_reset_for_resync} 篇文章的同步标记`)
          void adminRagApi.status().then(setRagStatus).catch(() => undefined)
          await adminRagApi.triggerSync()
          message.success('已提交全量向量同步，可切换到「同步任务」tab 查看进度')
        } catch (e: unknown) {
          const err = e as { response?: { data?: { message?: string } }; message?: string }
          message.error(err.response?.data?.message || err.message || '重建失败')
        } finally { setRebuildingMilvus(false) }
      },
    })
  }

  const handleDeleteHistory = async (id: number) => {
    try { await adminSystemApi.deleteSettingHistory(id); message.success('已删除'); await loadHistory(histPage) } catch { /* ignore */ }
  }

  const handleReapplyHistory = async (id: number) => {
    try {
      const resp = await adminSystemApi.reapplySettingHistory(id)
      resp.warning ? message.warning(resp.message) : message.success(resp.message)
      await Promise.all([loadHistory(histPage), refreshRagConfigForm()])
      void adminRagApi.status().then(setRagStatus).catch(() => undefined)
    } catch { /* ignore */ }
  }

  const ragApiKeyHint = ragApiKeySet ? '已配置，留空则保留当前值' : '使用第三方 API 时必须填写'

  const historyColumns = [
    { title: '嵌入来源', width: 88, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).embed_provider === 'api' ? <Tag color="purple">API</Tag> : <Tag color="blue">本地</Tag> },
    { title: '模型', width: 160, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).embed_model || '-' },
    { title: 'API URL', width: 200, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).embed_api_base || '-' },
    { title: 'API Key', width: 120, ellipsis: true, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).embed_api_key || '-' },
    { title: '切块', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).chunk_max_chars },
    { title: '重叠', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).chunk_overlap },
    { title: '同步间隔', width: 88, render: (_: unknown, row: ConfigSnapshot) => `${(row.config as RagSnapshotConfig).sync_interval_sec}s` },
    { title: '批量', width: 72, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).sync_batch },
    { title: '定时同步', width: 88, render: (_: unknown, row: ConfigSnapshot) => (row.config as RagSnapshotConfig).sync_enabled ? <Tag color="success">开</Tag> : <Tag>关</Tag> },
    { title: '操作者', width: 88, dataIndex: 'updatedByName', render: (v: string) => v || '-' },
    { title: '时间', width: 128, dataIndex: 'createdAt', render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss') },
    {
      title: '操作', key: 'actions', width: 100, fixed: 'right' as const,
      render: (_: unknown, row: ConfigSnapshot) => (
        <Space size={4} wrap>
          <Button type="link" size="small" onClick={() => void handleReapplyHistory(row.id)}>应用</Button>
          <Popconfirm title="确定删除这条历史记录？" onConfirm={() => void handleDeleteHistory(row.id)}>
            <Button type="link" size="small" danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card bordered={false} className={ui.panelCard} loading={loading}
      title={<span><DatabaseOutlined style={{ marginRight: 8 }} />Embedding 配置</span>}
      extra={
        <Space>
          <Space size={4}>
            <Text style={{ fontSize: 13 }}>定时同步</Text>
            <Switch size="small" checked={ragSyncEnabledWatch} checkedChildren="开" unCheckedChildren="关" onChange={(v) => ragForm.setFieldValue('sync_enabled', v)} />
          </Space>
          {ragStatus?.processManaged && (
            <Button icon={<ReloadOutlined />} loading={restartingRag} onClick={() => handleRagRestart()}>重启 RAG 服务</Button>
          )}
          <Button danger={!!ragStatus?.dimensionMismatch} loading={rebuildingMilvus} disabled={!ragStatus?.serviceReachable} onClick={() => handleRagRebuildAndSync()}>
            重建向量库并同步
          </Button>
          <Button type="primary" size="small" icon={<SaveOutlined />} loading={ragSaving} onClick={() => ragForm.submit()}>保存</Button>
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
        <Alert type="error" showIcon style={{ marginBottom: 16 }} message="向量维度不一致"
          description={<>当前嵌入模型为 {ragStatus.embedDim ?? '-'} 维，Milvus 集合为 {ragStatus.collectionDim ?? '-'} 维，无法写入或检索向量。请点击右上角<Text strong>「重建向量库并同步」</Text>修复。</>}
        />
      )}

      <Form form={ragForm} layout="vertical" onFinish={(v) => void handleSaveRagEmbed(v)}
        initialValues={{ embed_provider: 'local', embed_model: 'paraphrase-multilingual-MiniLM-L12-v2', embed_api_base: '', embed_api_key: '', chunk_max_chars: 420, chunk_overlap: 72, sync_enabled: true, sync_interval_sec: 120, sync_batch: 100 }}
      >
        <Row gutter={16}>
          <Col xs={24} md={8}>
            <Form.Item label="嵌入来源" name="embed_provider" rules={[{ required: true }]} extra="local=本地 Sentence-Transformers；api=OpenAI 兼容 Embedding API">
              <Select disabled={ragFieldLocked('rag.embed_provider')} options={[{ value: 'local', label: '本地模型（Sentence-Transformers）' }, { value: 'api', label: '第三方 API（OpenAI 兼容）' }]} />
            </Form.Item>
          </Col>
          <Col xs={24} md={16}>
            <Form.Item label={ragProvider === 'api' ? 'Embedding 模型名（API model）' : '句向量模型（HuggingFace id）'} name="embed_model" rules={[{ required: true, message: '请输入模型名' }]} extra="更换模型后若维度变化，需「重建向量库并同步」">
              <Input disabled={ragFieldLocked('rag.embed_model')} placeholder={ragProvider === 'api' ? 'text-embedding-3-small' : 'paraphrase-multilingual-MiniLM-L12-v2'} />
            </Form.Item>
          </Col>
          {ragProvider === 'local' && (
            <Col xs={24}>
              <Space wrap style={{ marginBottom: 16 }}>
                {EMBED_PRESETS.map((p) => (
                  <Button key={p.model} size="small" disabled={ragFieldLocked('rag.embed_model')} onClick={() => ragForm.setFieldValue('embed_model', p.model)}>{p.label}</Button>
                ))}
              </Space>
            </Col>
          )}
          {ragProvider === 'api' && (
            <>
              <Col xs={24} md={14}>
                <Form.Item label="Embedding API Base URL" name="embed_api_base" rules={[{ required: true, message: '请输入 API Base URL' }]}>
                  <Input disabled={ragFieldLocked('rag.embed_api_base')} placeholder="https://api.openai.com/v1" />
                </Form.Item>
                <Space wrap style={{ marginBottom: 16 }}>
                  {EMBED_API_PRESETS.map((p) => (
                    <Button key={p.label} size="small" disabled={ragFieldLocked('rag.embed_api_base')} onClick={() => ragForm.setFieldsValue({ embed_api_base: p.baseUrl, embed_model: p.model })}>{p.label}</Button>
                  ))}
                </Space>
              </Col>
              <Col xs={24} md={10}>
                <Form.Item label="Embedding API Key" name="embed_api_key" extra={ragApiKeyHint}>
                  <Input.Password disabled={ragFieldLocked('rag.embed_api_key')} autoComplete="new-password" placeholder={ragApiKeySet ? '留空则保留当前值' : 'sk-...'} />
                </Form.Item>
              </Col>
            </>
          )}
          <Col xs={24} md={6}>
            <Form.Item label="切块最大字符" name="chunk_max_chars">
              <InputNumber min={128} max={2000} style={{ width: '100%' }} disabled={ragFieldLocked('rag.chunk_max_chars')} />
            </Form.Item>
          </Col>
          <Col xs={24} md={6}>
            <Form.Item label="切块重叠字符" name="chunk_overlap">
              <InputNumber min={0} max={500} style={{ width: '100%' }} disabled={ragFieldLocked('rag.chunk_overlap')} />
            </Form.Item>
          </Col>
          <Col xs={24} md={6}>
            <Form.Item label="定时同步间隔（秒）" name="sync_interval_sec">
              <InputNumber min={30} max={86400} style={{ width: '100%' }} disabled={ragFieldLocked('rag.sync_interval_sec')} />
            </Form.Item>
          </Col>
          <Col xs={24} md={6}>
            <Form.Item label="单次同步文章数上限" name="sync_batch" extra="重建后可调大（最大 2000）以便一次同步全部">
              <InputNumber min={1} max={2000} style={{ width: '100%' }} disabled={ragFieldLocked('rag.sync_batch')} />
            </Form.Item>
          </Col>
        </Row>
      </Form>

      <Table<ConfigSnapshot>
        size="small" title={() => <Text type="secondary" style={{ fontSize: 12 }}>Embedding 配置变更历史</Text>}
        rowKey="id" style={{ marginTop: 8 }} scroll={{ x: 1400 }} dataSource={history}
        pagination={{ current: histPage, total: histTotal, pageSize: 8, showSizeChanger: false, onChange: (p) => void loadHistory(p) }}
        columns={historyColumns}
      />
    </Card>
  )
}

// ─── 同步任务 Tab ──────────────────────────────────────────────────────────────
const RagSyncTab: React.FC = () => {
  const [ragStatus, setRagStatus] = useState<RagStatus | null>(null)
  const [pendingEmbed, setPendingEmbed] = useState<number | null>(null)
  const [ragSyncEnabled, setRagSyncEnabled] = useState<boolean>(true)
  const [syncToggling, setSyncToggling] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [ragRuns, setRagRuns] = useState<RagSyncLog[]>([])
  const [ragTotal, setRagTotal] = useState(0)
  const [ragPage, setRagPage] = useState(1)
  const [ragLoading, setRagLoading] = useState(false)
  const [loading, setLoading] = useState(false)
  const refreshRef = useRef<ReturnType<typeof window.setInterval> | null>(null)

  const loadRagRuns = useCallback(async (page: number) => {
    setRagLoading(true)
    try {
      const r = await adminRagApi.runs({ page, pageSize: 10 })
      setRagTotal(r.total); setRagRuns(r.list); setRagPage(page)
    } finally { setRagLoading(false) }
  }, [])

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      await Promise.all([
        adminRagApi.status().then((rs) => { setRagStatus(rs); if (rs.syncEnabled != null) setRagSyncEnabled(rs.syncEnabled) }).catch(() => {}),
        adminRagApi.listArticles({ synced: 'no', page_size: 1 }).then((r) => setPendingEmbed(r.total)).catch(() => {}),
        loadRagRuns(1),
      ])
    } finally { setLoading(false) }
  }, [loadRagRuns])

  useEffect(() => {
    void fetchAll()
    refreshRef.current = window.setInterval(() => {
      void adminRagApi.listArticles({ synced: 'no', page_size: 1 }).then((r) => setPendingEmbed(r.total)).catch(() => {})
      void loadRagRuns(ragPage)
      void adminRagApi.status().then(setRagStatus).catch(() => undefined)
    }, 12_000)
    return () => { if (refreshRef.current) window.clearInterval(refreshRef.current) }
  }, [fetchAll, loadRagRuns, ragPage])

  const handleRagSyncToggle = async (checked: boolean) => {
    setSyncToggling(true)
    try {
      await adminRagApi.updateConfig({ sync_enabled: checked })
      setRagSyncEnabled(checked)
      message.success(checked ? '已启用定时同步' : '已暂停定时同步')
    } catch { message.error('设置失败') }
    finally { setSyncToggling(false) }
  }

  const handleRagSync = async () => {
    if (ragStatus?.dimensionMismatch) {
      message.warning('向量维度与 Milvus 集合不一致，请先到「配置」tab 执行「重建向量库并同步」')
      return
    }
    setSyncing(true)
    try {
      await adminRagApi.triggerSync()
      message.success('已提交向量同步，可在下方列表查看进度')
      await loadRagRuns(1)
      void adminRagApi.status().then(setRagStatus).catch(() => undefined)
    } catch (e: unknown) {
      const err = e as { response?: { data?: { message?: string } }; message?: string }
      message.error(err.response?.data?.message || err.message || '同步提交失败')
    } finally { setSyncing(false) }
  }

  return (
    <Card bordered={false} className={ui.panelCard}
      title={<span><DatabaseOutlined style={{ marginRight: 8 }} />RAG 向量同步</span>}
      extra={
        <Space>
          <Space size={4}>
            <Text style={{ fontSize: 13 }}>定时同步</Text>
            <Switch size="small" checked={ragSyncEnabled} loading={syncToggling} onChange={(v) => void handleRagSyncToggle(v)} checkedChildren="开" unCheckedChildren="关" />
          </Space>
          <Button type="primary" size="small" icon={<SyncOutlined />} loading={syncing} disabled={!ragStatus?.embeddingServiceUrl || !!ragStatus?.dimensionMismatch} onClick={() => void handleRagSync()}>
            立即同步
          </Button>
          <Button size="small" onClick={() => void loadRagRuns(ragPage)} loading={ragLoading}>刷新</Button>
        </Space>
      }
    >
      <Row gutter={[16, 16]} style={{ marginBottom: 20 }}>
        <Col xs={24} sm={8}>
          <Card size="small" style={{ textAlign: 'center' }}>
            <Statistic title="待向量化文章" value={pendingEmbed ?? '-'} suffix="篇" loading={loading}
              valueStyle={{ color: pendingEmbed && pendingEmbed > 0 ? '#E8A84A' : '#42C48C', fontSize: 28 }} />
            <div style={{ marginTop: 4 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>{pendingEmbed === 0 ? '全部已同步' : '等待向量化同步'}</Text>
            </div>
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small" style={{ textAlign: 'center' }}>
            <Statistic title="同步周期" value={ragStatus?.syncIntervalSecondsHint != null ? `${ragStatus.syncIntervalSecondsHint}s` : '-'}
              prefix={<ClockCircleOutlined />} valueStyle={{ fontSize: 28 }} />
            <div style={{ marginTop: 4 }}>
              {ragSyncEnabled
                ? <Badge status="processing" text={<Text type="secondary" style={{ fontSize: 12 }}>定时同步运行中</Text>} />
                : <Badge status="default" text={<Text type="secondary" style={{ fontSize: 12 }}>定时同步已暂停</Text>} />}
            </div>
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small" style={{ textAlign: 'center' }}>
            <Statistic title="RAG 服务" value={ragStatus?.serviceReachable ? '可达' : '不可达'}
              valueStyle={{ color: ragStatus?.serviceReachable ? '#42C48C' : '#EC6B6B', fontSize: 28 }} />
            {ragStatus?.embedModel && (
              <div style={{ marginTop: 4 }}>
                <Text type="secondary" style={{ fontSize: 11 }} ellipsis>{ragStatus.embedModel}</Text>
              </div>
            )}
          </Card>
        </Col>
      </Row>

      {ragStatus?.dimensionMismatch && (
        <Alert type="error" showIcon className={ui.infoBanner} message="向量维度不一致"
          description={<>当前嵌入模型为 {ragStatus.embedDim ?? '-'} 维，Milvus 集合为 {ragStatus.collectionDim ?? '-'} 维，无法写入向量。请前往<Text strong>「配置 → 重建向量库并同步」</Text>修复。</>}
        />
      )}
      {ragStatus?.embedderError && !ragStatus?.dimensionMismatch && (
        <Alert type="warning" showIcon className={ui.infoBanner} message="嵌入模型未就绪" description={ragStatus.embedderError} />
      )}

      <Table<RagSyncLog>
        size="small" rowKey="id" loading={ragLoading} dataSource={ragRuns} scroll={{ x: 960 }} className={ui.tableWrap}
        pagination={{ current: ragPage, total: ragTotal, pageSize: 10, showSizeChanger: false, onChange: (p) => void loadRagRuns(p) }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 72 },
          { title: '状态', dataIndex: 'status', width: 92, render: (s: RagSyncLog['status']) => <Tag className={s === 'success' ? ui.softTagSage : s === 'failed' ? ui.softTagRose : ui.softTagBlue}>{s}</Tag> },
          { title: '方式', dataIndex: 'mode', width: 100 },
          { title: '进度', dataIndex: 'progress', width: 72, render: (p: number) => `${p}%` },
          { title: '文章数', dataIndex: 'articlesProcessed', width: 88 },
          { title: '写入块', dataIndex: 'chunksUpserted', width: 88 },
          { title: '清理', dataIndex: 'chunksDeleted', width: 72 },
          { title: '开始', dataIndex: 'startedAt', width: 128, render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss') },
          { title: '结束', dataIndex: 'finishedAt', width: 128, render: (t: string | undefined) => t ? dayjs(t).format('MM-DD HH:mm:ss') : '-' },
          { title: '详情', dataIndex: 'progressDetail', ellipsis: true, render: (t: string) => t || '-' },
        ]}
      />
    </Card>
  )
}

// ─── 知识库 Tab ────────────────────────────────────────────────────────────────
const RagKBTab: React.FC = () => {
  const [list, setList] = useState<RagKBArticle[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [keyword, setKeyword] = useState('')
  const [inputKw, setInputKw] = useState('')
  const [platform, setPlatform] = useState('')
  const [topic, setTopic] = useState('')
  const [synced, setSynced] = useState<'yes' | 'no' | ''>('')
  const [deletingIds, setDeletingIds] = useState<Set<number>>(new Set())
  const [topicOptions, setTopicOptions] = useState<string[]>([])
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detail, setDetail] = useState<RagKBArticleDetail | null>(null)

  const PAGE_SIZE = 20

  const fetchList = useCallback(async (pg: number) => {
    setLoading(true)
    try {
      const r = await adminRagApi.listArticles({ page: pg, page_size: PAGE_SIZE, keyword: keyword || undefined, platform: platform || undefined, topic: topic || undefined, synced: synced || undefined })
      setList(r.list); setTotal(r.total); setPage(pg)
    } finally { setLoading(false) }
  }, [keyword, platform, topic, synced])

  useEffect(() => { void fetchList(1) }, [fetchList])

  useEffect(() => {
    workflowApi.listTopics().then((res) => setTopicOptions(res.topics)).catch(() => {})
  }, [])

  const handleDeleteEmbedding = async (id: number) => {
    setDeletingIds((prev) => new Set(prev).add(id))
    try { await adminRagApi.deleteEmbedding(id); message.success('已删除向量，下次同步时将重新生成'); void fetchList(page) }
    catch { message.error('操作失败') }
    finally { setDeletingIds((prev) => { const s = new Set(prev); s.delete(id); return s }) }
  }

  const handleOpenDetail = async (id: number) => {
    setDrawerOpen(true); setDetail(null); setDetailLoading(true)
    try { setDetail(await adminRagApi.getArticleDetail(id)) }
    catch { message.error('获取详情失败，请确认 RAG 服务正在运行'); setDrawerOpen(false) }
    finally { setDetailLoading(false) }
  }

  return (
    <>
      <Card bordered={false} className={`${ui.panelCard} ${ui.toolbar}`}>
        <Space wrap>
          <Input placeholder="搜索标题/内容" allowClear prefix={<SearchOutlined />} style={{ width: 240 }} value={inputKw}
            onChange={(e) => setInputKw(e.target.value)} onPressEnter={() => setKeyword(inputKw.trim())} />
          <Button onClick={() => setKeyword(inputKw.trim())} type="primary">搜索</Button>
          <Select style={{ width: 140 }} placeholder="平台筛选" allowClear value={platform || undefined} onChange={(v) => setPlatform(v ?? '')}
            options={[{ label: 'weibo', value: 'weibo' }, { label: 'xhs', value: 'xhs' }, { label: 'zhihu', value: 'zhihu' }, { label: 'bilibili', value: 'bilibili' }, { label: 'douyin', value: 'douyin' }, { label: 'tieba', value: 'tieba' }]}
          />
          <Select style={{ width: 140 }} placeholder="话题筛选" allowClear value={topic || undefined} onChange={(v) => setTopic(v ?? '')}
            options={topicOptions.map((t) => ({ label: t, value: t }))}
          />
          <Select style={{ width: 140 }} placeholder="向量状态" allowClear value={synced || undefined} onChange={(v) => setSynced((v as 'yes' | 'no' | '') ?? '')}
            options={[{ label: '已向量化', value: 'yes' }, { label: '未向量化', value: 'no' }]}
          />
          <Text type="secondary" style={{ fontSize: 12 }}>共 {total} 条</Text>
        </Space>
      </Card>

      <Card bordered={false} className={`${ui.panelCard} ${ui.tableWrap}`}>
        <Table<RagKBArticle>
          size="small" rowKey="id" loading={loading} dataSource={list} scroll={{ x: 900 }}
          pagination={{ current: page, total, pageSize: PAGE_SIZE, showSizeChanger: false, showTotal: (t) => `共 ${t} 条`, onChange: (p) => void fetchList(p) }}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 72 },
            { title: '标题', dataIndex: 'title', ellipsis: true, render: (t: string) => <Tooltip title={t}><span>{t || '-'}</span></Tooltip> },
            { title: '平台', dataIndex: 'platform', width: 100 },
            { title: '话题', dataIndex: 'topic', width: 120, ellipsis: true },
            { title: '发布时间', dataIndex: 'publishedAt', width: 128, render: (t?: string) => t ? dayjs(t).format('MM-DD HH:mm') : '-' },
            { title: '向量状态', dataIndex: 'synced', width: 110, render: (v: boolean) => v ? <Badge status="success" text={<Tag color="green">已向量化</Tag>} /> : <Badge status="default" text={<Tag>未同步</Tag>} /> },
            { title: '最近同步时间', dataIndex: 'embeddingSyncedAt', width: 148, render: (t?: string) => t ? dayjs(t).format('MM-DD HH:mm:ss') : '-' },
            {
              title: '操作', width: 160,
              render: (_, row) => (
                <Space size={4}>
                  <Button size="small" icon={<EyeOutlined />} onClick={() => void handleOpenDetail(row.id)}>详情</Button>
                  {row.synced && (
                    <Popconfirm title="删除此文章的向量？下次同步时将自动重建。" onConfirm={() => void handleDeleteEmbedding(row.id)} okText="删除" cancelText="取消">
                      <Button size="small" danger icon={<DeleteOutlined />} loading={deletingIds.has(row.id)}>删除向量</Button>
                    </Popconfirm>
                  )}
                </Space>
              ),
            },
          ]}
        />
      </Card>

      <Drawer title={detail?.article.title || '文章详情'} width={720} open={drawerOpen} onClose={() => setDrawerOpen(false)} destroyOnClose>
        {detailLoading && <Spin style={{ display: 'block', margin: '80px auto' }} />}
        {!detailLoading && detail && (
          <Space direction="vertical" style={{ width: '100%' }} size={24}>
            <Descriptions size="small" bordered column={2}>
              <Descriptions.Item label="平台">{detail.article.platform || '-'}</Descriptions.Item>
              <Descriptions.Item label="作者">{detail.article.author || '-'}</Descriptions.Item>
              <Descriptions.Item label="发布时间">{detail.article.publishedAt ? dayjs(detail.article.publishedAt).format('YYYY-MM-DD HH:mm') : '-'}</Descriptions.Item>
              <Descriptions.Item label="情感倾向"><Tag color={SENTIMENT_COLOR[detail.article.sentiment] ?? 'default'}>{detail.article.sentiment || '-'}</Tag></Descriptions.Item>
              {detail.article.aiTags && (
                <Descriptions.Item label="AI 标签" span={2}>
                  {(() => { try { return (JSON.parse(detail.article.aiTags ?? '[]') as string[]).map((t) => <Tag key={t}>{t}</Tag>) } catch { return <Text type="secondary">{detail.article.aiTags}</Text> } })()}
                </Descriptions.Item>
              )}
              <Descriptions.Item label="向量状态" span={2}>
                {detail.article.synced ? <Tag color="green">已向量化（{dayjs(detail.article.embeddingSyncedAt).format('MM-DD HH:mm')}）</Tag> : <Tag color="orange">待同步</Tag>}
              </Descriptions.Item>
              {detail.article.originUrl && (
                <Descriptions.Item label="原文链接" span={2}>
                  <a href={detail.article.originUrl} target="_blank" rel="noreferrer"><LinkOutlined style={{ marginRight: 4 }} />查看原文</a>
                </Descriptions.Item>
              )}
            </Descriptions>

            <div>
              <Space>
                <Text strong>向量切块（{detail.chunks.length} 个）</Text>
                <Text type="secondary" style={{ fontSize: 11 }}>编辑后立即重算向量；下次全量重同步会覆盖手动修改</Text>
              </Space>
              {detail.chunks.length === 0
                ? <div style={{ marginTop: 8, color: '#999', fontSize: 13 }}>{detail.article.synced ? '暂无 chunk 数据' : 'RAG 服务未运行或尚未同步'}</div>
                : (
                  <List style={{ marginTop: 8, maxHeight: 400, overflowY: 'auto' }} size="small"
                    dataSource={detail.chunks}
                    renderItem={(ck) => (
                      <ChunkRow key={ck.chunkPk} chunk={ck}
                        onUpdated={(pk, snippet) => setDetail((prev) => prev ? { ...prev, chunks: prev.chunks.map((c) => c.chunkPk === pk ? { ...c, snippet } : c) } : prev)}
                        onDeleted={(pk) => setDetail((prev) => prev ? { ...prev, chunks: prev.chunks.filter((c) => c.chunkPk !== pk) } : prev)}
                      />
                    )}
                  />
                )}
            </div>
          </Space>
        )}
      </Drawer>
    </>
  )
}

// ─── 主页面 ────────────────────────────────────────────────────────────────────
const RagPage: React.FC = () => (
  <div className={ui.pageShell}>
    <PageHeader
      title="向量知识库"
      subtitle="Embedding 配置、向量同步任务监控与知识库文章管理"
      icon={<DatabaseOutlined />}
    />
    <Tabs
      items={[
        { key: 'config', label: '配置', children: <RagConfigTab /> },
        { key: 'sync', label: '同步任务', children: <RagSyncTab /> },
        { key: 'kb', label: '知识库', children: <RagKBTab /> },
      ]}
    />
  </div>
)

export default RagPage

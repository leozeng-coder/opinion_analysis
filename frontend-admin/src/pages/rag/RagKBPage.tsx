import React, { useCallback, useEffect, useState } from 'react'
import {
  Badge,
  Button,
  Card,
  Descriptions,
  Drawer,
  Input,
  List,
  Popconfirm,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import {
  CheckOutlined,
  CloseOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
  LinkOutlined,
  ReloadOutlined,
  SearchOutlined,
} from '@ant-design/icons'
import { adminRagApi } from '@/api/admin-rag'
import { workflowApi } from '@/api/workflow'
import { platformLabel, PLATFORMS } from '@/utils/platform'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'
import type { RagKBArticle, RagKBArticleDetail, RagKBChunk } from '@/types'
import dayjs from 'dayjs'

const { Text } = Typography
const { TextArea } = Input

const SENTIMENT_COLOR: Record<string, string> = {
  positive: 'green',
  negative: 'red',
  neutral: 'default',
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
              <TextArea
                value={editText}
                onChange={e => setEditText(e.target.value)}
                autoSize={{ minRows: 3, maxRows: 10 }}
                style={{ fontSize: 12 }}
              />
              <Space>
                <Button size="small" type="primary" icon={<CheckOutlined />}
                  loading={saving} onClick={() => void handleSave()}>
                  保存并重算向量
                </Button>
                <Button size="small" icon={<CloseOutlined />} onClick={() => { setEditMode(false); setEditText(ck.snippet) }}>
                  取消
                </Button>
              </Space>
            </Space>
          ) : (
            <Text style={{ fontSize: 12 }}>
              {ck.snippet.length > 200 ? ck.snippet.slice(0, 200) + '…' : ck.snippet}
            </Text>
          )}
        </div>
        {!editMode && (
          <Space size={4} style={{ flexShrink: 0 }}>
            <Button type="text" size="small" icon={<EditOutlined />}
              onClick={() => { setEditText(ck.snippet); setEditMode(true) }} />
            <Popconfirm title="从 Milvus 删除此 chunk？下次同步时会重建。"
              onConfirm={handleDelete} okText="删除" cancelText="取消">
              <Button type="text" size="small" danger icon={<DeleteOutlined />} loading={deleting} />
            </Popconfirm>
          </Space>
        )}
      </div>
    </List.Item>
  )
}

// ─── 主页面 ────────────────────────────────────────────────────────────────────
const RagKBPage: React.FC = () => {
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
      const r = await adminRagApi.listArticles({
        page: pg, page_size: PAGE_SIZE,
        keyword: keyword || undefined,
        platform: platform || undefined,
        topic: topic || undefined,
        synced: synced || undefined,
      })
      setList(r.list); setTotal(r.total); setPage(pg)
    } finally { setLoading(false) }
  }, [keyword, platform, topic, synced])

  useEffect(() => { void fetchList(1) }, [fetchList])

  useEffect(() => {
    const loadTopics = async () => {
      try {
        const res = await workflowApi.listTopics()
        setTopicOptions(res.topics)
      } catch {
        // ignore
      }
    }
    void loadTopics()
  }, [])

  const handleSearch = () => setKeyword(inputKw.trim())

  const handleDeleteEmbedding = async (id: number) => {
    setDeletingIds(prev => new Set(prev).add(id))
    try {
      await adminRagApi.deleteEmbedding(id)
      message.success('已删除向量，下次同步时将重新生成')
      void fetchList(page)
    } catch { message.error('操作失败') }
    finally { setDeletingIds(prev => { const s = new Set(prev); s.delete(id); return s }) }
  }

  const handleOpenDetail = async (id: number) => {
    setDrawerOpen(true); setDetail(null); setDetailLoading(true)
    try {
      setDetail(await adminRagApi.getArticleDetail(id))
    } catch {
      message.error('获取详情失败，请确认 RAG 服务正在运行')
      setDrawerOpen(false)
    } finally { setDetailLoading(false) }
  }

  const handleChunkUpdated = (pk: string, snippet: string) =>
    setDetail(prev => prev ? {
      ...prev,
      chunks: prev.chunks.map(c => c.chunkPk === pk ? { ...c, snippet } : c),
    } : prev)

  const handleChunkDeleted = (pk: string) =>
    setDetail(prev => prev ? {
      ...prev,
      chunks: prev.chunks.filter(c => c.chunkPk !== pk),
    } : prev)

  return (
    <div className={ui.pageShell}>
      <PageHeader
        title="知识库文章管理"
        subtitle="查看与管理已向量化入库的舆情文章"
        icon={<DatabaseOutlined />}
        extra={
          <Button icon={<ReloadOutlined />} className={ui.ghostBtn} onClick={() => void fetchList(page)} loading={loading}>
            刷新
          </Button>
        }
      />

      <Card bordered={false} className={`${ui.panelCard} ${ui.toolbar}`}>
        <Space wrap>
          <Input placeholder="搜索标题/内容" allowClear prefix={<SearchOutlined />}
            style={{ width: 240 }} value={inputKw}
            onChange={e => setInputKw(e.target.value)} onPressEnter={handleSearch} />
          <Button onClick={handleSearch} type="primary">搜索</Button>
          <Select style={{ width: 140 }} placeholder="平台筛选" allowClear
            value={platform || undefined} onChange={v => setPlatform(v ?? '')}
            options={PLATFORMS.map((p) => ({ label: p.label, value: p.article }))}
          />
          <Select style={{ width: 140 }} placeholder="话题筛选" allowClear
            value={topic || undefined} onChange={v => setTopic(v ?? '')}
            options={topicOptions.map(t => ({ label: t, value: t }))}
          />
          <Select style={{ width: 140 }} placeholder="向量状态" allowClear
            value={synced || undefined} onChange={v => setSynced((v as 'yes' | 'no' | '') ?? '')}
            options={[{ label: '已向量化', value: 'yes' }, { label: '未向量化', value: 'no' }]}
          />
          <Text type="secondary" style={{ fontSize: 12 }}>共 {total} 条</Text>
        </Space>
      </Card>

      <Card bordered={false} className={`${ui.panelCard} ${ui.tableWrap}`}>
        <Table<RagKBArticle>
          size="small" rowKey="id" loading={loading} dataSource={list} scroll={{ x: 900 }}
          pagination={{
            current: page, total, pageSize: PAGE_SIZE, showSizeChanger: false,
            showTotal: t => `共 ${t} 条`, onChange: p => void fetchList(p),
          }}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 72 },
            {
              title: '标题', dataIndex: 'title', ellipsis: true,
              render: (t: string) => <Tooltip title={t}><span>{t || '-'}</span></Tooltip>,
            },
            { title: '平台', dataIndex: 'platform', width: 100 },
            { title: '话题', dataIndex: 'topic', width: 120, ellipsis: true },
            {
              title: '发布时间', dataIndex: 'publishedAt', width: 128,
              render: (t?: string) => t ? dayjs(t).format('MM-DD HH:mm') : '-',
            },
            {
              title: '向量状态', dataIndex: 'synced', width: 110,
              render: (v: boolean) => v
                ? <Badge status="success" text={<Tag color="green">已向量化</Tag>} />
                : <Badge status="default" text={<Tag>未同步</Tag>} />,
            },
            {
              title: '最近同步时间', dataIndex: 'embeddingSyncedAt', width: 148,
              render: (t?: string) => t ? dayjs(t).format('MM-DD HH:mm:ss') : '-',
            },
            {
              title: '操作', width: 160,
              render: (_, row) => (
                <Space size={4}>
                  <Button size="small" icon={<EyeOutlined />} onClick={() => void handleOpenDetail(row.id)}>
                    详情
                  </Button>
                  {row.synced && (
                    <Popconfirm title="删除此文章的向量？下次同步时将自动重建。"
                      onConfirm={() => void handleDeleteEmbedding(row.id)} okText="删除" cancelText="取消">
                      <Button size="small" danger icon={<DeleteOutlined />} loading={deletingIds.has(row.id)}>
                        删除向量
                      </Button>
                    </Popconfirm>
                  )}
                </Space>
              ),
            },
          ]}
        />
      </Card>

      <Drawer
        title={detail?.article.title || '文章详情'} width={720}
        open={drawerOpen} onClose={() => setDrawerOpen(false)} destroyOnClose
      >
        {detailLoading && <Spin style={{ display: 'block', margin: '80px auto' }} />}

        {!detailLoading && detail && (
          <Space direction="vertical" style={{ width: '100%' }} size={24}>
            <Descriptions size="small" bordered column={2}>
              <Descriptions.Item label="平台">{detail.article.platform ? platformLabel(detail.article.platform) : '-'}</Descriptions.Item>
              <Descriptions.Item label="作者">{detail.article.author || '-'}</Descriptions.Item>
              <Descriptions.Item label="发布时间">
                {detail.article.publishedAt ? dayjs(detail.article.publishedAt).format('YYYY-MM-DD HH:mm') : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="情感倾向">
                <Tag color={SENTIMENT_COLOR[detail.article.sentiment] ?? 'default'}>
                  {detail.article.sentiment || '-'}
                </Tag>
              </Descriptions.Item>
              {detail.article.aiTags && (
                <Descriptions.Item label="AI 标签" span={2}>
                  {(() => {
                    try { return (JSON.parse(detail.article.aiTags ?? '[]') as string[]).map(t => <Tag key={t}>{t}</Tag>) }
                    catch { return <Text type="secondary">{detail.article.aiTags}</Text> }
                  })()}
                </Descriptions.Item>
              )}
              <Descriptions.Item label="向量状态" span={2}>
                {detail.article.synced
                  ? <Tag color="green">已向量化（{dayjs(detail.article.embeddingSyncedAt).format('MM-DD HH:mm')}）</Tag>
                  : <Tag color="orange">待同步</Tag>}
              </Descriptions.Item>
              {detail.article.originUrl && (
                <Descriptions.Item label="原文链接" span={2}>
                  <a href={detail.article.originUrl} target="_blank" rel="noreferrer">
                    <LinkOutlined style={{ marginRight: 4 }} />查看原文
                  </a>
                </Descriptions.Item>
              )}
            </Descriptions>

            <div>
              <Space>
                <Text strong>向量切块（{detail.chunks.length} 个）</Text>
                <Text type="secondary" style={{ fontSize: 11 }}>编辑后立即重算向量；下次全量重同步会覆盖手动修改</Text>
              </Space>
              {detail.chunks.length === 0
                ? (
                  <div style={{ marginTop: 8, color: '#999', fontSize: 13 }}>
                    {detail.article.synced ? '暂无 chunk 数据' : 'RAG 服务未运行或尚未同步'}
                  </div>
                )
                : (
                  <List style={{ marginTop: 8, maxHeight: 400, overflowY: 'auto' }} size="small"
                    dataSource={detail.chunks}
                    renderItem={ck => (
                      <ChunkRow key={ck.chunkPk} chunk={ck}
                        onUpdated={handleChunkUpdated} onDeleted={handleChunkDeleted} />
                    )}
                  />
                )}
            </div>
          </Space>
        )}
      </Drawer>
    </div>
  )
}

export default RagKBPage

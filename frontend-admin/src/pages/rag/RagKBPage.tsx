import React, { useCallback, useEffect, useState } from 'react'
import {
  Badge,
  Button,
  Card,
  Input,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import { DatabaseOutlined, DeleteOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { adminRagApi } from '@/api/admin-rag'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'
import type { RagKBArticle } from '@/types'
import dayjs from 'dayjs'

const { Text } = Typography

const RagKBPage: React.FC = () => {
  const [list, setList] = useState<RagKBArticle[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [keyword, setKeyword] = useState('')
  const [inputKw, setInputKw] = useState('')
  const [platform, setPlatform] = useState('')
  const [synced, setSynced] = useState<'yes' | 'no' | ''>('')
  const [deletingIds, setDeletingIds] = useState<Set<number>>(new Set())

  const PAGE_SIZE = 20

  const fetchList = useCallback(async (pg: number) => {
    setLoading(true)
    try {
      const r = await adminRagApi.listArticles({
        page: pg,
        page_size: PAGE_SIZE,
        keyword: keyword || undefined,
        platform: platform || undefined,
        synced: synced || undefined,
      })
      setList(r.list)
      setTotal(r.total)
      setPage(pg)
    } finally {
      setLoading(false)
    }
  }, [keyword, platform, synced])

  useEffect(() => { void fetchList(1) }, [fetchList])

  const handleSearch = () => {
    setKeyword(inputKw.trim())
  }

  const handleDeleteEmbedding = async (id: number) => {
    setDeletingIds(prev => new Set(prev).add(id))
    try {
      await adminRagApi.deleteEmbedding(id)
      message.success('已删除向量，下次同步时将重新生成')
      void fetchList(page)
    } catch {
      message.error('操作失败')
    } finally {
      setDeletingIds(prev => { const s = new Set(prev); s.delete(id); return s })
    }
  }

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
        <Input
          placeholder="搜索标题/内容"
          allowClear
          prefix={<SearchOutlined />}
          style={{ width: 240 }}
          value={inputKw}
          onChange={e => setInputKw(e.target.value)}
          onPressEnter={handleSearch}
        />
        <Button onClick={handleSearch} type="primary">搜索</Button>
        <Select
          style={{ width: 140 }}
          placeholder="平台筛选"
          allowClear
          value={platform || undefined}
          onChange={v => setPlatform(v ?? '')}
          options={[
            { label: 'weibo', value: 'weibo' },
            { label: 'xhs', value: 'xhs' },
            { label: 'zhihu', value: 'zhihu' },
            { label: 'bilibili', value: 'bilibili' },
            { label: 'douyin', value: 'douyin' },
            { label: 'tieba', value: 'tieba' },
          ]}
        />
        <Select
          style={{ width: 140 }}
          placeholder="向量状态"
          allowClear
          value={synced || undefined}
          onChange={v => setSynced((v as 'yes' | 'no' | '') ?? '')}
          options={[
            { label: '已向量化', value: 'yes' },
            { label: '未向量化', value: 'no' },
          ]}
        />
        <Text type="secondary" style={{ fontSize: 12 }}>共 {total} 条</Text>
      </Space>
      </Card>

      <Card bordered={false} className={`${ui.panelCard} ${ui.tableWrap}`}>
      <Table<RagKBArticle>
        size="small"
        rowKey="id"
        loading={loading}
        dataSource={list}
        scroll={{ x: 900 }}
        pagination={{
          current: page,
          total,
          pageSize: PAGE_SIZE,
          showSizeChanger: false,
          showTotal: t => `共 ${t} 条`,
          onChange: p => void fetchList(p),
        }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 72 },
          {
            title: '标题',
            dataIndex: 'title',
            ellipsis: true,
            render: (t: string) => (
              <Tooltip title={t}>
                <span>{t || '-'}</span>
              </Tooltip>
            ),
          },
          { title: '平台', dataIndex: 'platform', width: 100 },
          {
            title: '发布时间',
            dataIndex: 'publishedAt',
            width: 128,
            render: (t?: string) => t ? dayjs(t).format('MM-DD HH:mm') : '-',
          },
          {
            title: '向量状态',
            dataIndex: 'synced',
            width: 110,
            render: (v: boolean) =>
              v ? (
                <Badge status="success" text={<Tag color="green">已向量化</Tag>} />
              ) : (
                <Badge status="default" text={<Tag>未同步</Tag>} />
              ),
          },
          {
            title: '最近同步时间',
            dataIndex: 'embeddingSyncedAt',
            width: 148,
            render: (t?: string) => t ? dayjs(t).format('MM-DD HH:mm:ss') : '-',
          },
          {
            title: '操作',
            width: 100,
            render: (_, row) =>
              row.synced ? (
                <Popconfirm
                  title="删除此文章的向量？下次同步时将自动重建。"
                  onConfirm={() => void handleDeleteEmbedding(row.id)}
                  okText="删除"
                  cancelText="取消"
                >
                  <Button
                    size="small"
                    danger
                    icon={<DeleteOutlined />}
                    loading={deletingIds.has(row.id)}
                  >
                    删除向量
                  </Button>
                </Popconfirm>
              ) : null,
          },
        ]}
      />
      </Card>
    </div>
  )
}

export default RagKBPage

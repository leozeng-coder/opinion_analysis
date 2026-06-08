import React, { useCallback, useEffect, useState } from 'react'
import {
  Card, Table, Tag, Space, Select, DatePicker, Button, Typography, Avatar, Image, message, Modal, Descriptions, List,
} from 'antd'
import {
  ReloadOutlined, UserOutlined, LikeOutlined, CommentOutlined, ShareAltOutlined,
  EyeOutlined, StarOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { platformDataApi, type PlatformDataItem } from '@/api/platform-data'
import { platformCommentApi, type PlatformCommentItem } from '@/api/platform-comment'

const { Text, Link } = Typography
const { RangePicker } = DatePicker

// 解析图片 URL（处理 JSON 数组或单个 URL）
const parseImageUrl = (coverUrl?: string): string | undefined => {
  if (!coverUrl) return undefined
  try {
    const parsed = JSON.parse(coverUrl)
    if (Array.isArray(parsed) && parsed.length > 0) {
      return parsed[0]
    }
  } catch {
    // 不是 JSON，直接返回
  }
  return coverUrl
}

// 解析所有图片 URL
const parseAllImageUrls = (coverUrl?: string): string[] => {
  if (!coverUrl) return []
  try {
    const parsed = JSON.parse(coverUrl)
    if (Array.isArray(parsed)) {
      return parsed
    }
  } catch {
    // 不是 JSON，返回单个 URL
  }
  return [coverUrl]
}

// 平台选项
const PLATFORM_OPTIONS = [
  { label: '小红书', value: 'xhs' },
  { label: '抖音', value: 'dy' },
  { label: 'B站', value: 'bili' },
  { label: '微博', value: 'wb' },
  { label: '快手', value: 'ks' },
  { label: '贴吧', value: 'tieba' },
  { label: '知乎', value: 'zhihu' },
]

// 平台颜色映射
const PLATFORM_COLORS: Record<string, string> = {
  xhs: 'red',
  dy: 'blue',
  bili: 'cyan',
  wb: 'orange',
  ks: 'orange',
  tieba: 'purple',
  zhihu: 'blue',
}

const getCommentStatLabel = (platform: string) => (platform === 'tieba' ? '回复' : '评论')

const PlatformDataPage: React.FC = () => {
  const [list, setList] = useState<PlatformDataItem[]>([])
  const [loading, setLoading] = useState(false)
  const [platform, setPlatform] = useState('zhihu')
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs] | null>(null)
  const [pagination, setPagination] = useState({ current: 1, pageSize: 20, total: 0 })
  const [detailVisible, setDetailVisible] = useState(false)
  const [selectedItem, setSelectedItem] = useState<PlatformDataItem | null>(null)
  const [comments, setComments] = useState<PlatformCommentItem[]>([])
  const [commentsLoading, setCommentsLoading] = useState(false)
  const [commentsPagination, setCommentsPagination] = useState({ current: 1, pageSize: 10, total: 0 })

  const fetchData = useCallback(async () => {
    setLoading(true)
    try {
      const params = {
        platform,
        startDate: dateRange?.[0]?.format('YYYY-MM-DD'),
        endDate: dateRange?.[1]?.format('YYYY-MM-DD'),
        page: pagination.current,
        pageSize: pagination.pageSize,
      }
      const res = await platformDataApi.list(params)
      setList(res.data)
      // 用函数式更新，只改 total，不覆盖用户可能已翻到的新页码
      setPagination(prev => ({ ...prev, total: res.total }))
    } catch (error) {
      void message.error('加载数据失败')
    } finally {
      setLoading(false)
    }
  }, [platform, dateRange, pagination.current, pagination.pageSize])

  useEffect(() => {
    void fetchData()
  }, [fetchData])

  const handleTableChange = (newPagination: any) => {
    setPagination({
      current: newPagination.current,
      pageSize: newPagination.pageSize,
      total: pagination.total,
    })
  }

  const handleReset = () => {
    setPlatform('zhihu')
    setDateRange(null)
    setPagination({ current: 1, pageSize: 20, total: 0 })
  }

  const handleViewDetail = (item: PlatformDataItem) => {
    setSelectedItem(item)
    setDetailVisible(true)
    setCommentsPagination({ current: 1, pageSize: 10, total: 0 })
    void fetchComments(item.id, item.platform, 1, 10)
  }

  const fetchComments = useCallback(async (itemId: number, platform: string, page: number, pageSize: number) => {
    setCommentsLoading(true)
    try {
      const res = await platformCommentApi.list({ platform, itemId, page, pageSize })
      setComments(res.data || [])
      setCommentsPagination({ current: page, pageSize, total: res.total || 0 })
    } catch (error) {
      void message.error('加载评论失败')
      setComments([])
      setCommentsPagination({ current: 1, pageSize: 10, total: 0 })
    } finally {
      setCommentsLoading(false)
    }
  }, [])

  const handleCommentsPageChange = (page: number, pageSize: number) => {
    if (selectedItem) {
      void fetchComments(selectedItem.id, selectedItem.platform, page, pageSize)
    }
  }

  const columns: ColumnsType<PlatformDataItem> = [
    {
      title: '平台',
      dataIndex: 'platform',
      width: 80,
      fixed: 'left',
      render: (p: string) => (
        <Tag color={PLATFORM_COLORS[p] || 'default'}>
          {PLATFORM_OPTIONS.find(opt => opt.value === p)?.label || p}
        </Tag>
      ),
    },
    {
      title: '作者',
      dataIndex: 'author',
      width: 150,
      render: (author: string, record) => (
        <Space size={8}>
          {record.avatar ? (
            <Avatar size="small" src={record.avatar} />
          ) : (
            <Avatar size="small" icon={<UserOutlined />} />
          )}
          <Text ellipsis style={{ maxWidth: 100 }}>{author}</Text>
        </Space>
      ),
    },
    {
      title: '标题/内容',
      dataIndex: 'title',
      ellipsis: true,
      render: (title: string, record) => (
        <Space direction="vertical" size={4} style={{ width: '100%' }}>
          {title && <Text strong ellipsis>{title}</Text>}
          <Text type="secondary" ellipsis style={{ fontSize: 12 }}>
            {record.content}
          </Text>
          {record.coverUrl && parseImageUrl(record.coverUrl) && (
            <Image
              src={parseImageUrl(record.coverUrl)}
              alt="cover"
              width={80}
              height={80}
              style={{ objectFit: 'cover', borderRadius: 4 }}
              preview={{ mask: '预览' }}
              fallback="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mN8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="
            />
          )}
        </Space>
      ),
    },
    {
      title: '互动数据',
      key: 'stats',
      width: 200,
      render: (_, record) => (
        <Space direction="vertical" size={2}>
          {record.likeCount !== undefined && (
            <Space size={4}>
              <LikeOutlined style={{ color: '#ff4d4f' }} />
              <Text type="secondary" style={{ fontSize: 12 }}>{record.likeCount}</Text>
            </Space>
          )}
          {record.commentCount !== undefined && (
            <Space size={4}>
              <CommentOutlined style={{ color: '#1890ff' }} />
              <Text type="secondary" style={{ fontSize: 12 }}>
                {getCommentStatLabel(record.platform)} {record.commentCount}
              </Text>
            </Space>
          )}
          {record.shareCount !== undefined && (
            <Space size={4}>
              <ShareAltOutlined style={{ color: '#52c41a' }} />
              <Text type="secondary" style={{ fontSize: 12 }}>{record.shareCount}</Text>
            </Space>
          )}
          {record.viewCount !== undefined && (
            <Space size={4}>
              <EyeOutlined style={{ color: '#722ed1' }} />
              <Text type="secondary" style={{ fontSize: 12 }}>{record.viewCount}</Text>
            </Space>
          )}
          {record.collectCount !== undefined && (
            <Space size={4}>
              <StarOutlined style={{ color: '#faad14' }} />
              <Text type="secondary" style={{ fontSize: 12 }}>{record.collectCount}</Text>
            </Space>
          )}
        </Space>
      ),
    },
    {
      title: 'IP属地',
      dataIndex: 'ipLocation',
      width: 100,
      render: (ip: string) => ip ? <Text type="secondary">{ip}</Text> : '—',
    },
    {
      title: '发布时间',
      dataIndex: 'publishTime',
      width: 150,
      render: (t: string | null) => t ? dayjs(t).format('YYYY-MM-DD HH:mm') : '—',
    },
    {
      title: '操作',
      key: 'ops',
      width: 150,
      fixed: 'right',
      render: (_, record) => (
        <Space>
          <Button type="link" size="small" onClick={() => handleViewDetail(record)}>
            详情
          </Button>
          <Link href={record.url} target="_blank">原文</Link>
        </Space>
      ),
    },
  ]

  return (
    <div style={{ height: 'calc(100vh - 96px)', display: 'flex', flexDirection: 'column', overflow: 'hidden', padding: '0 0 0 0' }}>
      <Card
        title="平台数据"
        style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', margin: 0 }}
        styles={{ body: { flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', padding: '16px 24px 0' } }}
      >
        <Space size={12} wrap style={{ marginBottom: 12, flexShrink: 0 }}>
          <Select
            value={platform}
            onChange={(val) => { setPlatform(val); setPagination(prev => ({ ...prev, current: 1 })) }}
            options={PLATFORM_OPTIONS}
            style={{ width: 140 }}
            allowClear={false}
          />
          <RangePicker
            value={dateRange}
            onChange={(dates) => setDateRange(dates as [dayjs.Dayjs, dayjs.Dayjs] | null)}
            format="YYYY-MM-DD"
            placeholder={['开始日期', '结束日期']}
          />
          <Button icon={<ReloadOutlined />} onClick={() => void fetchData()}>
            刷新
          </Button>
          <Button onClick={handleReset}>重置</Button>
        </Space>

        <Table
          columns={columns}
          dataSource={list}
          loading={loading}
          rowKey={(record) => `${record.platform}-${record.id}`}
          pagination={{
            current: pagination.current,
            pageSize: pagination.pageSize,
            total: pagination.total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total) => `共 ${total} 条`,
          }}
          onChange={handleTableChange}
          scroll={{ x: 1200, y: 'calc(100vh - 340px)' }}
          style={{ flex: 1 }}
        />
      </Card>

      <Modal
        title="内容详情"
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={[
          <Button key="close" onClick={() => setDetailVisible(false)}>
            关闭
          </Button>,
          <Button key="view" type="primary" href={selectedItem?.url} target="_blank">
            查看原文
          </Button>,
        ]}
        width={900}
      >
        {selectedItem && (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Descriptions column={2} bordered size="small">
              <Descriptions.Item label="平台" span={1}>
                <Tag color={PLATFORM_COLORS[selectedItem.platform] || 'default'}>
                  {PLATFORM_OPTIONS.find(opt => opt.value === selectedItem.platform)?.label || selectedItem.platform}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="发布时间" span={1}>
                {selectedItem.publishTime ? dayjs(selectedItem.publishTime).format('YYYY-MM-DD HH:mm:ss') : '—'}
              </Descriptions.Item>
              <Descriptions.Item label="作者" span={2}>
                <Space>
                  {selectedItem.avatar ? (
                    <Avatar src={selectedItem.avatar} />
                  ) : (
                    <Avatar icon={<UserOutlined />} />
                  )}
                  <Text>{selectedItem.author}</Text>
                </Space>
              </Descriptions.Item>
              {selectedItem.ipLocation && (
                <Descriptions.Item label="IP属地" span={2}>
                  {selectedItem.ipLocation}
                </Descriptions.Item>
              )}
              {selectedItem.title && (
                <Descriptions.Item label="标题" span={2}>
                  <Text strong>{selectedItem.title}</Text>
                </Descriptions.Item>
              )}
              <Descriptions.Item label="内容" span={2}>
                <div style={{ maxHeight: 200, overflow: 'auto', whiteSpace: 'pre-wrap' }}>
                  {selectedItem.content}
                </div>
              </Descriptions.Item>
              <Descriptions.Item label="互动数据" span={2}>
                <Space size={16} wrap>
                  {selectedItem.likeCount !== undefined && (
                    <Space size={4}>
                      <LikeOutlined style={{ color: '#ff4d4f' }} />
                      <Text>点赞 {selectedItem.likeCount}</Text>
                    </Space>
                  )}
                  {selectedItem.commentCount !== undefined && (
                    <Space size={4}>
                      <CommentOutlined style={{ color: '#1890ff' }} />
                      <Text>{getCommentStatLabel(selectedItem.platform)} {selectedItem.commentCount}</Text>
                    </Space>
                  )}
                  {selectedItem.shareCount !== undefined && (
                    <Space size={4}>
                      <ShareAltOutlined style={{ color: '#52c41a' }} />
                      <Text>分享 {selectedItem.shareCount}</Text>
                    </Space>
                  )}
                  {selectedItem.viewCount !== undefined && (
                    <Space size={4}>
                      <EyeOutlined style={{ color: '#722ed1' }} />
                      <Text>浏览 {selectedItem.viewCount}</Text>
                    </Space>
                  )}
                  {selectedItem.collectCount !== undefined && (
                    <Space size={4}>
                      <StarOutlined style={{ color: '#faad14' }} />
                      <Text>收藏 {selectedItem.collectCount}</Text>
                    </Space>
                  )}
                </Space>
              </Descriptions.Item>
            </Descriptions>

            {selectedItem.coverUrl && parseAllImageUrls(selectedItem.coverUrl).length > 0 && (
              <div>
                <Text strong style={{ marginBottom: 8, display: 'block' }}>图片</Text>
                <Image.PreviewGroup>
                  <Space wrap>
                    {parseAllImageUrls(selectedItem.coverUrl).map((url, idx) => (
                      <Image
                        key={idx}
                        src={url}
                        alt={`image-${idx}`}
                        width={120}
                        height={120}
                        style={{ objectFit: 'cover', borderRadius: 4 }}
                        fallback="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mN8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="
                      />
                    ))}
                  </Space>
                </Image.PreviewGroup>
              </div>
            )}

            <div>
              <Text strong style={{ marginBottom: 8, display: 'block' }}>
                {selectedItem.platform === 'tieba' ? '回复' : '评论'} ({commentsPagination.total})
              </Text>
              <List
                loading={commentsLoading}
                dataSource={comments}
                locale={{ emptyText: '暂无评论' }}
                pagination={{
                  current: commentsPagination.current,
                  pageSize: commentsPagination.pageSize,
                  total: commentsPagination.total,
                  onChange: handleCommentsPageChange,
                  showSizeChanger: false,
                  size: 'small',
                }}
                renderItem={(comment) => (
                  <List.Item key={comment.id}>
                    <List.Item.Meta
                      avatar={
                        comment.avatar ? (
                          <Avatar src={comment.avatar} />
                        ) : (
                          <Avatar icon={<UserOutlined />} />
                        )
                      }
                      title={
                        <Space>
                          <Text strong>{comment.nickname}</Text>
                          {comment.ipLocation && (
                            <Text type="secondary" style={{ fontSize: 12 }}>
                              {comment.ipLocation}
                            </Text>
                          )}
                          {comment.createTime && (
                            <Text type="secondary" style={{ fontSize: 12 }}>
                              {dayjs(comment.createTime).format('YYYY-MM-DD HH:mm')}
                            </Text>
                          )}
                        </Space>
                      }
                      description={
                        <div>
                          <div style={{ marginBottom: 8, whiteSpace: 'pre-wrap' }}>
                            {comment.content}
                          </div>
                          <Space size={16}>
                            {comment.likeCount !== undefined && (
                              <Space size={4}>
                                <LikeOutlined style={{ fontSize: 12 }} />
                                <Text type="secondary" style={{ fontSize: 12 }}>
                                  {comment.likeCount}
                                </Text>
                              </Space>
                            )}
                            {comment.subCommentCount !== undefined && comment.subCommentCount > 0 && (
                              <Space size={4}>
                                <CommentOutlined style={{ fontSize: 12 }} />
                                <Text type="secondary" style={{ fontSize: 12 }}>
                                  {comment.subCommentCount}
                                </Text>
                              </Space>
                            )}
                          </Space>
                        </div>
                      }
                    />
                  </List.Item>
                )}
              />
            </div>
          </Space>
        )}
      </Modal>
    </div>
  )
}

export default PlatformDataPage

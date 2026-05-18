import React, { useCallback, useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Drawer,
  Empty,
  Input,
  Space,
  Spin,
  Typography,
} from 'antd'
import { SendOutlined, DeleteOutlined } from '@ant-design/icons'
import { useLocation } from 'react-router-dom'
import { postAiChat, type AiChatTurn } from '@/api/aiChat'
import styles from './AiChatDrawer.module.css'

const { Text, Paragraph } = Typography

function buildPageHint(pathname: string): string {
  const map: Record<string, string> = {
    '/': '首页/仪表盘入口',
    '/dashboard': '概览仪表盘：关注整体舆情指标与趋势',
    '/opinion': '舆情数据列表：浏览、筛选单条舆情',
    '/topics': '热点话题：查看话题聚合与关键词',
    '/alerts': '预警中心：规则与预警记录',
    '/stats': '统计分析：图表与统计视图',
    '/crawler': '爬虫调度：任务运行与进度',
  }
  return map[pathname] ?? `当前路径 ${pathname}`
}

const WELCOME =
  '你好，我是舆情分析助手。我可以帮你解读页面功能、梳理分析思路、概括趋势关注点。' +
  '我不会凭空编造数据库里的具体数字；需要明细时请在本系统各页面查看或告诉我你已掌握的数据。'

type Props = {
  open: boolean
  onClose: () => void
}

const AiChatDrawer: React.FC<Props> = ({ open, onClose }) => {
  const { pathname } = useLocation()
  const pageHint = useMemo(() => buildPageHint(pathname), [pathname])

  const [messages, setMessages] = useState<AiChatTurn[]>([])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)

  const handleClear = useCallback(() => {
    setMessages([])
    setInput('')
  }, [])

  const handleSend = useCallback(async () => {
    const text = input.trim()
    if (!text || loading) return

    const userMsg: AiChatTurn = { role: 'user', content: text }
    const nextHist = [...messages, userMsg]
    setMessages(nextHist)
    setInput('')
    setLoading(true)

    try {
      const res = await postAiChat({
        messages: nextHist,
        pageHint,
      })
      setMessages((prev) => [...prev, { role: 'assistant', content: res.reply || '（空回复）' }])
    } catch {
      setMessages((prev) => {
        if (prev.length === 0) return prev
        const last = prev[prev.length - 1]
        if (last.role === 'user' && last.content === text) {
          return prev.slice(0, -1)
        }
        return prev
      })
    } finally {
      setLoading(false)
    }
  }, [input, loading, messages, pageHint])

  return (
    <Drawer
      title="智能助手"
      placement="right"
      width={420}
      onClose={onClose}
      open={open}
      destroyOnClose={false}
      extra={
        <Button type="text" icon={<DeleteOutlined />} onClick={handleClear}>
          清空
        </Button>
      }
    >
      <Alert
        type="info"
        showIcon
        message="上下文"
        description={<Text type="secondary">{pageHint}</Text>}
        style={{ marginBottom: 12 }}
      />

      <div className={styles.body}>
        {messages.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={<Paragraph type="secondary">{WELCOME}</Paragraph>} />
        ) : (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            {messages.map((m, i) => (
              <div
                key={i}
                className={
                  m.role === 'user' ? `${styles.bubble} ${styles.user}` : `${styles.bubble} ${styles.assistant}`
                }
              >
                <Text strong>{m.role === 'user' ? '你' : '助手'}</Text>
                <Paragraph style={{ marginBottom: 0, marginTop: 4, whiteSpace: 'pre-wrap' }}>{m.content}</Paragraph>
              </div>
            ))}
            {loading && (
              <div className={`${styles.bubble} ${styles.assistant}`}>
                <Spin size="small" /> <Text type="secondary">正在思考…</Text>
              </div>
            )}
          </Space>
        )}
      </div>

      <div className={styles.footer}>
        <Space direction="vertical" style={{ width: '100%' }} size="small">
          <Input.TextArea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e: React.KeyboardEvent<HTMLTextAreaElement>) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                void handleSend()
              }
            }}
            disabled={loading}
            placeholder="输入问题，Enter 发送，Shift+Enter 换行"
            autoSize={{ minRows: 3, maxRows: 6 }}
          />
          <Button
            type="primary"
            icon={<SendOutlined />}
            onClick={() => void handleSend()}
            disabled={loading || !input.trim()}
            block
          >
            发送
          </Button>
        </Space>
      </div>
    </Drawer>
  )
}

export default AiChatDrawer

import React, {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'
import { Button, Input, Typography, Spin } from 'antd'
import {
  DeleteOutlined,
  SendOutlined,
  VerticalAlignBottomOutlined,
} from '@ant-design/icons'
import { postAiChat, type AiChatTurn } from '@/api/aiChat'
import styles from './AiAssistantPage.module.css'

const { Text } = Typography

const STORAGE_KEY = 'opinion_frontend_ai_chat_v1'

const WELCOME_SHORT =
  '你好，我是舆情分析助手，可协助解读趋势、梳理分析思路、概括关注点。'

const WELCOME_DETAIL =
  '我不会凭空编造你系统里没有的数据；需要具体数字时请在各业务页查看或粘贴你掌握的信息。'

const HERO_HEADING = '有什么我能帮你的吗？'

function loadStoredMessages(): AiChatTurn[] {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return []
    return parsed.filter(
      (m): m is AiChatTurn =>
        m &&
        typeof m === 'object' &&
        (m as AiChatTurn).role &&
        typeof (m as AiChatTurn).content === 'string'
    )
  } catch {
    return []
  }
}

function saveMessages(msgs: AiChatTurn[]) {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(msgs))
  } catch {
    /* ignore quota */
  }
}

function sessionTitle(messages: AiChatTurn[]): string {
  const first = messages.find((m) => m.role === 'user')
  if (!first) return '舆情分析助手'
  const t = first.content.trim().replace(/\s+/g, ' ')
  if (t.length <= 36) return t
  return `${t.slice(0, 36)}…`
}

const AiAssistantPage: React.FC = () => {
  const [messages, setMessages] = useState<AiChatTurn[]>(loadStoredMessages)
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [showScrollFab, setShowScrollFab] = useState(false)
  const threadRef = useRef<HTMLElement>(null)

  useEffect(() => {
    saveMessages(messages)
  }, [messages])

  const scrollThreadToBottom = useCallback((behavior: ScrollBehavior = 'auto') => {
    const el = threadRef.current
    if (!el) return
    el.scrollTo({ top: el.scrollHeight, behavior })
  }, [])

  useLayoutEffect(() => {
    scrollThreadToBottom('auto')
  }, [messages, loading, scrollThreadToBottom])

  const onThreadScroll = useCallback(() => {
    const el = threadRef.current
    if (!el) return
    const dist = el.scrollHeight - el.scrollTop - el.clientHeight
    setShowScrollFab(dist > 120)
  }, [])

  useEffect(() => {
    const el = threadRef.current
    if (!el) return
    onThreadScroll()
    el.addEventListener('scroll', onThreadScroll, { passive: true })
    return () => el.removeEventListener('scroll', onThreadScroll)
  }, [onThreadScroll, messages.length])

  const handleClear = useCallback(() => {
    setMessages([])
    setInput('')
    try {
      sessionStorage.removeItem(STORAGE_KEY)
    } catch {
      /* ignore */
    }
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
      })
      setMessages((prev) => [
        ...prev,
        { role: 'assistant', content: res.reply || '（空回复）' },
      ])
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
  }, [input, loading, messages])

  const title = sessionTitle(messages)

  return (
    <div className={styles.page}>
      <header className={styles.topBar}>
        <div className={styles.topBarInner}>
          <div className={styles.topBarSide}>
            <span className={styles.cornerMuted}>智能助手</span>
          </div>
          <h1 className={styles.sessionTitle}>{title}</h1>
          <div className={`${styles.topBarSide} ${styles.topBarSideEnd}`}>
            <Button
              type="text"
              className={styles.clearBtn}
              icon={<DeleteOutlined />}
              onClick={handleClear}
              disabled={messages.length === 0 && !input.trim()}
            >
              清空
            </Button>
          </div>
        </div>
      </header>

      <main
        className={
          messages.length === 0 && !loading
            ? `${styles.doc} ${styles.docEmpty}`
            : styles.doc
        }
        ref={threadRef}
      >
        {messages.length === 0 ? (
          <div className={styles.heroEmpty}>
            <h2 className={styles.heroTitle}>{HERO_HEADING}</h2>
            <p className={styles.heroLead}>{WELCOME_SHORT}</p>
            <p className={styles.heroMuted}>{WELCOME_DETAIL}</p>
          </div>
        ) : (
          messages.map((m, i) => {
            const isUser = m.role === 'user'
            return (
              <article
                key={`${m.role}-${i}-${m.content.slice(0, 32)}`}
                className={styles.block}
              >
                {isUser ? (
                  <div className={styles.userBlock}>
                    <div className={styles.userContent}>{m.content}</div>
                  </div>
                ) : (
                  <div className={styles.assistantBlock}>
                    <p className={styles.assistantProse}>{m.content}</p>
                  </div>
                )}
              </article>
            )
          })
        )}
        {loading && (
          <div className={styles.block}>
            <div className={styles.thinkingRow}>
              <Spin size="small" />
              <span>正在生成回复…</span>
            </div>
          </div>
        )}
      </main>

      <div className={styles.composerDock}>
        <div className={styles.composerInner}>
          {showScrollFab && messages.length > 0 && (
            <div
              style={{
                display: 'flex',
                justifyContent: 'center',
                marginBottom: 8,
              }}
            >
              <Button
                type="default"
                shape="circle"
                className={styles.scrollDown}
                icon={<VerticalAlignBottomOutlined />}
                onClick={() => scrollThreadToBottom('smooth')}
                aria-label="滚动到底部"
              />
            </div>
          )}
          <div className={styles.composerPill}>
            <Input.TextArea
              value={input}
              variant="borderless"
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e: React.KeyboardEvent<HTMLTextAreaElement>) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  void handleSend()
                }
              }}
              disabled={loading}
              placeholder={
                messages.length === 0
                  ? '发消息，Enter 发送 · Shift+Enter 换行'
                  : '继续对话，Enter 发送 · Shift+Enter 换行'
              }
              autoSize={{ minRows: 1, maxRows: 6 }}
              style={{ flex: 1 }}
            />
            <Button
              type="primary"
              shape="circle"
              className={styles.sendCircle}
              icon={<SendOutlined />}
              onClick={() => void handleSend()}
              disabled={loading || !input.trim()}
              aria-label="发送"
            />
          </div>
          <Text type="secondary" className={styles.composerMeta}>
            对话仅在当前标签页暂存 · 关闭或清空后丢失
          </Text>
        </div>
      </div>
    </div>
  )
}

export default AiAssistantPage

import React, {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'
import {
  Button,
  Input,
  Typography,
  Spin,
  message,
  Modal,
  Form,
  Select,
} from 'antd'
import {
  DeleteOutlined,
  SendOutlined,
  VerticalAlignBottomOutlined,
  PlusOutlined,
  EditOutlined,
  CopyOutlined,
  RedoOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  chatSessionApi,
  type ChatSession,
  type ChatMessage,
} from '@/api/chatSession'
import { workflowApi } from '@/api/workflow'
import page from '@/styles/page.module.css'
import styles from './AiAssistantPage.module.css'

const { Text } = Typography

const WELCOME_SHORT =
  '你好，我是舆情分析助手，可协助解读趋势、梳理分析思路、概括关注点。'

const WELCOME_DETAIL =
  '我不会凭空编造你系统里没有的数据；需要具体数字时请在各业务页查看或粘贴你掌握的信息。'

const HERO_HEADING = '有什么我能帮你的吗？'

function normalizeMessages(rows: ChatMessage[]): ChatMessage[] {
  return (rows ?? []).map((m) => ({
    ...m,
    role:
      String(m.role).toLowerCase() === 'assistant' ? 'assistant' : 'user',
  }))
}

const AiAssistantPage: React.FC = () => {
  const [sessions, setSessions] = useState<ChatSession[]>([])
  const [currentSessionId, setCurrentSessionId] = useState<number | null>(null)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [loadingSessions, setLoadingSessions] = useState(true)
  const [creating, setCreating] = useState(false)
  const [showScrollFab, setShowScrollFab] = useState(false)
  const [topicOptions, setTopicOptions] = useState<string[]>([])
  const [selectedTopics, setSelectedTopics] = useState<string[]>([])
  const [loadingTopics, setLoadingTopics] = useState(false)
  const [copiedId, setCopiedId] = useState<number | null>(null)
  const [loadingSessionId, setLoadingSessionId] = useState<number | null>(null)
  const threadRef = useRef<HTMLElement>(null)
  const sessionMessagesRef = useRef<Map<number, ChatMessage[]>>(new Map())
  const abortControllerRef = useRef<AbortController | null>(null)
  const [renameForm] = Form.useForm<{ title: string }>()
  const [renameTarget, setRenameTarget] = useState<{
    id: number
    title: string
  } | null>(null)

  const loadSessions = useCallback(async () => {
    try {
      const res = await chatSessionApi.list()
      setSessions(res.list ?? [])
      // 确保初始化时清除加载状态
      setLoadingSessionId(null)
    } catch {
      message.error('加载会话列表失败')
      setLoadingSessionId(null)
    } finally {
      setLoadingSessions(false)
    }
  }, [])

  useEffect(() => {
    void loadSessions()
    void loadTopics()
  }, [loadSessions])

  const loadTopics = useCallback(async () => {
    setLoadingTopics(true)
    try {
      const res = await workflowApi.listTopics()
      setTopicOptions(res.topics)
    } catch {
      message.error('加载话题列表失败')
    } finally {
      setLoadingTopics(false)
    }
  }, [])

  const loadSession = useCallback(async (id: number) => {
    try {
      // 先从缓存加载
      const cached = sessionMessagesRef.current.get(id)
      if (cached) {
        setMessages(cached)
        setCurrentSessionId(id)
        setInput('')
      } else {
        // 缓存没有则从服务器加载
        const res = await chatSessionApi.get(id)
        const normalized = normalizeMessages(res.messages ?? [])
        sessionMessagesRef.current.set(id, normalized)
        setMessages(normalized)
        setCurrentSessionId(id)
        setInput('')
      }
    } catch {
      message.error('加载会话失败')
    }
  }, [])

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

  const handleNewChat = useCallback(async () => {
    setCreating(true)
    try {
      const res = await chatSessionApi.create()
      const session = res.session
      setSessions((prev) => [session, ...prev.filter((s) => s.id !== session.id)])
      setCurrentSessionId(session.id)
      setMessages([])
      setInput('')
    } catch {
      message.error('创建会话失败')
    } finally {
      setCreating(false)
    }
  }, [])

  const openRename = useCallback(
    (s: ChatSession, e: React.MouseEvent) => {
      e.stopPropagation()
      setRenameTarget({ id: s.id, title: s.title })
      renameForm.setFieldsValue({ title: s.title })
    },
    [renameForm]
  )

  const submitRename = useCallback(async () => {
    if (!renameTarget) return
    try {
      const v = await renameForm.validateFields()
      const title = v.title.trim()
      if (!title) {
        message.warning('请输入标题')
        return
      }
      await chatSessionApi.rename(renameTarget.id, title)
      setSessions((prev) =>
        prev.map((s) => (s.id === renameTarget.id ? { ...s, title } : s))
      )
      setRenameTarget(null)
      message.success('已重命名')
    } catch {
      /* validate */
    }
  }, [renameForm, renameTarget])

  const handleDeleteSession = useCallback(
    (id: number, e: React.MouseEvent) => {
      e.stopPropagation()
      Modal.confirm({
        title: '删除会话？',
        content: '本地与服务器记录将一并删除。',
        okText: '删除',
        okType: 'danger',
        cancelText: '取消',
        onOk: async () => {
          try {
            await chatSessionApi.delete(id)
            setSessions((prev) => prev.filter((s) => s.id !== id))
            if (currentSessionId === id) {
              setCurrentSessionId(null)
              setMessages([])
              setInput('')
            }
          } catch {
            message.error('删除失败')
          }
        },
      })
    },
    [currentSessionId]
  )

  const handleSend = useCallback(async () => {
    const text = input.trim()
    if (!text || loading) return

    setInput('')
    setLoading(true)

    // 创建 AbortController
    const controller = new AbortController()
    abortControllerRef.current = controller

    // 添加用户消息到界面
    const userMsg: ChatMessage = {
      id: Date.now(),
      sessionId: currentSessionId ?? 0,
      role: 'user',
      content: text,
      createdAt: new Date().toISOString(),
    }
    setMessages((prev) => [...prev, userMsg])

    // 同步更新缓存
    if (currentSessionId) {
      const cached = sessionMessagesRef.current.get(currentSessionId) || []
      sessionMessagesRef.current.set(currentSessionId, [...cached, userMsg])
    }

    // 创建助手消息占位符
    const assistantMsg: ChatMessage = {
      id: Date.now() + 1,
      sessionId: currentSessionId ?? 0,
      role: 'assistant',
      content: '',
      createdAt: new Date().toISOString(),
    }
    setMessages((prev) => [...prev, assistantMsg])

    // 同步更新缓存
    if (currentSessionId) {
      const cached = sessionMessagesRef.current.get(currentSessionId) || []
      sessionMessagesRef.current.set(currentSessionId, [...cached, assistantMsg])
    }

    let newSessionId = currentSessionId
    let newTitle = ''

    // 标记当前会话正在生成
    const targetSessionId = currentSessionId
    setLoadingSessionId(targetSessionId)

    try {
      await chatSessionApi.chatStream(
        {
          sessionId: currentSessionId ?? undefined,
          content: text,
          topics: selectedTopics.length > 0 ? selectedTopics : undefined,
        },
        {
          onSession: (data) => {
            newSessionId = data.sessionId
            newTitle = data.title
            setCurrentSessionId(data.sessionId)
            setLoadingSessionId(data.sessionId)
          },
          onContent: (chunk) => {
            // 更新缓存和当前显示的消息
            const updateMessages = (prev: ChatMessage[]) => {
              const updated = [...prev]
              const lastMsg = updated[updated.length - 1]
              if (lastMsg && lastMsg.role === 'assistant' &&
                  (lastMsg.sessionId === targetSessionId || lastMsg.sessionId === newSessionId)) {
                updated[updated.length - 1] = {
                  ...lastMsg,
                  content: lastMsg.content + chunk
                }
              }
              return updated
            }

            // 更新缓存
            const sid = newSessionId || targetSessionId
            if (sid) {
              const cached = sessionMessagesRef.current.get(sid) || []
              sessionMessagesRef.current.set(sid, updateMessages(cached))
            }

            // 只有当前在这个会话时才更新显示
            setMessages((prev) => {
              // 检查当前是否还在目标会话
              const currentSid = newSessionId || targetSessionId
              if (currentSessionId !== currentSid) {
                return prev
              }
              return updateMessages(prev)
            })
          },
          onDone: (data) => {
            if (data.title) {
              newTitle = data.title
            }
          },
          onError: (error) => {
            message.error(error)
          },
          signal: controller.signal,
        }
      )

      // 流式完成后刷新会话列表
      await loadSessions()

      // 如果标题有更新，同步到会话列表
      if (newTitle && newSessionId) {
        setSessions((prev) =>
          prev.map((s) => (s.id === newSessionId ? { ...s, title: newTitle } : s))
        )
      }
    } catch (err: any) {
      // 如果是用户主动中断，不显示错误
      if (err?.name === 'AbortError' || controller.signal.aborted) {
        message.info('已停止生成')
      } else {
        message.error('发送失败')
        setInput(text)
        // 移除添加的消息
        setMessages((prev) => prev.slice(0, -2))
      }
    } finally {
      setLoading(false)
      setLoadingSessionId(null)
      abortControllerRef.current = null
    }
  }, [input, loading, currentSessionId, loadSessions, selectedTopics])

  const handleCopy = useCallback((messageId: number, content: string) => {
    navigator.clipboard.writeText(content).then(() => {
      setCopiedId(messageId)
      message.success('已复制到剪贴板')
      setTimeout(() => setCopiedId(null), 2000)
    }).catch(() => {
      message.error('复制失败')
    })
  }, [])

  const handleRegenerate = useCallback(async () => {
    if (loading || messages.length < 2 || !currentSessionId) return

    // 确保当前显示的 messages 属于当前会话
    const sessionMessages = messages.filter(m => m.sessionId === currentSessionId)
    if (sessionMessages.length < 2) return

    const lastUserMsg = [...sessionMessages].reverse().find(m => m.role === 'user')
    if (!lastUserMsg) return

    try {
      await chatSessionApi.regenerate(currentSessionId)

      // 创建 AbortController
      const controller = new AbortController()
      abortControllerRef.current = controller

      // 只移除当前会话的最后一条 assistant 消息
      setMessages((prev) => {
        const lastAssistantIndex = [...prev].reverse().findIndex(m =>
          m.role === 'assistant' && m.sessionId === currentSessionId
        )
        if (lastAssistantIndex === -1) return prev
        const actualIndex = prev.length - 1 - lastAssistantIndex
        return [...prev.slice(0, actualIndex), ...prev.slice(actualIndex + 1)]
      })

      // 同步更新缓存
      if (currentSessionId) {
        const cached = sessionMessagesRef.current.get(currentSessionId) || []
        const lastAssistantIndex = [...cached].reverse().findIndex(m =>
          m.role === 'assistant' && m.sessionId === currentSessionId
        )
        if (lastAssistantIndex !== -1) {
          const actualIndex = cached.length - 1 - lastAssistantIndex
          sessionMessagesRef.current.set(currentSessionId, [
            ...cached.slice(0, actualIndex),
            ...cached.slice(actualIndex + 1)
          ])
        }
      }

      setLoading(true)
      setLoadingSessionId(currentSessionId)

      const targetSessionId = currentSessionId

      const assistantMsg: ChatMessage = {
        id: Date.now(),
        sessionId: currentSessionId,
        role: 'assistant',
        content: '',
        createdAt: new Date().toISOString(),
      }
      setMessages((prev) => [...prev, assistantMsg])

      // 同步更新缓存
      if (currentSessionId) {
        const cached = sessionMessagesRef.current.get(currentSessionId) || []
        sessionMessagesRef.current.set(currentSessionId, [...cached, assistantMsg])
      }

      let newTitle = ''

      await chatSessionApi.chatStream(
        {
          sessionId: currentSessionId,
          content: lastUserMsg.content,
          topics: selectedTopics.length > 0 ? selectedTopics : undefined,
          isRegenerate: true,
        },
        {
          onSession: (data) => {
            if (data.title) {
              newTitle = data.title
            }
          },
          onContent: (chunk) => {
            const updateMessages = (prev: ChatMessage[]) => {
              const updated = [...prev]
              const lastMsg = updated[updated.length - 1]
              if (lastMsg && lastMsg.role === 'assistant' && lastMsg.sessionId === targetSessionId) {
                updated[updated.length - 1] = {
                  ...lastMsg,
                  content: lastMsg.content + chunk
                }
              }
              return updated
            }

            // 更新缓存
            if (targetSessionId) {
              const cached = sessionMessagesRef.current.get(targetSessionId) || []
              sessionMessagesRef.current.set(targetSessionId, updateMessages(cached))
            }

            // 只有当前在这个会话时才更新显示
            setMessages((prev) => {
              if (currentSessionId !== targetSessionId) {
                return prev
              }
              return updateMessages(prev)
            })
          },
          onDone: (data) => {
            if (data.title) {
              newTitle = data.title
            }
          },
          onError: (error) => {
            message.error(error)
          },
          signal: controller.signal,
        }
      )

      await loadSessions()

      if (newTitle) {
        setSessions((prev) =>
          prev.map((s) => (s.id === currentSessionId ? { ...s, title: newTitle } : s))
        )
      }
    } catch (err: any) {
      // 如果是用户主动中断，不显示错误
      if (err?.name === 'AbortError' || abortControllerRef.current?.signal.aborted) {
        message.info('已停止生成')
      } else {
        message.error('重新生成失败')
      }
    } finally {
      setLoading(false)
      setLoadingSessionId(null)
      abortControllerRef.current = null
    }
  }, [loading, messages, currentSessionId, loadSessions, selectedTopics])

  const handleStop = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
      abortControllerRef.current = null
    }
  }, [])

  const currentSession = sessions.find((s) => s.id === currentSessionId)
  const titleBar = currentSession?.title ?? '舆情分析助手'

  return (
    <div className={page.pageShellFlush}>
    <div className={styles.shell}>
      <aside className={styles.sidebar}>
        <div className={styles.sidebarHeader}>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => void handleNewChat()}
            loading={creating}
            block
          >
            新对话
          </Button>
        </div>
        <div className={styles.sessionList}>
          {loadingSessions ? (
            <div className={styles.sessionListPlaceholder}>
              <Spin size="small" />
            </div>
          ) : sessions.length === 0 ? (
            <div className={styles.sessionListPlaceholder}>
              <Text type="secondary">暂无历史，点「新对话」开始</Text>
            </div>
          ) : (
            sessions.map((s) => (
              <div
                key={s.id}
                className={
                  s.id === currentSessionId
                    ? `${styles.sessionItem} ${styles.sessionItemActive}`
                    : styles.sessionItem
                }
                onClick={() => void loadSession(s.id)}
              >
                <div className={styles.sessionItemMain}>
                  <div className={styles.sessionItemTitle}>{s.title}</div>
                  <div className={styles.sessionItemTime}>
                    {dayjs(s.updatedAt).format('MM-DD HH:mm')}
                  </div>
                </div>
                <div className={styles.sessionItemActions}>
                  <Button
                    type="text"
                    size="small"
                    icon={<EditOutlined />}
                    aria-label="重命名"
                    onClick={(e) => openRename(s, e)}
                  />
                  <Button
                    type="text"
                    size="small"
                    danger
                    icon={<DeleteOutlined />}
                    aria-label="删除"
                    onClick={(e) => handleDeleteSession(s.id, e)}
                  />
                </div>
              </div>
            ))
          )}
        </div>
        <div className={styles.sidebarFoot}>
          <Text type="secondary" className={styles.sidebarFootText}>
          历史对话
          </Text>
        </div>
      </aside>

      <div className={styles.mainArea}>
        <div className={styles.page}>
          <header className={styles.topBar}>
            <div className={styles.topBarInner}>
              <div className={styles.topBarSide}>
                <span className={styles.cornerMuted}>智能助手</span>
              </div>
              <h1 className={styles.sessionTitle}>{titleBar}</h1>
              <div className={`${styles.topBarSide} ${styles.topBarSideEnd}`}>
                {topicOptions.length > 0 && (
                  <Select
                    mode="multiple"
                    placeholder="选择话题筛选"
                    value={selectedTopics}
                    onChange={setSelectedTopics}
                    options={topicOptions.map(t => ({ label: t, value: t }))}
                    loading={loadingTopics}
                    allowClear
                    maxTagCount="responsive"
                    style={{ width: 280 }}
                    size="middle"
                  />
                )}
              </div>
            </div>
          </header>

          <main
            className={
              messages.length === 0 && loadingSessionId !== currentSessionId
                ? `${styles.doc} ${styles.docEmpty}`
                : styles.doc
            }
            ref={threadRef}
          >
            <div className={styles.docInner}>
              {messages.length === 0 ? (
                <div className={styles.heroEmpty}>
                  <h2 className={styles.heroTitle}>{HERO_HEADING}</h2>
                  <p className={styles.heroLead}>{WELCOME_SHORT}</p>
                  <p className={styles.heroMuted}>{WELCOME_DETAIL}</p>
                </div>
              ) : (
                messages.map((m, idx) => {
                  const isUser = m.role === 'user'
                  const isLastAssistant = !isUser && idx === messages.length - 1
                  return (
                    <article key={m.id} className={styles.block}>
                      {isUser ? (
                        <div className={styles.userBlock}>
                          <div className={styles.userContent}>{m.content}</div>
                        </div>
                      ) : (
                        <div className={styles.assistantBlock}>
                          <div className={styles.assistantProse}>
                            <ReactMarkdown remarkPlugins={[remarkGfm]}>
                              {m.content}
                            </ReactMarkdown>
                          </div>
                          {m.content && loadingSessionId !== currentSessionId && (
                            <div className={styles.messageActions}>
                              <Button
                                type="text"
                                size="small"
                                icon={<CopyOutlined />}
                                onClick={() => handleCopy(m.id, m.content)}
                                className={styles.actionBtn}
                              >
                                {copiedId === m.id ? '已复制' : '复制'}
                              </Button>
                              {isLastAssistant && (
                                <Button
                                  type="text"
                                  size="small"
                                  icon={<RedoOutlined />}
                                  onClick={() => void handleRegenerate()}
                                  className={styles.actionBtn}
                                >
                                  重新生成
                                </Button>
                              )}
                            </div>
                          )}
                        </div>
                      )}
                    </article>
                  )
                })
              )}
              {loadingSessionId === currentSessionId && (
                <div className={styles.block}>
                  <div className={styles.thinkingRow}>
                    <Spin size="small" />
                    <span>正在生成回复…</span>
                  </div>
                </div>
              )}
            </div>
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
                      if (loadingSessionId === currentSessionId) {
                        handleStop()
                      } else {
                        void handleSend()
                      }
                    }
                  }}
                  disabled={loadingSessionId === currentSessionId}
                  placeholder={
                    messages.length === 0
                      ? '发消息，Enter 发送 · Shift+Enter 换行'
                      : '继续对话，Enter 发送 · Shift+Enter 换行'
                  }
                  autoSize={{ minRows: 1, maxRows: 6 }}
                  style={{ flex: 1 }}
                />
                {loadingSessionId === currentSessionId ? (
                  <Button
                    danger
                    shape="circle"
                    className={styles.sendCircle}
                    onClick={handleStop}
                    aria-label="停止生成"
                  >
                    <span style={{
                      display: 'inline-block',
                      width: '10px',
                      height: '10px',
                      backgroundColor: 'currentColor',
                      borderRadius: '2px'
                    }} />
                  </Button>
                ) : (
                  <Button
                    type="primary"
                    shape="circle"
                    className={styles.sendCircle}
                    icon={<SendOutlined />}
                    onClick={() => void handleSend()}
                    disabled={!input.trim()}
                    aria-label="发送"
                  />
                )}
              </div>
              <Text type="secondary" className={styles.composerMeta}>
              </Text>
            </div>
          </div>
        </div>
      </div>

      <Modal
        title="重命名会话"
        open={renameTarget !== null}
        onOk={() => void submitRename()}
        onCancel={() => setRenameTarget(null)}
        destroyOnClose
      >
        <Form form={renameForm} layout="vertical">
          <Form.Item
            name="title"
            label="标题"
            rules={[{ required: true, message: '请输入标题' }]}
          >
            <Input maxLength={128} placeholder="会话标题" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
    </div>
  )
}

export default AiAssistantPage

import request from './request'

export interface ChatSession {
  id: number
  userId: number
  title: string
  createdAt: string
  updatedAt: string
}

export interface ChatMessage {
  id: number
  sessionId: number
  role: 'user' | 'assistant'
  content: string
  createdAt: string
}

export interface SessionWithMessages {
  session: ChatSession
  messages: ChatMessage[]
}

export interface ChatRequest {
  sessionId?: number | null
  content: string
  pageHint?: string
  useRag?: boolean
  topics?: string[]
  isRegenerate?: boolean
}

export interface ChatResponse {
  sessionId: number
  title: string
  reply: string
}

export interface StreamChatCallbacks {
  onSession?: (data: { sessionId: number; title: string; ragUsed: boolean }) => void
  onContent: (chunk: string) => void
  onDone?: (data: { done: boolean; title?: string }) => void
  onError?: (error: string) => void
  signal?: AbortSignal
}

function buildChatBody(data: ChatRequest): Record<string, unknown> {
  const body: Record<string, unknown> = { content: data.content }
  if (data.sessionId != null) body.sessionId = data.sessionId
  if (data.pageHint != null && data.pageHint !== '')
    body.pageHint = data.pageHint
  if (data.useRag != null) body.useRag = data.useRag
  if (data.topics != null && data.topics.length > 0) body.topics = data.topics
  if (data.isRegenerate != null) body.isRegenerate = data.isRegenerate
  return body
}

export const chatSessionApi = {
  list: async () =>
    (await request.get('/ai/sessions')) as { list: ChatSession[] },

  /** 后端按 user_id 隔离；新建空会话供侧栏「新对话」 */
  create: async (title?: string) =>
    (await request.post('/ai/sessions', title?.trim() ? { title } : {})) as {
      session: ChatSession
    },

  get: async (id: number) =>
    (await request.get(`/ai/sessions/${id}`)) as SessionWithMessages,

  delete: async (id: number) => {
    await request.delete(`/ai/sessions/${id}`)
  },

  rename: async (id: number, title: string) => {
    await request.patch(`/ai/sessions/${id}`, { title })
  },

  regenerate: async (id: number) => {
    await request.post(`/ai/sessions/${id}/regenerate`, {})
  },

  chat: async (data: ChatRequest) =>
    (await request.post(
      '/ai/sessions/chat',
      buildChatBody(data),
      { timeout: 120000 }
    )) as ChatResponse,

  /** 流式聊天，通过回调接收增量内容 */
  chatStream: async (data: ChatRequest, callbacks: StreamChatCallbacks) => {
    // 从 localStorage 获取 token（与 zustand store 同步）
    const token = localStorage.getItem('auth-storage')
    let authToken = ''
    if (token) {
      try {
        const parsed = JSON.parse(token)
        authToken = parsed.state?.token || ''
      } catch {
        // ignore
      }
    }

    const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
    const url = `${baseURL}/ai/sessions/chat`

    const controller = new AbortController()
    const timeoutId = setTimeout(() => controller.abort(), 120000) // 2分钟超时

    // 如果外部传入了 signal，监听它的 abort 事件
    if (callbacks.signal) {
      callbacks.signal.addEventListener('abort', () => controller.abort())
    }

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(authToken ? { 'Authorization': `Bearer ${authToken}` } : {}),
        },
        body: JSON.stringify(buildChatBody(data)),
        signal: controller.signal,
      })

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`)
      }

      const reader = response.body?.getReader()
      if (!reader) {
        throw new Error('No response body')
      }

      const decoder = new TextDecoder()
      let buffer = ''
      let streamCompleted = false

      try {
        while (true) {
          const { done, value } = await reader.read()
          if (done) {
            streamCompleted = true
            break
          }

          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n')
          buffer = lines.pop() || ''

          for (const line of lines) {
            if (!line.trim() || line.startsWith(':')) continue

            if (line.startsWith('event:')) {
              // 事件类型行，跳过
              continue
            }

            if (line.startsWith('data:')) {
              const data = line.substring(5).trim()
              try {
                const parsed = JSON.parse(data)

                // 处理不同类型的事件
                if (parsed.sessionId !== undefined) {
                  // session 事件
                  callbacks.onSession?.(parsed)
                } else if (parsed.content !== undefined) {
                  // 增量内容
                  callbacks.onContent(parsed.content)
                } else if (parsed.done === true) {
                  // 完成事件
                  streamCompleted = true
                  callbacks.onDone?.(parsed)
                } else if (parsed.error !== undefined) {
                  // 错误事件
                  callbacks.onError?.(parsed.error)
                  throw new Error(parsed.error)
                } else if (parsed.ragUsed !== undefined) {
                  // meta 事件（无状态聊天）
                  // 可以忽略或记录
                }
              } catch (e) {
                console.warn('Failed to parse SSE data:', data, e)
              }
            }
          }
        }
      } finally {
        reader.releaseLock()
      }

      if (!streamCompleted) {
        console.warn('Stream ended without done event')
      }
    } finally {
      clearTimeout(timeoutId)
    }
  },
}

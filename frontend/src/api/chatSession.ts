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
  webSearch?: boolean
  topics?: string[]
  isRegenerate?: boolean
}

export interface ChatResponse {
  sessionId: number
  title: string
  reply: string
}

export interface ThinkStep {
  step: 'intent' | 'retrieval' | 'reasoning' | 'generate' | string
  title: string
  content?: string
  status: 'running' | 'done' | 'skipped' | 'error'
}

export interface StreamChatCallbacks {
  onSession?: (data: { sessionId: number; title: string; ragUsed: boolean; deepThink?: boolean }) => void
  onContent: (chunk: string) => void
  onDone?: (data: { done: boolean; title?: string }) => void
  onError?: (error: string) => void
  onThinkStep?: (step: ThinkStep) => void
  signal?: AbortSignal
}

function buildChatBody(data: ChatRequest): Record<string, unknown> {
  const body: Record<string, unknown> = { content: data.content }
  if (data.sessionId != null) body.sessionId = data.sessionId
  if (data.pageHint != null && data.pageHint !== '')
    body.pageHint = data.pageHint
  if (data.useRag != null) body.useRag = data.useRag
  if (data.webSearch != null) body.webSearch = data.webSearch
  if (data.topics != null && data.topics.length > 0) body.topics = data.topics
  if (data.isRegenerate != null) body.isRegenerate = data.isRegenerate
  return body
}

function getAuthToken(): string {
  const stored = localStorage.getItem('auth-storage')
  if (!stored) return ''
  try {
    return JSON.parse(stored).state?.token || ''
  } catch {
    return ''
  }
}

/** Shared SSE stream consumer used by both chatStream and deepChatStream. */
async function consumeSSEStream(
  url: string,
  body: Record<string, unknown>,
  callbacks: StreamChatCallbacks,
  timeoutMs = 120000,
): Promise<void> {
  const authToken = getAuthToken()
  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs)

  if (callbacks.signal) {
    callbacks.signal.addEventListener('abort', () => controller.abort())
  }

  try {
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
      },
      body: JSON.stringify(body),
      signal: controller.signal,
    })

    if (!response.ok) throw new Error(`HTTP ${response.status}`)

    const reader = response.body?.getReader()
    if (!reader) throw new Error('No response body')

    const decoder = new TextDecoder()
    let buffer = ''
    let streamCompleted = false
    let lastEventType = ''

    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) { streamCompleted = true; break }

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          if (!line.trim() || line.startsWith(':')) continue

          if (line.startsWith('event:')) {
            lastEventType = line.substring(6).trim()
            continue
          }

          if (line.startsWith('data:')) {
            const rawData = line.substring(5).trim()
            try {
              const parsed = JSON.parse(rawData)

              if (lastEventType === 'think_step') {
                callbacks.onThinkStep?.(parsed as ThinkStep)
              } else if (lastEventType === 'session' || parsed.sessionId !== undefined) {
                callbacks.onSession?.(parsed)
              } else if (lastEventType === 'done' || parsed.done === true) {
                streamCompleted = true
                callbacks.onDone?.(parsed)
              } else if (lastEventType === 'error' || parsed.error !== undefined) {
                callbacks.onError?.(parsed.error)
                throw new Error(parsed.error)
              } else if (parsed.content !== undefined) {
                callbacks.onContent(parsed.content)
              }
            } catch (e) {
              if (e instanceof Error && lastEventType === 'error') throw e
              console.warn('Failed to parse SSE data:', rawData)
            }
            lastEventType = ''
          }
        }
      }
    } finally {
      reader.releaseLock()
    }

    if (!streamCompleted) console.warn('Stream ended without done event')
  } finally {
    clearTimeout(timeoutId)
  }
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
    const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
    await consumeSSEStream(`${baseURL}/ai/sessions/chat`, buildChatBody(data), callbacks)
  },

  /** 深度思考流式聊天：运行多轮工具调用推理流水线后再生成回答（耗时更长，超时放宽到 180s） */
  deepChatStream: async (data: ChatRequest, callbacks: StreamChatCallbacks) => {
    const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
    await consumeSSEStream(`${baseURL}/ai/sessions/chat/deep`, buildChatBody(data), callbacks, 180000)
  },

  /** 查询深度思考模式可用能力（如联网搜索是否由管理员启用） */
  capabilities: async () =>
    (await request.get('/ai/sessions/capabilities')) as { webSearch: boolean },
}

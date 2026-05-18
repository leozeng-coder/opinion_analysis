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
}

export interface ChatResponse {
  sessionId: number
  title: string
  reply: string
}

function buildChatBody(data: ChatRequest): Record<string, unknown> {
  const body: Record<string, unknown> = { content: data.content }
  if (data.sessionId != null) body.sessionId = data.sessionId
  if (data.pageHint != null && data.pageHint !== '')
    body.pageHint = data.pageHint
  if (data.useRag != null) body.useRag = data.useRag
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

  chat: async (data: ChatRequest) =>
    (await request.post(
      '/ai/sessions/chat',
      buildChatBody(data),
      { timeout: 120000 }
    )) as ChatResponse,
}

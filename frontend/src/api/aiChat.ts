import request from './request'

export interface AiChatTurn {
  role: 'user' | 'assistant'
  content: string
}

export async function postAiChat(payload: { messages: AiChatTurn[]; pageHint?: string }) {
  const data = (await request.post('/ai/chat', payload, {
    timeout: 120000,
  })) as { reply: string }
  return data
}
